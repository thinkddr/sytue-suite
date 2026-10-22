-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

CREATE OR REPLACE FUNCTION notifications_solo_se_marca()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF current_setting('app.merge', true) = 'on' THEN
        RETURN NEW;
    END IF;
    IF (NEW.id, NEW.tenant, NEW.subject, NEW.source, NEW.type, NEW.level, NEW.title,
        NEW.body, NEW.url, NEW.params, NEW.created_at)
       IS DISTINCT FROM
       (OLD.id, OLD.tenant, OLD.subject, OLD.source, OLD.type, OLD.level, OLD.title,
        OLD.body, OLD.url, OLD.params, OLD.created_at) THEN
        RAISE EXCEPTION 'notifications: de un aviso solo se cambia si está leído';
    END IF;
    RETURN NEW;
END
$$;

DROP INDEX IF EXISTS notifications_primer_en_vivo;
DROP INDEX IF EXISTS notifications_clave_origen;
-- Sin la columna, los reconstruidos serían indistinguibles de los avisos en vivo, y la
-- campana los enseñaría sin su marca: se borran (se pueden volver a reconstruir).
-- Con FORCE ROW LEVEL SECURITY ni el dueño ve las filas: la política de mantenimiento.
SELECT set_config('app.maintenance', 'on', false);
DELETE FROM notifications WHERE clave_origen IS NOT NULL;
SELECT set_config('app.maintenance', '', false);
ALTER TABLE notifications DROP COLUMN IF EXISTS clave_origen;
