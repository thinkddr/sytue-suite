-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Se revierte solo el defecto de las nuevas organizaciones. No se vuelven a
-- escribir las existentes: «app» puede ser ya una elección del administrador.
ALTER TABLE organizations ALTER COLUMN color SET DEFAULT 'petroleo';
