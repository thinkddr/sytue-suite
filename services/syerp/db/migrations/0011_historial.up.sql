-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El historial de cada registro, como el «chatter» de Odoo: quién lo creó, qué
-- cambió quién y cuándo (campo a campo, con el valor de antes y el de después),
-- cambios de estado, papelera, y las notas que deja la gente.
--
-- Lo escribe la base con un trigger y no el código: así entra TODO lo que cambia
-- un registro —la ficha, un lote, una importación, el banco, Sytue AI— sin que un
-- camino nuevo se lo salte. Quién actúa lo pone el servicio en `app.sub` al abrir
-- la transacción; sin él (un proceso del propio servicio) queda vacío.

CREATE TABLE historial (
    id        uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant    text        NOT NULL,
    modelo    text        NOT NULL,
    registro  uuid        NOT NULL,
    sub       text        NOT NULL DEFAULT '',
    tipo      text        NOT NULL,
    -- {campo: {"antes": …, "despues": …}}; `estado` va como un campo más.
    cambios   jsonb       NOT NULL DEFAULT '{}'::jsonb,
    texto     text        NOT NULL DEFAULT '',
    creado_en timestamptz NOT NULL DEFAULT clock_timestamp(),

    CONSTRAINT historial_tipo CHECK (tipo IN ('creado', 'cambio', 'borrado', 'restaurado', 'nota')),
    CONSTRAINT historial_texto CHECK (length(texto) <= 5000)
);

CREATE INDEX historial_de_registro ON historial (tenant, modelo, registro, creado_en DESC);

ALTER TABLE historial ENABLE ROW LEVEL SECURITY;
ALTER TABLE historial FORCE ROW LEVEL SECURITY;

CREATE POLICY historial_tenant_isolation ON historial
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));

-- Literal, como en la 0009: el gate de fusión lee el SQL sin ejecutarlo.
CREATE POLICY historial_platform_merge ON historial
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE FUNCTION historial_de_records() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    quien   text := COALESCE(current_setting('app.sub', true), '');
    cambios jsonb;
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO historial (tenant, modelo, registro, sub, tipo)
        VALUES (NEW.tenant, NEW.model, NEW.id, quien, 'creado');
        RETURN NULL;
    END IF;
    -- Una fusión de organizaciones reescribe el tenant: no es un cambio del registro.
    IF NEW.tenant IS DISTINCT FROM OLD.tenant THEN
        RETURN NULL;
    END IF;
    IF NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL THEN
        INSERT INTO historial (tenant, modelo, registro, sub, tipo)
        VALUES (NEW.tenant, NEW.model, NEW.id, quien, 'borrado');
    ELSIF NEW.deleted_at IS NULL AND OLD.deleted_at IS NOT NULL THEN
        INSERT INTO historial (tenant, modelo, registro, sub, tipo)
        VALUES (NEW.tenant, NEW.model, NEW.id, quien, 'restaurado');
    END IF;
    SELECT COALESCE(jsonb_object_agg(clave, jsonb_build_object('antes', OLD.data -> clave, 'despues', NEW.data -> clave)), '{}'::jsonb)
      INTO cambios
      FROM jsonb_object_keys(OLD.data || NEW.data) AS clave
     WHERE OLD.data -> clave IS DISTINCT FROM NEW.data -> clave;
    IF NEW.estado IS DISTINCT FROM OLD.estado THEN
        cambios := cambios || jsonb_build_object('estado', jsonb_build_object('antes', OLD.estado, 'despues', NEW.estado));
    END IF;
    IF cambios <> '{}'::jsonb THEN
        INSERT INTO historial (tenant, modelo, registro, sub, tipo, cambios)
        VALUES (NEW.tenant, NEW.model, NEW.id, quien, 'cambio', cambios);
    END IF;
    RETURN NULL;
END
$$;

CREATE TRIGGER records_historial
    AFTER INSERT OR UPDATE ON records
    FOR EACH ROW EXECUTE FUNCTION historial_de_records();
