-- SPDX-License-Identifier: Apache-2.0
-- Signed public PENDING capture, no keys/epoch/routing permit.
-- sytue:resource-roster-capture-v1
GRANT SELECT,INSERT,UPDATE ON resource_e2e_roster_captures TO docs_app;
GRANT SELECT,INSERT ON resource_e2e_roster_pages TO docs_app;
SELECT sytue_scope_merge_register('resource_e2e_roster_captures'::regclass);
SELECT sytue_scope_merge_register('resource_e2e_roster_pages'::regclass);
SELECT sytue_scope_admission_register('resource_e2e_roster_captures'::regclass,false);
SELECT sytue_scope_admission_register('resource_e2e_roster_pages'::regclass,false);
