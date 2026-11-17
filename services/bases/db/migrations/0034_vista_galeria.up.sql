-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- VISTA GALERÍA: tarjetas con el primer adjunto de imagen del registro como portada.
-- Se configura como la rejilla (filtros y orden), así que solo se amplía el CHECK:
-- ninguna vista existente cambia. (0032 es del rango de la forja, ver AGENTS.md, y 0033 el almacenamiento privado.)
ALTER TABLE base_views DROP CONSTRAINT base_views_kind_check;
ALTER TABLE base_views ADD CONSTRAINT base_views_kind_check
    CHECK (kind IN ('grid', 'kanban', 'form', 'gallery'));
