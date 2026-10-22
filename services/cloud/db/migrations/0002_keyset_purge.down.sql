-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte índices keyset/purga y políticas de mantenimiento.

DROP POLICY IF EXISTS usage_records_maintenance ON usage_records;
DROP POLICY IF EXISTS git_webhooks_maintenance ON git_webhooks;
DROP POLICY IF EXISTS deployments_maintenance ON deployments;
DROP POLICY IF EXISTS builds_maintenance ON builds;

DROP INDEX IF EXISTS usage_records_period_from_idx;
DROP INDEX IF EXISTS events_outbox_processed_idx;
DROP INDEX IF EXISTS git_webhooks_received_idx;
DROP INDEX IF EXISTS deployments_retired_preview_idx;
DROP INDEX IF EXISTS builds_terminal_created_idx;

CREATE INDEX deployments_project_idx ON deployments (tenant, project_id, created_at DESC);
DROP INDEX IF EXISTS deployments_project_created_id_idx;

CREATE INDEX builds_project_idx ON builds (tenant, project_id, created_at DESC);
DROP INDEX IF EXISTS builds_project_created_id_idx;
