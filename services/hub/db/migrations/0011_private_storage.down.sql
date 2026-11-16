-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM hub_private_storage_states WHERE mode <> 'legacy') THEN
  RAISE EXCEPTION 'private Hub migration cannot be downgraded after a source is frozen';
 END IF;
END $$;
SELECT sytue_scope_admission_remove();
DROP TABLE private_scope_admission_control;
DROP FUNCTION sytue_scope_admission_remove();
DROP FUNCTION sytue_scope_admission_register_existing();
DROP FUNCTION sytue_scope_admission_tables();
DROP FUNCTION sytue_scope_admission_transition(uuid, text, text, text);
DROP FUNCTION sytue_scope_admission_register(regclass, boolean);
DROP FUNCTION sytue_scope_admission_data_guard();
DROP FUNCTION sytue_scope_admission_check(text, boolean);
DROP FUNCTION sytue_scope_admission_control_guard();
SELECT sytue_scope_merge_unregister(ARRAY['hub_private_storage_snapshot'::regclass]);
DROP FUNCTION sytue_scope_merge_unregister(regclass[]);
DROP FUNCTION sytue_scope_merge_register(regclass);
DROP FUNCTION sytue_scope_merge_preflight(text);
DROP FUNCTION sytue_scope_merge_allowed();
DROP FUNCTION hub_freeze_private_storage(text, text, uuid);
DROP TABLE hub_private_storage_snapshot;
DROP TABLE hub_private_storage_states;
