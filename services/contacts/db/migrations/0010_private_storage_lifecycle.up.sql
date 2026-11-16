-- SPDX-License-Identifier: Apache-2.0
-- Install the shared predicate and lifecycle check on existing deployments too.
-- sytue:private-storage-fence-v1
CREATE TRIGGER contacts_private_state_scope_guard
 BEFORE UPDATE OR DELETE ON contacts_private_storage_states
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
CREATE TRIGGER contacts_private_snapshot_immutable
 BEFORE UPDATE OR DELETE ON contacts_private_storage_snapshot
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_snapshot_immutable();
CREATE POLICY contacts_private_storage_states_platform_merge ON contacts_private_storage_states
 USING (current_setting('app.merge',true)='on')
 WITH CHECK (current_setting('app.merge',true)='on');
CREATE POLICY contacts_private_storage_snapshot_platform_merge ON contacts_private_storage_snapshot
 USING (current_setting('app.merge',true)='on')
 WITH CHECK (current_setting('app.merge',true)='on');
