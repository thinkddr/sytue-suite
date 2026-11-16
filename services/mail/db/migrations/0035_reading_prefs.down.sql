-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS mail_trusted_senders;
ALTER TABLE accounts
  DROP COLUMN IF EXISTS swipe_left,
  DROP COLUMN IF EXISTS swipe_right,
  DROP COLUMN IF EXISTS auto_advance;
