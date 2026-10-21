-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0010_uptime_alerts (orden inverso: dependencias FK primero).

DROP POLICY IF EXISTS alert_deliveries_maintenance ON alert_deliveries;
DROP POLICY IF EXISTS alert_deliveries_tenant_isolation ON alert_deliveries;
DROP POLICY IF EXISTS alert_policies_tenant_isolation ON alert_policies;
DROP POLICY IF EXISTS uptime_checks_maintenance ON uptime_checks;
DROP POLICY IF EXISTS uptime_checks_tenant_isolation ON uptime_checks;

DROP TABLE IF EXISTS alert_deliveries;
DROP TABLE IF EXISTS alert_policies;
DROP TABLE IF EXISTS uptime_checks;
