-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- VERI*FACTU (RD 1007/2023, Orden HAC/1177/2024): cada factura emitida deja un
-- registro de facturación encadenado con el anterior, que se envía a la Agencia
-- Tributaria. Lo que aquí manda la base, y no el código, es lo que la ley pide
-- que no se pueda deshacer: la cadena sin bifurcaciones, los registros sin
-- cambios ni borrados y la factura emitida sin retoques.

-- Desde cuándo la organización factura con VERI*FACTU (nulo = no lo usa) y su
-- número de instalación, que no se repite nunca para el mismo NIF (FAQ AEAT, 4).
-- `verifactu_proximo_envio` es el control de flujo que marca la AEAT, compartido
-- entre réplicas.
ALTER TABLE organizacion
    ADD COLUMN verifactu_desde         timestamptz,
    ADD COLUMN verifactu_instalacion   text        NOT NULL DEFAULT '',
    ADD COLUMN verifactu_proximo_envio timestamptz NOT NULL DEFAULT '-infinity';

CREATE TABLE verifactu_registros (
    id                uuid        PRIMARY KEY,
    secuencia         bigint      GENERATED ALWAYS AS IDENTITY,
    tenant            text        NOT NULL,
    instalacion       text        NOT NULL,
    factura           uuid        NOT NULL,
    tipo              text        NOT NULL,
    nif               text        NOT NULL,
    num_serie         text        NOT NULL,
    fecha_expedicion  date        NOT NULL,
    huella            text        NOT NULL,
    huella_anterior   text        NOT NULL,
    xml               text        NOT NULL,
    generado_en       timestamptz NOT NULL,
    generado_por      text        NOT NULL,
    -- Lo que contestó la AEAT. Lo único que cambia de la fila.
    estado            text        NOT NULL DEFAULT 'pendiente',
    csv               text        NOT NULL DEFAULT '',
    codigo_error      integer,
    descripcion_error text        NOT NULL DEFAULT '',
    intentos          integer     NOT NULL DEFAULT 0,
    enviado_en        timestamptz,

    CONSTRAINT verifactu_registros_tipo CHECK (tipo IN ('alta', 'anulacion')),
    CONSTRAINT verifactu_registros_estado CHECK (estado IN ('pendiente', 'correcto', 'aceptado_con_errores', 'incorrecto')),
    -- Dos registros no pueden colgar del mismo anterior: la cadena no se bifurca
    -- aunque dos réplicas emitan a la vez.
    CONSTRAINT verifactu_registros_cadena UNIQUE (tenant, instalacion, huella_anterior)
);

CREATE INDEX verifactu_registros_de_factura ON verifactu_registros (tenant, factura, secuencia);
CREATE INDEX verifactu_registros_pendientes ON verifactu_registros (tenant, secuencia) WHERE estado = 'pendiente';

ALTER TABLE verifactu_registros ENABLE ROW LEVEL SECURITY;
ALTER TABLE verifactu_registros FORCE ROW LEVEL SECURITY;

CREATE POLICY verifactu_registros_tenant_isolation ON verifactu_registros
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Literal, como en la 0009: el gate de fusión lee el SQL sin ejecutarlo.
CREATE POLICY verifactu_registros_platform_merge ON verifactu_registros
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

-- El envío necesita saber qué organizaciones tienen algo pendiente antes de fijar
-- el tenant de cada una. Solo lectura.
CREATE POLICY verifactu_registros_maintenance ON verifactu_registros
    FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on');

-- Un registro generado no se cambia ni se borra: solo se anota lo que contestó la
-- AEAT. (El tenant sí puede cambiar: es lo que reescribe una fusión.)
CREATE FUNCTION verifactu_registro_inalterable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'un registro de facturación no se borra' USING ERRCODE = 'SY001';
    END IF;
    IF (NEW.id, NEW.secuencia, NEW.instalacion, NEW.factura, NEW.tipo, NEW.nif, NEW.num_serie,
        NEW.fecha_expedicion, NEW.huella, NEW.huella_anterior, NEW.xml, NEW.generado_en, NEW.generado_por)
       IS DISTINCT FROM
       (OLD.id, OLD.secuencia, OLD.instalacion, OLD.factura, OLD.tipo, OLD.nif, OLD.num_serie,
        OLD.fecha_expedicion, OLD.huella, OLD.huella_anterior, OLD.xml, OLD.generado_en, OLD.generado_por) THEN
        RAISE EXCEPTION 'un registro de facturación no se modifica' USING ERRCODE = 'SY001';
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER verifactu_registros_inalterables
    BEFORE UPDATE OR DELETE ON verifactu_registros
    FOR EACH ROW EXECUTE FUNCTION verifactu_registro_inalterable();

-- Una factura con registro ya no vuelve a borrador, no va a la papelera, no cambia
-- de número, no se anula sin su registro de anulación y, anulada, no revive. Los datos que lleva el
-- registro tampoco cambian, salvo para corregir un alta que la AEAT rechazó: lo
-- demás se corrige con una rectificativa. Las notas y el vencimiento, sí.
CREATE FUNCTION verifactu_factura_emitida() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    ultimo_tipo   text;
    ultimo_estado text;
BEGIN
    SELECT tipo, estado INTO ultimo_tipo, ultimo_estado
      FROM verifactu_registros
     WHERE tenant = OLD.tenant AND factura = OLD.id
     ORDER BY secuencia DESC
     LIMIT 1;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    IF TG_OP = 'DELETE'
       OR NEW.estado = 'borrador'
       OR (OLD.estado = 'anulado' AND NEW.estado <> 'anulado')
       OR (NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL)
       OR NEW.ref IS DISTINCT FROM OLD.ref THEN
        RAISE EXCEPTION 'la factura ya se ha enviado a la Agencia Tributaria' USING ERRCODE = 'SY002';
    END IF;
    IF NEW.estado = 'anulado' AND OLD.estado <> 'anulado' AND ultimo_tipo <> 'anulacion' THEN
        RAISE EXCEPTION 'la factura se anula con su registro de anulación' USING ERRCODE = 'SY002';
    END IF;
    IF NOT (ultimo_tipo = 'alta' AND ultimo_estado = 'incorrecto')
       AND (NEW.data->'fecha', NEW.data->'cliente', NEW.data->'base', NEW.data->'iva', NEW.data->'total',
            NEW.data->'regimen_iva', NEW.data->'rectifica')
           IS DISTINCT FROM
           (OLD.data->'fecha', OLD.data->'cliente', OLD.data->'base', OLD.data->'iva', OLD.data->'total',
            OLD.data->'regimen_iva', OLD.data->'rectifica') THEN
        RAISE EXCEPTION 'la factura ya se ha enviado a la Agencia Tributaria' USING ERRCODE = 'SY002';
    END IF;
    RETURN NULL;
END
$$;

-- AFTER y con WHEN: el resto de modelos ni llaman a la función, y dentro de la
-- transacción ya se ven los registros que se acaban de generar.
CREATE TRIGGER records_factura_emitida
    AFTER UPDATE OR DELETE ON records
    FOR EACH ROW WHEN (OLD.model = 'factura_venta')
    EXECUTE FUNCTION verifactu_factura_emitida();
