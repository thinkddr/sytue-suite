-- SPDX-License-Identifier: Apache-2.0
-- Retained legacy source only. Devices publish and verify its E2E replacement;
-- neither this migration nor a source read deletes the original application data.
CREATE TABLE erp_private_storage_states (
 tenant text NOT NULL,
 owner_sub text NOT NULL CHECK (owner_sub <> ''),
 mode text NOT NULL DEFAULT 'legacy' CHECK (mode IN ('legacy', 'building', 'frozen', 'private')),
 epoch uuid,
 started_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now(),
 source_rows bigint NOT NULL DEFAULT 0 CHECK (source_rows >= 0),
 source_blobs bigint NOT NULL DEFAULT 0 CHECK (source_blobs >= 0 AND source_blobs <= source_rows),
 PRIMARY KEY (tenant, owner_sub),
 CHECK ((mode = 'legacy' AND epoch IS NULL AND started_at IS NULL) OR
        (mode <> 'legacy' AND epoch IS NOT NULL AND started_at IS NOT NULL))
);
ALTER TABLE erp_private_storage_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE erp_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY erp_private_state_owner ON erp_private_storage_states
 USING (tenant = current_setting('app.tenant', true) AND owner_sub = current_setting('app.owner_sub', true))
 WITH CHECK (tenant = current_setting('app.tenant', true) AND owner_sub = current_setting('app.owner_sub', true));
CREATE POLICY erp_private_state_maintenance ON erp_private_storage_states
 USING (current_setting('app.maintenance', true) = 'on')
 WITH CHECK (current_setting('app.maintenance', true) = 'on');
CREATE POLICY erp_private_state_merge ON erp_private_storage_states
 USING (current_setting('app.merge', true) = 'on')
 WITH CHECK (current_setting('app.merge', true) = 'on');

CREATE TABLE erp_private_storage_snapshot (
 tenant text NOT NULL,
 owner_sub text NOT NULL,
 epoch uuid NOT NULL,
 kind text COLLATE "C" NOT NULL CHECK (kind IN ('record', 'activity')),
 id text COLLATE "C" NOT NULL,
 payload bytea NOT NULL CHECK (octet_length(payload) BETWEEN 2 AND 33554432),
 size_bytes bigint GENERATED ALWAYS AS (octet_length(payload)::bigint) STORED,
 sha256 text GENERATED ALWAYS AS (encode(digest(payload, 'sha256'), 'hex')) STORED,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant, owner_sub, epoch, kind, id),
 FOREIGN KEY (tenant, owner_sub) REFERENCES erp_private_storage_states(tenant, owner_sub)
);
ALTER TABLE erp_private_storage_snapshot ENABLE ROW LEVEL SECURITY;
ALTER TABLE erp_private_storage_snapshot FORCE ROW LEVEL SECURITY;
CREATE POLICY erp_private_snapshot_owner ON erp_private_storage_snapshot FOR SELECT
 USING (tenant = current_setting('app.tenant', true) AND owner_sub = current_setting('app.owner_sub', true));
CREATE POLICY erp_private_snapshot_build ON erp_private_storage_snapshot FOR INSERT
 WITH CHECK (tenant = current_setting('app.tenant', true) AND owner_sub = current_setting('app.owner_sub', true)
  AND EXISTS (SELECT 1 FROM erp_private_storage_states s WHERE s.tenant = erp_private_storage_snapshot.tenant
   AND s.owner_sub = erp_private_storage_snapshot.owner_sub AND s.epoch = erp_private_storage_snapshot.epoch AND s.mode = 'building'));
CREATE POLICY erp_private_snapshot_maintenance ON erp_private_storage_snapshot FOR SELECT
 USING (current_setting('app.maintenance', true) = 'on');
CREATE POLICY erp_private_snapshot_merge ON erp_private_storage_snapshot
 USING (current_setting('app.merge', true) = 'on')
 WITH CHECK (current_setting('app.merge', true) = 'on');

-- sytue:private-storage-fence-v1
CREATE CONSTRAINT TRIGGER erp_private_build_completed
 AFTER INSERT OR UPDATE ON erp_private_storage_states DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_build_completed();

CREATE TRIGGER erp_private_state_immutable BEFORE UPDATE OR DELETE ON erp_private_storage_states
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
CREATE TRIGGER erp_private_snapshot_immutable BEFORE UPDATE OR DELETE ON erp_private_storage_snapshot
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();
ALTER TABLE erp_private_storage_states ENABLE ALWAYS TRIGGER erp_private_state_immutable;
ALTER TABLE erp_private_storage_snapshot ENABLE ALWAYS TRIGGER erp_private_snapshot_immutable;

CREATE OR REPLACE FUNCTION erp_freeze_private_storage(p_tenant text, p_owner_sub text, p_epoch uuid)
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
 INSERT INTO erp_private_storage_states (tenant, owner_sub)
  VALUES (p_tenant, p_owner_sub)
  ON CONFLICT (tenant, owner_sub) DO NOTHING;
 SELECT mode, epoch INTO v_mode, v_epoch
  FROM erp_private_storage_states
  WHERE tenant = p_tenant AND owner_sub = p_owner_sub
  FOR UPDATE;
 IF v_mode = 'frozen' AND v_epoch = p_epoch THEN
  SELECT source_rows INTO v_rows
   FROM erp_private_storage_states
   WHERE tenant = p_tenant AND owner_sub = p_owner_sub;
  RETURN v_rows;
 END IF;
 IF v_mode NOT IN ('legacy', 'building') OR (v_mode = 'building' AND v_epoch IS DISTINCT FROM p_epoch) THEN
  RAISE EXCEPTION USING ERRCODE = 'PST01', MESSAGE = 'private_storage_scope_immutable';
 END IF;
 IF v_mode = 'legacy' THEN
  UPDATE erp_private_storage_states
   SET mode = 'building', epoch = p_epoch, started_at = clock_timestamp(), updated_at = now()
   WHERE tenant = p_tenant AND owner_sub = p_owner_sub;
 END IF;
 WITH source_rows AS (
  SELECT 'record'::text AS kind,
         r.id::text AS id,
         to_jsonb(r) - 'tenant' AS payload
    FROM records r
   WHERE r.tenant = p_tenant
     AND r.deleted_at IS NULL
     AND coalesce(r.data->>'owner_sub', r.data->>'responsable_sub', '') = p_owner_sub
 )
 INSERT INTO erp_private_storage_snapshot (tenant, owner_sub, epoch, kind, id, payload)
  SELECT p_tenant, p_owner_sub, p_epoch, kind, id, convert_to(payload::text, 'UTF8')
    FROM source_rows;
 GET DIAGNOSTICS v_rows = ROW_COUNT;
 IF v_mode = 'legacy' THEN
  UPDATE erp_private_storage_states
   SET mode = 'frozen', source_rows = v_rows, source_blobs = 0, updated_at = now()
   WHERE tenant = p_tenant AND owner_sub = p_owner_sub AND epoch = p_epoch AND mode = 'building';
 END IF;
 RETURN v_rows;
END;
$$;

-- sytue:scope-bound-merge-v1
SELECT sytue_scope_merge_register('erp_private_storage_snapshot'::regclass);

-- sytue:scope-admission-v1
SELECT sytue_scope_admission_register_existing();
