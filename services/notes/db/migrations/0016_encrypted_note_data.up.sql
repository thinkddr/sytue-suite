-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- The ciphertext now includes the existing note attachments, not only its text.
ALTER TABLE notes DROP CONSTRAINT note_cipher_len;
ALTER TABLE notes ADD CONSTRAINT note_cipher_len CHECK (octet_length(content_cipher) <= 16777216);
CREATE INDEX note_page_idx ON notes (tenant, notebook_id, id) WHERE deleted_at IS NULL;
