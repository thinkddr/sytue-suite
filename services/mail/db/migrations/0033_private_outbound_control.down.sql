-- SPDX-License-Identifier: Apache-2.0
-- Dropping a controlled generation could make a stale device send before a
-- newer encrypted schedule. CHECK validation scans every row even under RLS;
-- downgrading requires explicitly retiring all controlled receipts first.
ALTER TABLE mail_private_submissions ADD CONSTRAINT mail_private_submission_downgrade_guard CHECK(control_generation=0 AND NOT canceled);
ALTER TABLE mail_private_submission_recipients DROP CONSTRAINT mail_private_submission_recipients_state_check;
ALTER TABLE mail_private_submission_recipients ADD CONSTRAINT mail_private_submission_recipients_state_check CHECK(state IN ('ready','sending','accepted','failed','uncertain'));
ALTER TABLE mail_private_submissions DROP CONSTRAINT mail_private_submission_downgrade_guard, DROP CONSTRAINT mail_private_submission_control_pair, DROP COLUMN control_generation, DROP COLUMN control_commitment, DROP COLUMN canceled;
