-- SPDX-License-Identifier: Apache-2.0
-- Existing source/schema remain unchanged. Install the common implementation
-- without changing the already published 0035/0036 migration contents.
-- sytue:private-storage-fence-v1
CREATE OR REPLACE FUNCTION drive_private_storage_legacy_guard(t text,o text)
RETURNS void LANGUAGE plpgsql SET search_path=pg_catalog,public AS $$
BEGIN
 IF o IS NULL OR o='' THEN RETURN; END IF;
 PERFORM public.sytue_private_storage_guard('public.drive_private_storage_states'::regclass,t,o);
END;
$$;
-- Keep the published deferred constraint active for older replicas. The common
-- completion check is additive; regular triggers retain their names and replace
-- only the implementation, inside the migration transaction.
CREATE CONSTRAINT TRIGGER drive_private_storage_shared_build_completed
 AFTER INSERT OR UPDATE ON drive_private_storage_states DEFERRABLE INITIALLY DEFERRED
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_build_completed();
CREATE OR REPLACE TRIGGER drive_private_storage_state_scope_guard BEFORE UPDATE OR DELETE ON drive_private_storage_states
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
CREATE OR REPLACE TRIGGER drive_private_storage_snapshot_immutable BEFORE UPDATE OR DELETE ON drive_private_storage_snapshot
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();
CREATE OR REPLACE TRIGGER drive_private_storage_source_immutable BEFORE UPDATE OR DELETE ON drive_private_storage_source_blobs
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();
