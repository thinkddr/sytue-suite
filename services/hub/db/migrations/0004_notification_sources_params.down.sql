-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El CHECK de `source` no se repone: con avisos de Rooms o Docus ya guardados, la
-- vuelta atrás fallaría, y el origen lo sigue validando el dominio.
ALTER TABLE notifications DROP COLUMN IF EXISTS params;
