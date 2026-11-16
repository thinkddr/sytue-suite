-- SPDX-License-Identifier: Apache-2.0
-- sytue:scope-bound-merge-v1
SELECT sytue_scope_merge_register('matrix_storage_authorities'::regclass);
SELECT sytue_scope_merge_register('matrix_storage_authority_history'::regclass);
SELECT sytue_scope_merge_register('matrix_storage_root_consents'::regclass);
SELECT sytue_scope_merge_register('matrix_storage_rooms'::regclass);
SELECT sytue_scope_merge_register('matrix_storage_isolated_rooms'::regclass);
SELECT sytue_scope_merge_register('matrix_storage_cutovers'::regclass);
SELECT sytue_scope_merge_register('matrix_draft_rooms'::regclass);
-- Legacy and isolated routing references already name encrypted records whose
-- signed Scope cannot follow an administrative tenant/owner/room rewrite.
CREATE FUNCTION matrix_storage_mapping_scope_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD IS DISTINCT FROM NEW THEN
  RAISE EXCEPTION 'matrix storage mapping scope is immutable' USING ERRCODE='PST01';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_storage_mapping_scope_immutable BEFORE UPDATE ON matrix_storage_rooms
 FOR EACH ROW EXECUTE FUNCTION matrix_storage_mapping_scope_immutable();
CREATE TRIGGER matrix_storage_mapping_scope_immutable BEFORE UPDATE ON matrix_storage_isolated_rooms
 FOR EACH ROW EXECUTE FUNCTION matrix_storage_mapping_scope_immutable();
CREATE TRIGGER matrix_storage_mapping_scope_immutable BEFORE UPDATE ON matrix_draft_rooms
 FOR EACH ROW EXECUTE FUNCTION matrix_storage_mapping_scope_immutable();
