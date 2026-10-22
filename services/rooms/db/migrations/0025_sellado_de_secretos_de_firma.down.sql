-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Quita el camino de mantenimiento. El aislamiento por organización no cambia:
-- era una política PERMISSIVE aparte, no una relajación de la de tenant.

DROP POLICY IF EXISTS slash_commands_maintenance ON slash_commands;
DROP POLICY IF EXISTS outgoing_webhooks_maintenance ON outgoing_webhooks;
