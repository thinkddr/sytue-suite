-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS documents_source_ref_uniq;
ALTER TABLE documents DROP COLUMN IF EXISTS source_ref;
