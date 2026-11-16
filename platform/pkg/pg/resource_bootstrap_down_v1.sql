-- SPDX-License-Identifier: Apache-2.0
DO $$BEGIN
 IF EXISTS(SELECT 1 FROM resource_e2e_bootstraps)OR EXISTS(SELECT 1 FROM resource_e2e_bootstrap_slots)THEN RAISE EXCEPTION 'cannot discard retained bootstrap evidence' USING ERRCODE='PST02';END IF;
END $$;
DROP TRIGGER resource_e2e_bootstrap_complete ON resource_e2e_resources;
DROP FUNCTION sytue_resource_e2e_bootstrap_complete();
DROP TABLE resource_e2e_bootstrap_slots,resource_e2e_bootstraps;
DROP FUNCTION sytue_resource_e2e_bootstrap_guard(),sytue_resource_e2e_bootstrap_slot_guard();
