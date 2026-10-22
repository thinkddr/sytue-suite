-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP POLICY IF EXISTS quotas_tenant_isolation ON quotas;
DROP POLICY IF EXISTS shares_tenant_isolation ON shares;
DROP POLICY IF EXISTS files_tenant_isolation ON files;
DROP POLICY IF EXISTS folders_tenant_isolation ON folders;

DROP TABLE IF EXISTS events_outbox;
DROP TABLE IF EXISTS quotas;
DROP TABLE IF EXISTS shares;
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS folders;
