-- SPDX-License-Identifier: Apache-2.0
-- A populated retained migration cannot be silently discarded by a rollback.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM contacts_private_storage_states WHERE mode<>'legacy') THEN
  RAISE EXCEPTION 'contacts_private_storage_retained_source_exists';
 END IF;
END $$;
DROP TRIGGER contacts_private_books_guard ON address_books;
DROP TRIGGER contacts_private_cards_guard ON card_objects;
DROP TRIGGER contacts_private_people_guard ON contacts;
DROP TRIGGER contacts_private_emails_guard ON contact_emails;
DROP TRIGGER contacts_private_phones_guard ON contact_phones;
DROP TRIGGER contacts_private_groups_guard ON contact_groups;
DROP TRIGGER contacts_private_members_guard ON contact_group_members;
DROP FUNCTION contacts_private_row_guard();
DROP TABLE contacts_private_storage_snapshot;
DROP TABLE contacts_private_storage_states;
