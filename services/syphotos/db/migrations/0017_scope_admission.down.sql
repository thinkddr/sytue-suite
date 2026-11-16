-- SPDX-License-Identifier: Apache-2.0
SELECT sytue_scope_admission_remove();
DROP TABLE private_scope_admission_control;
DROP FUNCTION sytue_scope_admission_remove();
DROP FUNCTION sytue_scope_admission_register_existing();
DROP FUNCTION sytue_scope_admission_tables();
DROP FUNCTION sytue_scope_admission_transition(uuid,text,text,text);
DROP FUNCTION sytue_scope_admission_register(regclass,boolean);
DROP FUNCTION sytue_scope_admission_data_guard();
DROP FUNCTION sytue_scope_admission_check(text,boolean);
DROP FUNCTION sytue_scope_admission_control_guard();
SELECT sytue_scope_merge_unregister(ARRAY['photos_private_storage_snapshot'::regclass,'photos_private_storage_preflights'::regclass,'photos_private_storage_preflight_inventory'::regclass]);
DROP TRIGGER photos_private_preflight_scope_immutable ON photos_private_storage_preflights;
DROP FUNCTION photos_private_preflight_scope_immutable();
DROP POLICY assets_platform_merge ON assets;
DROP POLICY albums_platform_merge ON albums;
DROP POLICY edits_platform_merge ON edits;
DROP POLICY album_assets_platform_merge ON album_assets;
ALTER POLICY org_settings_platform_merge ON org_settings USING(current_setting('app.platform_merge',true)='on') WITH CHECK(current_setting('app.platform_merge',true)='on');
