-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE users  DROP COLUMN location;
ALTER TABLE users  DROP COLUMN title;
ALTER TABLE users  DROP COLUMN department;
ALTER TABLE groups DROP COLUMN membership_rule;
