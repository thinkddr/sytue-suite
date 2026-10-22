-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Copia server-side por REFERENCIA: «Hacer una copia» de un fichero (o carpeta,
-- recursiva) crea filas de metadatos NUEVAS que apuntan al MISMO blob (storage_key)
-- que el original, sin duplicar bytes en S3 (O(1) en bytes). Como consecuencia,
-- varios ficheros VIVOS pueden compartir un blob (antes solo lo hacían un fichero y
-- sus propias versiones, vía RestoreVersion). Al PURGAR uno, el servicio ya no debe
-- borrar el blob mientras otra fila (fichero vivo/en papelera o versión) lo
-- referencie; ese chequeo de referencia —y una futura recolección de blobs
-- huérfanos— barre por storage_key, así que lo indexamos por (tenant, storage_key).
CREATE INDEX files_storage_key_idx ON files (tenant, storage_key);
CREATE INDEX file_versions_storage_key_idx ON file_versions (tenant, storage_key);
