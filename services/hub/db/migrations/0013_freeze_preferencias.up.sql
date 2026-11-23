-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- hub_freeze_private_storage (0011) leía `notification_preferences` y su columna
-- `type`, que no existen: la tabla es `notification_prefs` (0002) y la columna,
-- `kind`. Toda congelación de la fuente de One fallaba, así que ninguna persona
-- podía empezar la migración de sus avisos. Se redefine con los nombres reales; el
-- resto de la función no cambia.
CREATE OR REPLACE FUNCTION hub_freeze_private_storage(p_tenant text, p_owner_sub text, p_epoch uuid)
RETURNS bigint LANGUAGE plpgsql
SET search_path = pg_catalog, public AS $$
DECLARE
 v_mode text;
 v_epoch uuid;
 v_rows bigint := 0;
BEGIN
 IF p_tenant IS NULL OR p_tenant = '' OR p_owner_sub IS NULL OR p_owner_sub = '' OR p_epoch IS NULL THEN
  RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'private_storage_scope_invalid';
 END IF;
 INSERT INTO hub_private_storage_states (tenant, owner_sub)
  VALUES (p_tenant, p_owner_sub)
  ON CONFLICT (tenant, owner_sub) DO NOTHING;
 SELECT mode, epoch INTO v_mode, v_epoch
  FROM hub_private_storage_states
  WHERE tenant = p_tenant AND owner_sub = p_owner_sub
  FOR UPDATE;
 IF v_mode = 'frozen' AND v_epoch = p_epoch THEN
  SELECT source_rows INTO v_rows
   FROM hub_private_storage_states
   WHERE tenant = p_tenant AND owner_sub = p_owner_sub;
  RETURN v_rows;
 END IF;
 IF v_mode NOT IN ('legacy', 'building') OR (v_mode = 'building' AND v_epoch IS DISTINCT FROM p_epoch) THEN
  RAISE EXCEPTION USING ERRCODE = 'PST01', MESSAGE = 'private_storage_scope_immutable';
 END IF;
 IF v_mode = 'legacy' THEN
  UPDATE hub_private_storage_states
   SET mode = 'building', epoch = p_epoch, started_at = clock_timestamp(), updated_at = now()
   WHERE tenant = p_tenant AND owner_sub = p_owner_sub;
 END IF;
 WITH source_rows AS (
  SELECT 'notification'::text AS kind,
         n.id::text AS id,
         to_jsonb(n) - 'tenant' AS payload
    FROM notifications n
   WHERE n.tenant = p_tenant AND n.subject = p_owner_sub
  UNION ALL
  SELECT 'preference'::text AS kind,
         encode(digest(convert_to(p.source || ':' || p.kind, 'UTF8'), 'sha256'), 'hex') AS id,
         to_jsonb(p) - 'tenant' AS payload
    FROM notification_prefs p
   WHERE p.tenant = p_tenant AND p.subject = p_owner_sub
 )
 INSERT INTO hub_private_storage_snapshot (tenant, owner_sub, epoch, kind, id, payload)
  SELECT p_tenant, p_owner_sub, p_epoch, kind, id, convert_to(payload::text, 'UTF8')
    FROM source_rows;
 GET DIAGNOSTICS v_rows = ROW_COUNT;
 IF v_mode = 'legacy' THEN
  UPDATE hub_private_storage_states
   SET mode = 'frozen', source_rows = v_rows, source_blobs = 0, updated_at = now()
   WHERE tenant = p_tenant AND owner_sub = p_owner_sub AND epoch = p_epoch AND mode = 'building';
 END IF;
 RETURN v_rows;
END;
$$;
