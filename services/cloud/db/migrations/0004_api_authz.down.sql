-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0004_api_authz (orden inverso).

DROP POLICY IF EXISTS memberships_tenant_isolation ON memberships;
DROP POLICY IF EXISTS api_tokens_maintenance ON api_tokens;
DROP POLICY IF EXISTS api_tokens_tenant_isolation ON api_tokens;

DROP INDEX IF EXISTS platform_rate_limits_updated_at_idx;
DROP TABLE IF EXISTS platform_rate_limits;

ALTER TABLE projects DROP COLUMN IF EXISTS webhook_secret_enc;

DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS api_tokens;
