-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira SOLO la de `organizacion`: las otras siete las puso la 0003 y las quita
-- su propia bajada. Deshacer esta migración no debe dejar la fusión rota para las
-- tablas que ya funcionaban antes de que existiera.

DROP POLICY IF EXISTS organizacion_platform_merge ON organizacion;
