-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS channel_members_category;
ALTER TABLE channel_members DROP COLUMN IF EXISTS sort_order;
ALTER TABLE channel_members DROP COLUMN IF EXISTS category_id;
DROP TABLE IF EXISTS channel_categories;
