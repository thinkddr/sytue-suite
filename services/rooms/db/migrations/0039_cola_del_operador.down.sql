-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP POLICY IF EXISTS message_reports_maintenance ON message_reports;
ALTER TABLE message_reports DROP COLUMN IF EXISTS resolved_from;
