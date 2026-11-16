-- SPDX-License-Identifier: Apache-2.0
-- Receipts contain only immutable request commitments and delivery state.
-- No subject, recipients, headers, MIME, attachment or SMTP response is retained.
CREATE TABLE mail_private_submissions (
 tenant text NOT NULL,
 owner_sub text NOT NULL CHECK(owner_sub<>''),
 epoch uuid NOT NULL,
 operation uuid NOT NULL,
 fingerprint text NOT NULL CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
 recipient_count integer NOT NULL CHECK(recipient_count BETWEEN 1 AND 100),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,owner_sub,epoch,operation),
 FOREIGN KEY(tenant,owner_sub) REFERENCES mail_private_storage_states(tenant,owner_sub)
);
CREATE TABLE mail_private_submission_recipients (
 tenant text NOT NULL,
 owner_sub text NOT NULL,
 epoch uuid NOT NULL,
 operation uuid NOT NULL,
 ordinal integer NOT NULL CHECK(ordinal BETWEEN 0 AND 99),
 state text NOT NULL DEFAULT 'ready' CHECK(state IN ('ready','sending','accepted','failed','uncertain')),
 PRIMARY KEY(tenant,owner_sub,epoch,operation,ordinal),
 FOREIGN KEY(tenant,owner_sub,epoch,operation) REFERENCES mail_private_submissions(tenant,owner_sub,epoch,operation)
);
ALTER TABLE mail_private_submissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_private_submissions FORCE ROW LEVEL SECURITY;
ALTER TABLE mail_private_submission_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_private_submission_recipients FORCE ROW LEVEL SECURITY;
CREATE POLICY mail_private_submission_owner ON mail_private_submissions
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_mail_owner',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_mail_owner',true));
CREATE POLICY mail_private_submission_recipient_owner ON mail_private_submission_recipients
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_mail_owner',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.private_mail_owner',true));
SELECT sytue_scope_merge_register('mail_private_submissions'::regclass);
SELECT sytue_scope_merge_register('mail_private_submission_recipients'::regclass);
SELECT sytue_scope_admission_register('mail_private_submissions'::regclass,false);
SELECT sytue_scope_admission_register('mail_private_submission_recipients'::regclass,false);
