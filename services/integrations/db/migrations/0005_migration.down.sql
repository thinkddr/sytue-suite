-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0005_migration: suelta las tablas de migración de contenido (sus
-- políticas RLS, índices y grants caen con ellas). El rol integrations_app lo
-- posee/retira 0001.

DROP TABLE IF EXISTS migration_map;
DROP TABLE IF EXISTS migration_jobs;
