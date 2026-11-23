-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Guarda de escritura de la fuente de One (ADR 0086), como la de contacts (0009 de
-- contacts): mientras la fuente de una persona no es `legacy` (building, frozen o
-- private), sus filas de `notifications` y `notification_prefs` no se crean, cambian
-- ni borran. Sin ella, congelar solo cambiaba a dónde iban las apps nativas: la web y
-- los servicios seguían escribiendo, y «soltar la fuente» (0012) devolvía a legacy algo
-- distinto de lo congelado.
--
-- Va DESPUÉS de la entrada privada (0014) a propósito: un aviso que un servicio deja a
-- una persona con la fuente congelada ya no llega aquí, va cifrado a la entrada
-- (store.AdmitirAvisoPrivado). Esta guarda no tira ningún aviso; si alguno llegara
-- por otro camino, el rechazo es un error para quien lo manda, nunca un silencio.
--
-- La dueña de una fila es (tenant, subject): lo mismo que captura
-- hub_freeze_private_storage (0013). El bloqueo de fila, la carrera con la congelación
-- y el «no» tras congelar son de la función común sytue_private_storage_guard
-- (platform/pkg/pg/private_storage_v1.sql); aquí solo se decide a quién mirar.
--
-- Excepciones, a propósito:
--   · Un `subject` que no puede ser dueña de una fuente —un buzón por correo
--     (`email:…`, hubnotify.PorCorreo), vacío o de más de 1024 bytes— no crea fila de
--     estado: solo se rechaza si alguien tuviera ya una congelada con ese nombre.
--   · Un DELETE con `app.merge='on'` pasa: es la baja de la cuenta
--     (services/id merge.BorrarPersona, en InMergeTx). La fusión de organizaciones
--     solo hace UPDATE y sigue guardada. Sin esto, nadie que hubiera migrado One
--     podría borrar su cuenta.

-- Predicado de enrutado para los trabajos por lotes (purga, marcar todo en otras
-- organizaciones): no sustituye al cerrojo de la guarda; evita que una persona
-- congelada tumbe el lote de todas las demás. Nunca falla por el formato del subject.
CREATE FUNCTION hub_private_storage_frozen(t text, s text) RETURNS boolean
LANGUAGE sql STABLE
SET search_path = pg_catalog, public SET app.maintenance = 'on' AS $$
 SELECT coalesce(s, '') <> '' AND octet_length(s) <= 1024
  AND EXISTS (SELECT 1 FROM public.hub_private_storage_states st
               WHERE st.tenant = t AND st.owner_sub = s AND st.mode <> 'legacy');
$$;

CREATE FUNCTION hub_private_row_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path = pg_catalog, public SET app.maintenance = 'on' AS $$
DECLARE
 value jsonb;
 t text;
 s text;
BEGIN
 IF TG_OP = 'DELETE' AND current_setting('app.merge', true) = 'on' THEN
  RETURN OLD;
 END IF;
 -- OLD y NEW: cambiar el subject o el tenant de una fila toca a las dos dueñas.
 FOR value IN SELECT v FROM (VALUES
   (CASE WHEN TG_OP <> 'INSERT' THEN to_jsonb(OLD) END),
   (CASE WHEN TG_OP <> 'DELETE' THEN to_jsonb(NEW) END)) x(v) WHERE v IS NOT NULL
 LOOP
  t := value->>'tenant';
  s := value->>'subject';
  IF coalesce(s, '') = '' OR octet_length(s) > 1024 OR s LIKE 'email:%' THEN
   IF public.hub_private_storage_frozen(t, s) THEN
    RAISE EXCEPTION USING ERRCODE = 'PST01', MESSAGE = 'private_storage_legacy_write_blocked';
   END IF;
  ELSE
   PERFORM public.sytue_private_storage_guard('public.hub_private_storage_states'::regclass, t, s);
  END IF;
 END LOOP;
 IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$;

CREATE TRIGGER hub_private_notifications_guard BEFORE INSERT OR UPDATE OR DELETE ON notifications
 FOR EACH ROW EXECUTE FUNCTION hub_private_row_guard();
CREATE TRIGGER hub_private_prefs_guard BEFORE INSERT OR UPDATE OR DELETE ON notification_prefs
 FOR EACH ROW EXECUTE FUNCTION hub_private_row_guard();
