-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP POLICY IF EXISTS metric_alerts_platform_merge ON metric_alerts;
DROP TABLE IF EXISTS metric_alerts;

-- Devuelve el CHECK al conjunto de la 0033. Si quedara alguna política de consumo,
-- el ALTER fallaría; se borran primero para que el down sea aplicable.
DELETE FROM alert_policies WHERE event IN ('metric.cpu_high', 'metric.memory_high');
ALTER TABLE alert_policies DROP CONSTRAINT IF EXISTS alert_policies_event_check;
ALTER TABLE alert_policies ADD CONSTRAINT alert_policies_event_check
    CHECK (event IN ('deploy.failed', 'deploy.ready', 'uptime.down', 'uptime.up',
                     'billing.spend_threshold', 'sizing.advice'));
