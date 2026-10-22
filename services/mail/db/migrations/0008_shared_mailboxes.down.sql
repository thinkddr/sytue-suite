-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP TABLE IF EXISTS mailbox_acl;
ALTER TABLE accounts DROP COLUMN IF EXISTS kind;
