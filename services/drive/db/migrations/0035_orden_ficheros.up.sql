-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Índices del orden en el servicio (`?orden=modificado|tamano&sentido=` en
-- GET /folders/{id}/children). ListFilesPage pagina por keyset con la tupla
-- (columna, id) dentro de una carpeta; sin un índice que empiece por
-- (tenant, folder_id, columna) cada página de una carpeta grande ordenaría TODOS
-- sus ficheros para quedarse con 200. El sentido descendente recorre el mismo
-- índice hacia atrás, así que no hace falta uno por sentido.
--
-- El orden por nombre ya tiene el suyo: files_unique_name_in_folder (0001), que
-- es único por carpeta y hace trivial el desempate por id.

CREATE INDEX files_folder_updated_idx
    ON files (tenant, folder_id, updated_at, id)
    WHERE deleted_at IS NULL;

CREATE INDEX files_folder_size_idx
    ON files (tenant, folder_id, size_bytes, id)
    WHERE deleted_at IS NULL;
