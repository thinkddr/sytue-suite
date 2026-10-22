-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Líneas de factura y factura simplificada. Las líneas (`linea_factura`) y el tipo
-- de factura (completa o simplificada) salen en el registro de VERI*FACTU y en el
-- impreso: emitida la factura, tampoco cambian. Es la misma regla que la 0010 pone
-- a la cabecera, con la misma excepción (se corrige un alta que la AEAT rechazó),
-- y la pone la base por lo mismo: ningún camino del código se la puede saltar.

-- La cabecera, igual que en la 0010 y con el tipo de factura entre lo que no cambia.
CREATE OR REPLACE FUNCTION verifactu_factura_emitida() RETURNS trigger
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
            NEW.data->'regimen_iva', NEW.data->'rectifica', NEW.data->'tipo_factura')
           IS DISTINCT FROM
           (OLD.data->'fecha', OLD.data->'cliente', OLD.data->'base', OLD.data->'iva', OLD.data->'total',
            OLD.data->'regimen_iva', OLD.data->'rectifica', OLD.data->'tipo_factura') THEN
        RAISE EXCEPTION 'la factura ya se ha enviado a la Agencia Tributaria' USING ERRCODE = 'SY002';
    END IF;
    RETURN NULL;
END
$$;

-- ¿Está cerrada la factura? Lo está si tiene registro y el último no es un alta
-- rechazada (esa se corrige y se vuelve a enviar).
CREATE FUNCTION verifactu_factura_cerrada(p_tenant text, p_factura text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT COALESCE((SELECT NOT (tipo = 'alta' AND estado = 'incorrecto')
                       FROM verifactu_registros
                      WHERE tenant = p_tenant AND factura = p_factura::uuid
                      ORDER BY secuencia DESC
                      LIMIT 1), false)
$$;

-- Una línea de una factura cerrada no se crea, no se cambia, no va a la papelera
-- ni vuelve de ella, y no se borra; tampoco se pasa de una factura a otra. Un
-- cambio que solo toca el tenant (lo que reescribe una fusión) sí pasa.
CREATE FUNCTION verifactu_linea_de_factura_emitida() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND (NEW.data, NEW.deleted_at) IS NOT DISTINCT FROM (OLD.data, OLD.deleted_at) THEN
        RETURN NULL;
    END IF;
    IF TG_OP <> 'INSERT' THEN
        IF verifactu_factura_cerrada(OLD.tenant, OLD.data->>'factura') THEN
            RAISE EXCEPTION 'la factura ya se ha enviado a la Agencia Tributaria' USING ERRCODE = 'SY002';
        END IF;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        IF verifactu_factura_cerrada(NEW.tenant, NEW.data->>'factura') THEN
            RAISE EXCEPTION 'la factura ya se ha enviado a la Agencia Tributaria' USING ERRCODE = 'SY002';
        END IF;
    END IF;
    RETURN NULL;
END
$$;

-- AFTER y con WHEN, como el de la cabecera: el resto de modelos ni llaman a la función.
CREATE TRIGGER records_linea_de_factura_nueva
    AFTER INSERT ON records
    FOR EACH ROW WHEN (NEW.model = 'linea_factura')
    EXECUTE FUNCTION verifactu_linea_de_factura_emitida();

CREATE TRIGGER records_linea_de_factura_emitida
    AFTER UPDATE OR DELETE ON records
    FOR EACH ROW WHEN (OLD.model = 'linea_factura')
    EXECUTE FUNCTION verifactu_linea_de_factura_emitida();
