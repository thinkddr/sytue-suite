-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte índices del reconciliador, torn_down_at y políticas de mantenimiento.

DROP INDEX IF EXISTS deployments_ready_prod_idx;
DROP INDEX IF EXISTS deployments_teardown_idx;

ALTER TABLE deployments DROP COLUMN IF EXISTS torn_down_at;

DROP POLICY IF EXISTS domains_maintenance ON domains;
DROP POLICY IF EXISTS managed_services_maintenance ON managed_services;
DROP POLICY IF EXISTS projects_maintenance ON projects;
