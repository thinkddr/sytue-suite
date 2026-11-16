-- SPDX-License-Identifier: Apache-2.0
-- Durable PRE-epoch phase, while source content remains frozen and retained.
-- sytue:resource-bootstrap-v1
GRANT SELECT,INSERT,UPDATE ON resource_e2e_bootstraps,resource_e2e_bootstrap_slots TO docs_app;
SELECT sytue_scope_merge_register('resource_e2e_bootstraps'::regclass);
SELECT sytue_scope_merge_register('resource_e2e_bootstrap_slots'::regclass);
SELECT sytue_scope_admission_register('resource_e2e_bootstraps'::regclass,false);
SELECT sytue_scope_admission_register('resource_e2e_bootstrap_slots'::regclass,false);
