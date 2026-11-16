-- SPDX-License-Identifier: Apache-2.0
-- Shared opaque resource relay; no application plaintext or epoch private keys.
-- sytue:resource-relay-v1
GRANT SELECT,INSERT ON resource_e2e_resources,resource_e2e_epochs,resource_e2e_candidates,resource_e2e_receipts,resource_e2e_public_evidence TO docs_app;
GRANT UPDATE ON resource_e2e_resources,resource_e2e_candidates TO docs_app;
SELECT sytue_scope_merge_register('resource_e2e_resources'::regclass);
SELECT sytue_scope_merge_register('resource_e2e_epochs'::regclass);
SELECT sytue_scope_merge_register('resource_e2e_candidates'::regclass);
SELECT sytue_scope_merge_register('resource_e2e_receipts'::regclass);
SELECT sytue_scope_merge_register('resource_e2e_public_evidence'::regclass);
SELECT sytue_scope_admission_register('resource_e2e_resources'::regclass,false);
SELECT sytue_scope_admission_register('resource_e2e_epochs'::regclass,false);
SELECT sytue_scope_admission_register('resource_e2e_candidates'::regclass,false);
SELECT sytue_scope_admission_register('resource_e2e_receipts'::regclass,false);
SELECT sytue_scope_admission_register('resource_e2e_public_evidence'::regclass,false);
