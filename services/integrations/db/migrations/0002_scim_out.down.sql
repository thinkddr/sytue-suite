-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0002_scim_out: suelta las tablas de SCIM saliente (sus políticas RLS y
-- grants caen con ellas). El rol integrations_app lo posee/retira 0001.

DROP TABLE IF EXISTS sync_runs;
DROP TABLE IF EXISTS scim_map;
