-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Quita la valla de escritura de la fuente privada de Bases.
DROP TRIGGER IF EXISTS bases_private_links_guard ON base_links;
DROP TRIGGER IF EXISTS bases_private_records_guard ON base_records;
DROP TRIGGER IF EXISTS bases_private_views_guard ON base_views;
DROP TRIGGER IF EXISTS bases_private_fields_guard ON base_fields;
DROP TRIGGER IF EXISTS bases_private_tables_guard ON base_tables;
DROP TRIGGER IF EXISTS bases_private_permissions_guard ON base_permissions;
DROP TRIGGER IF EXISTS bases_private_bases_guard ON bases;
DROP FUNCTION IF EXISTS bases_private_row_guard();
DROP FUNCTION IF EXISTS bases_private_owner_guard(text, text, uuid, boolean);
