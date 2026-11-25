-- SPDX-License-Identifier: Apache-2.0
-- La traza de auditoría de un servicio (docs/security.md §1.7): quién accedió a
-- qué, cuándo y desde dónde, sin el contenido. La escriben dos caminos:
--
--   * el cumplimiento (legal hold, eDiscovery: `legalhold.*`, `ediscovery.*`),
--     que Drive y Mail ya escribían en esta misma tabla;
--   * el ACCESO ordinario (`acceso.*`: descargas, aperturas, compartir,
--     cambios de permisos, exportaciones), por el punto de estrangulamiento de
--     platform/pkg/auditlog (Registro.Middleware y Anotar).
--
-- Idempotente a propósito: en Drive y Mail la tabla ya existe con esta forma y
-- esto le añade lo que le faltaba; en Docs, Rooms, Calendar, Contacts y el Hub
-- la crea.
CREATE TABLE IF NOT EXISTS audit_log (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    tenant      text        NOT NULL,
    actor       text        NOT NULL,                     -- sub de Sytue ID, o «link:<id>» en un enlace público
    action      text        NOT NULL,
    target      text        NOT NULL DEFAULT '',          -- id del recurso; nunca su contenido
    detail      jsonb       NOT NULL DEFAULT '{}'::jsonb  -- request_id, ip, vía; nunca contenido
);
CREATE INDEX IF NOT EXISTS audit_log_tenant_idx ON audit_log (tenant, occurred_at DESC);
-- La retención (`acceso.*` a los doce meses) sin recorrer la traza de cumplimiento.
CREATE INDEX IF NOT EXISTS audit_log_acceso_idx ON audit_log (occurred_at) WHERE action LIKE 'acceso.%';

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS audit_log_tenant_isolation ON audit_log;
CREATE POLICY audit_log_tenant_isolation ON audit_log
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));
DROP POLICY IF EXISTS audit_log_platform_merge ON audit_log;
CREATE POLICY audit_log_platform_merge ON audit_log
    USING (current_setting('app.merge', true) = 'on')
    WITH CHECK (current_setting('app.merge', true) = 'on');
-- La retención: el mantenimiento ve y borra SOLO acceso.* (el cumplimiento sigue
-- su propio plazo, el del caso). Dos políticas y no FOR ALL: el mantenimiento no
-- escribe en la traza. La de lectura hace falta porque un DELETE con WHERE
-- también pasa por las políticas de SELECT.
DROP POLICY IF EXISTS audit_log_retencion_ver ON audit_log;
CREATE POLICY audit_log_retencion_ver ON audit_log FOR SELECT
    USING (current_setting('app.maintenance', true) = 'on' AND action LIKE 'acceso.%');
DROP POLICY IF EXISTS audit_log_retencion ON audit_log;
CREATE POLICY audit_log_retencion ON audit_log FOR DELETE
    USING (current_setting('app.maintenance', true) = 'on' AND action LIKE 'acceso.%');

-- SOLO DE AÑADIR. Nadie edita una línea de la traza, y solo se borra por tres
-- caminos: la retención (mantenimiento, acceso.*), la purga de una organización
-- que desaparece y la fusión, que reescribe el tenant y nada más.
CREATE OR REPLACE FUNCTION audit_log_solo_anadir() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF current_setting('app.merge', true) = 'on'
           AND (to_jsonb(NEW) - 'tenant') = (to_jsonb(OLD) - 'tenant') THEN
            RETURN NEW;
        END IF;
        RAISE EXCEPTION 'audit_log es solo de añadir' USING ERRCODE = 'check_violation';
    END IF;
    IF current_setting('app.merge', true) = 'on'
       OR (current_setting('app.maintenance', true) = 'on' AND OLD.action LIKE 'acceso.%') THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'audit_log es solo de añadir' USING ERRCODE = 'check_violation';
END $$;

DROP TRIGGER IF EXISTS audit_log_solo_anadir ON audit_log;
CREATE TRIGGER audit_log_solo_anadir
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_solo_anadir();
