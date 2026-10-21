-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira la política de mantenimiento y los índices de papelera. El aislamiento
-- por organización no depende de ellos: lo que se pierde es que el worker de
-- retención pueda purgar, y volvería a no borrar nada —en silencio, como antes—.
--
-- Si se vuelve atrás con el worker todavía en marcha, conviene apagarlo también
-- (`TRASH_RETENTION=0`): si no, seguirá despertándose cada hora para purgar cero
-- filas y anotándolo en el log.

DROP POLICY IF EXISTS notes_mantenimiento     ON notes;
DROP POLICY IF EXISTS notebooks_mantenimiento ON notebooks;

DROP INDEX IF EXISTS notes_papelera_idx;
DROP INDEX IF EXISTS notebooks_papelera_idx;
