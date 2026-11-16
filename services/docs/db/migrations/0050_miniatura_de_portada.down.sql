-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
ALTER TABLE documents DROP COLUMN IF EXISTS thumbnail_seq;
ALTER TABLE documents DROP COLUMN IF EXISTS thumbnail_key;
