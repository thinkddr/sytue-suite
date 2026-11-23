-- SPDX-License-Identifier: Apache-2.0
-- Vuelta al disparador v1. Una fuente soltada es legacy y vale para v1.
SELECT sytue_private_storage_release_uninstall('public.erp_private_storage_states'::regclass);
DROP FUNCTION IF EXISTS sytue_private_storage_release_uninstall(regclass);
DROP FUNCTION IF EXISTS sytue_private_storage_release_install(regclass);
DROP FUNCTION IF EXISTS sytue_private_storage_release(regclass,text,text,uuid,timestamptz,text[]);
DROP FUNCTION IF EXISTS sytue_private_storage_state_scope_guard_v2();
DROP FUNCTION IF EXISTS sytue_private_storage_release_key(text,text,text,uuid);
