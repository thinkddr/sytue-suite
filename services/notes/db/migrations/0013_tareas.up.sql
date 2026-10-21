-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- Cuándo se completó o se canceló una tarea. Cancelada = tachada y fuera del
-- traslado diario; las dos marcas se excluyen (lo hace cumplir NoteInput.Normalize).
ALTER TABLE notes ADD COLUMN checked_at timestamptz, ADD COLUMN cancelled_at timestamptz;
-- Las ya marcadas no saben cuándo: su última edición es la mejor aproximación.
UPDATE notes SET checked_at = updated_at WHERE checked;
-- GET /tasks?state=open: las pendientes de una persona sin recorrer todas sus notas.
CREATE INDEX notes_tareas_abiertas ON notes (tenant, owner_sub)
    WHERE kind = 'todo' AND NOT checked AND deleted_at IS NULL AND cancelled_at IS NULL;
