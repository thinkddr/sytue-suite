-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Al revertir se pierden las invitaciones pendientes: quien no las había aceptado se
-- queda fuera y hay que volver a añadirle.
DROP TABLE IF EXISTS invitaciones_canal;
