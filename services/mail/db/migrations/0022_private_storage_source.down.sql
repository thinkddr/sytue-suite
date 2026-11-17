-- SPDX-License-Identifier: Apache-2.0
-- A retained source must never be discarded by an automatic downgrade.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM mail_private_storage_states WHERE mode<>'legacy') THEN
  RAISE EXCEPTION 'mail_private_source_requires_verified_retirement';
 END IF;
END $$;
DROP TRIGGER mail_private_accounts_guard ON accounts;
DROP TRIGGER mail_private_mailboxes_guard ON mailboxes;
DROP TRIGGER mail_private_threads_guard ON threads;
DROP TRIGGER mail_private_messages_guard ON messages;
DROP TRIGGER mail_private_outbound_guard ON outbound;
DROP TRIGGER mail_private_labels_guard ON mail_labels;
DROP TRIGGER mail_private_filters_guard ON mail_filters;
DROP TRIGGER mail_private_vacation_guard ON mail_vacation;
DROP TRIGGER mail_private_vacation_log_guard ON mail_vacation_log;
DROP TRIGGER mail_private_acl_guard ON mailbox_acl;
DROP TRIGGER mail_private_requests_guard ON mailbox_peticiones_edicion;
DROP TRIGGER mail_private_aliases_guard ON aliases;
DROP TRIGGER mail_private_smime_guard ON smime_identities;
DROP FUNCTION mail_private_storage_row_guard();
DROP FUNCTION mail_private_storage_guard(text,uuid);
DROP FUNCTION mail_private_storage_retains_blob(text);
DROP FUNCTION mail_private_storage_frozen_account(text,uuid);
DROP TABLE mail_private_storage_source;
DROP TABLE mail_private_storage_states;
-- The common v1 functions can be used by other domain migrations; retain them.
