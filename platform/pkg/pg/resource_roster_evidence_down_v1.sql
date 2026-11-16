-- SPDX-License-Identifier: Apache-2.0
DROP TRIGGER resource_e2e_roster_bootstrap_fence ON resource_e2e_bootstraps;
DROP FUNCTION sytue_resource_e2e_roster_bootstrap_fence();
DROP TRIGGER resource_e2e_roster_capture_evidence_required ON resource_e2e_roster_captures;
DROP FUNCTION sytue_resource_e2e_roster_evidence_required();
ALTER TABLE resource_e2e_roster_captures DROP CONSTRAINT resource_e2e_roster_evidence_shape,
 DROP COLUMN evidence_sha256,DROP COLUMN evidence,DROP COLUMN creator_signing_key,DROP COLUMN creator_encryption_key,DROP COLUMN root_generation;
