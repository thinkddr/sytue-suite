-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0005_deploy_notifications (orden inverso).

DROP POLICY IF EXISTS deploy_notifications_tenant_isolation ON deploy_notifications;
DROP TABLE IF EXISTS deploy_notifications;

ALTER TABLE projects      DROP COLUMN IF EXISTS notify_webhook_url;
ALTER TABLE organizations DROP COLUMN IF EXISTS notify_webhook_url;
