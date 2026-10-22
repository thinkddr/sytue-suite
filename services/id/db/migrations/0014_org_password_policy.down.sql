-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
ALTER TABLE organizations
	DROP COLUMN IF EXISTS password_require_complexity,
	DROP COLUMN IF EXISTS password_min_length;
