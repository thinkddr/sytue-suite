-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- Refuse a rollback while sealed attachments exceed the old bound; never truncate.
ALTER TABLE notes DROP CONSTRAINT note_cipher_len;
ALTER TABLE notes ADD CONSTRAINT note_cipher_len CHECK (length(content_cipher) <= 400000);
DROP INDEX note_page_idx;
