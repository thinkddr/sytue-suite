-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM contacts_private_storage_states WHERE mode<>'legacy') THEN
  RAISE EXCEPTION 'contacts_private_storage_retained_source_exists';
 END IF;
END $$;
DROP TRIGGER contacts_private_state_scope_guard ON contacts_private_storage_states;
DROP TRIGGER contacts_private_snapshot_immutable ON contacts_private_storage_snapshot;
DROP POLICY contacts_private_storage_snapshot_platform_merge ON contacts_private_storage_snapshot;
DROP POLICY contacts_private_storage_states_platform_merge ON contacts_private_storage_states;
