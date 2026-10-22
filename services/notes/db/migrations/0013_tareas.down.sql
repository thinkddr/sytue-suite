-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
DROP INDEX notes_tareas_abiertas;
ALTER TABLE notes DROP COLUMN checked_at, DROP COLUMN cancelled_at;
