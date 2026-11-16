-- SPDX-License-Identifier: Apache-2.0
-- Retained commitments belong to resumable import; ordinary rollback cannot
-- remove the only proof identifying an accepted encrypted source plan.
-- Accept locks this parent before the inventory. Lock it before inspecting
-- accepted plans, so a commit cannot land between the guard and DROP.
LOCK TABLE photos_private_storage_preflights IN ACCESS EXCLUSIVE MODE;
SET LOCAL row_security=off;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM photos_private_storage_preflights WHERE plan_event_id IS NOT NULL) THEN
  RAISE EXCEPTION USING ERRCODE='PST01',MESSAGE='private_storage_source_immutable';
 END IF;
END $$;
DROP TABLE photos_private_storage_preflight_inventory;
