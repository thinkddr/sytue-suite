-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Bajarla retira lo compartido con personas: vuelve a verlo solo quien lo veía por la
-- visibilidad del cuaderno. La edición concedida (notebook_editores) se queda.
DROP TABLE IF EXISTS note_personas;
DROP TABLE IF EXISTS notebook_lectores;
