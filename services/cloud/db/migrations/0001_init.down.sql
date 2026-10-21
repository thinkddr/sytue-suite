-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte el esquema base de Sytue Cloud.

DROP FUNCTION IF EXISTS cloud_resolve_repo_tenant(text);

DROP TABLE IF EXISTS events_outbox;
DROP TABLE IF EXISTS git_webhooks;
DROP TABLE IF EXISTS usage_records;
DROP TABLE IF EXISTS managed_services;
DROP TABLE IF EXISTS env_vars;
DROP TABLE IF EXISTS domains;
DROP TABLE IF EXISTS deployments;
DROP TABLE IF EXISTS builds;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS organizations;
