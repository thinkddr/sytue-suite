-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE alert_policies   NO FORCE ROW LEVEL SECURITY;
ALTER TABLE alert_deliveries NO FORCE ROW LEVEL SECURITY;
ALTER TABLE uptime_checks    NO FORCE ROW LEVEL SECURITY;
