-- SPDX-License-Identifier: Apache-2.0
SELECT sytue_scope_merge_unregister(ARRAY['matrix_draft_rooms'::regclass,'matrix_storage_authorities'::regclass,'matrix_storage_authority_history'::regclass,'matrix_storage_cutovers'::regclass,'matrix_storage_isolated_rooms'::regclass,'matrix_storage_rooms'::regclass,'matrix_storage_root_consents'::regclass]);
DROP TRIGGER matrix_storage_mapping_scope_immutable ON matrix_storage_rooms;
DROP TRIGGER matrix_storage_mapping_scope_immutable ON matrix_storage_isolated_rooms;
DROP TRIGGER matrix_storage_mapping_scope_immutable ON matrix_draft_rooms;
DROP FUNCTION matrix_storage_mapping_scope_immutable();
