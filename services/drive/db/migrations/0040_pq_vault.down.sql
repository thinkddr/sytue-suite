-- SPDX-License-Identifier: Apache-2.0
SELECT sytue_scope_merge_unregister(ARRAY['pq_vault_records'::regclass,'pq_vault_keys'::regclass,'pq_vaults'::regclass]);
DROP FUNCTION sytue_scope_merge_unregister(regclass[]);
DROP FUNCTION sytue_scope_merge_register(regclass);
DROP FUNCTION sytue_scope_merge_preflight(text);
DROP FUNCTION sytue_scope_merge_allowed();
DROP TABLE IF EXISTS pq_vault_records;
DROP TABLE IF EXISTS pq_vault_keys;
DROP TABLE IF EXISTS pq_vaults;
