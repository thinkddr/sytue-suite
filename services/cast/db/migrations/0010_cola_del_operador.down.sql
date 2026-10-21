-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP POLICY IF EXISTS reports_maintenance ON reports;
ALTER TABLE reports DROP COLUMN IF EXISTS resolved_from;
