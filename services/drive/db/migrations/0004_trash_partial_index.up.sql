-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Índices parciales de la papelera: ListTrash filtra por
-- `tenant = $1 AND deleted_at IS NOT NULL` y ordena por deleted_at DESC, que
-- los índices existentes (todos WHERE deleted_at IS NULL) no cubren — era un
-- seq-scan de la tabla completa por cada listado de papelera.
--
-- Nota: el índice keyset de ficheros por carpeta, (tenant, folder_id,
-- lower(name)) WHERE deleted_at IS NULL, ya existe desde 0001 como el índice
-- ÚNICO files_unique_name_in_folder; ListFilesPage lo aprovecha tal cual (la
-- unicidad de lower(name) por carpeta hace trivial el desempate por id).

CREATE INDEX folders_trash_idx
    ON folders (tenant, deleted_at)
    WHERE deleted_at IS NOT NULL;

CREATE INDEX files_trash_idx
    ON files (tenant, deleted_at)
    WHERE deleted_at IS NOT NULL;
