-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS mfa_continuations;
COMMENT ON COLUMN totp_secrets.secret IS NULL;
DROP INDEX IF EXISTS signing_keys_next_demoted_idx;
ALTER TABLE signing_keys DROP COLUMN IF EXISTS demoted_at;
COMMENT ON COLUMN signing_keys.private_pem IS NULL;
