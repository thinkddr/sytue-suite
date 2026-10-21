-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Avisos RECONSTRUIDOS de lo que pasó antes de que el registro de avisos funcionara
-- (hasta el 2026-09-25 ningún servicio llegaba a guardar uno). Los deja el comando de
-- mantenimiento `avisos-historicos` de cada servicio por POST
-- /internal/notifications/historic, nunca la ruta de siempre.
--
-- `clave_origen` es el elemento del que sale el aviso («mail:<id del mensaje>»): con
-- ella el mismo elemento da un solo aviso a cada persona por muchas veces que se lance
-- el comando. NULL es un aviso en vivo, y ser NOT NULL es también la MARCA que la
-- campana enseña («anterior al registro de avisos»): una sola columna para las dos
-- cosas, así no pueden contradecirse.
ALTER TABLE notifications ADD COLUMN clave_origen text;

CREATE UNIQUE INDEX notifications_clave_origen
    ON notifications (tenant, subject, clave_origen)
    WHERE clave_origen IS NOT NULL;

-- El primer aviso EN VIVO de cada tipo, que es el techo de lo que se reconstruye.
CREATE INDEX notifications_primer_en_vivo
    ON notifications (tenant, type, created_at)
    WHERE clave_origen IS NULL;

-- La marca tampoco se cambia después: de un aviso escrito solo cambian `read` y
-- `read_at` (0006), y ahora la lista incluye `clave_origen`.
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
        NEW.body, NEW.url, NEW.params, NEW.created_at, NEW.clave_origen)
       IS DISTINCT FROM
       (OLD.id, OLD.tenant, OLD.subject, OLD.source, OLD.type, OLD.level, OLD.title,
        OLD.body, OLD.url, OLD.params, OLD.created_at, OLD.clave_origen) THEN
        RAISE EXCEPTION 'notifications: de un aviso solo se cambia si está leído';
    END IF;
    RETURN NEW;
END
$$;
