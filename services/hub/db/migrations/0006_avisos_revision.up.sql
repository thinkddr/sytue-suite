-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Lo que dejó la revisión de seguridad de los avisos de la persona (ADR 0053).

-- 1. Marcar como leído es lo ÚNICO que se puede cambiar de un aviso. La política de
--    UPDATE de la 0005 es por fila; esto la hace por columna: de un aviso ya escrito
--    solo cambian `read` y `read_at`. La fusión de organizaciones (`app.merge`)
--    reescribe `tenant`, y es la única excepción.
CREATE FUNCTION notifications_solo_se_marca()
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

CREATE TRIGGER notifications_solo_se_marca
    BEFORE UPDATE ON notifications
    FOR EACH ROW EXECUTE FUNCTION notifications_solo_se_marca();

-- 2. Las preferencias de silencio son de la PERSONA, como sus avisos: lo que calla en
--    una de sus organizaciones calla también lo que le llega de otra. El servicio las
--    lee al crear un aviso, con `app.destinatarios` = ese destinatario, y solo para
--    decidir si lo entrega: nunca salen hacia fuera. Solo lectura.
CREATE POLICY notification_prefs_destinatario_lee ON notification_prefs
    FOR SELECT
    USING (subject = ANY (string_to_array(nullif(current_setting('app.destinatarios', true), ''), chr(31))));

-- 3. Las direcciones que Sytue ID retira (`/internal/notifications/forget`): sus avisos
--    se borran en todas las organizaciones, y la dirección se apunta aquí con la hora.
--    Un token emitido ANTES de esa hora no la puede usar para leer avisos: el de quien la
--    tenía sigue vivo unos minutos, y en ese rato no debe ver los de quien la recibe.
CREATE TABLE buzones_olvidados (
    buzon    text        PRIMARY KEY,
    olvidado timestamptz NOT NULL DEFAULT now()
);
