-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM resource_e2e_public_evidence) OR EXISTS(SELECT 1 FROM resource_e2e_resources) OR EXISTS(SELECT 1 FROM resource_e2e_candidates) OR EXISTS(SELECT 1 FROM resource_e2e_receipts) THEN RAISE EXCEPTION 'cannot roll back retained encrypted resource history' USING ERRCODE='PST02';END IF;
END $$;
DROP TABLE resource_e2e_public_evidence,resource_e2e_receipts,resource_e2e_candidates,resource_e2e_epochs,resource_e2e_resources CASCADE;
DROP FUNCTION sytue_resource_e2e_receipt_guard(),sytue_resource_e2e_resource_guard(),sytue_resource_e2e_candidate_guard(),sytue_resource_e2e_immutable(),sytue_resource_e2e_ciphertext_only(bytea);
