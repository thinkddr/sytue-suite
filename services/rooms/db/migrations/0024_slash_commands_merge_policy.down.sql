-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira la política de fusión. El aislamiento por tenant no depende de ella: lo
-- aplica slash_commands_tenant_isolation (0023).

DROP POLICY IF EXISTS slash_commands_platform_merge ON slash_commands;
