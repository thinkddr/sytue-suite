-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0003_governance: suelta las tablas de gobernanza de egress (sus políticas
-- RLS y grants caen con ellas). El rol integrations_app lo posee/retira 0001.

DROP TABLE IF EXISTS policy_decisions;
DROP TABLE IF EXISTS policies;
