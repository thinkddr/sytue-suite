-- SPDX-License-Identifier: Apache-2.0
ALTER TABLE channel_members
  DROP COLUMN cleared_before,
  DROP COLUMN hidden_at;
