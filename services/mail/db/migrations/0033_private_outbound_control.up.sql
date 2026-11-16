-- SPDX-License-Identifier: Apache-2.0
-- Only opaque generation fences are durable on the relay. Scheduled times,
-- recipients, MIME and the consented outbox control remain inside E2E records.
ALTER TABLE mail_private_submissions
 ADD COLUMN control_generation bigint NOT NULL DEFAULT 0 CHECK(control_generation>=0),
 ADD COLUMN control_commitment text NOT NULL DEFAULT '' CHECK(control_commitment='' OR control_commitment ~ '^[0-9a-f]{64}$'),
 ADD COLUMN canceled boolean NOT NULL DEFAULT false,
 ADD CONSTRAINT mail_private_submission_control_pair CHECK((control_generation=0 AND control_commitment='') OR (control_generation>0 AND control_commitment<>''));
ALTER TABLE mail_private_submission_recipients DROP CONSTRAINT mail_private_submission_recipients_state_check;
ALTER TABLE mail_private_submission_recipients ADD CONSTRAINT mail_private_submission_recipients_state_check CHECK(state IN ('ready','sending','accepted','failed','uncertain','canceled'));
