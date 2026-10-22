-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira la política de fusión. El aislamiento por tenant no depende de ella: lo
-- aplica message_drafts_tenant_isolation (0026).

DROP POLICY IF EXISTS message_drafts_platform_merge ON message_drafts;
