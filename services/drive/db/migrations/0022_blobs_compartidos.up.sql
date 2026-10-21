-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Deduplicación de ficheros ENTRE organizaciones.
--
-- Un fichero recién subido se mueve a una clave direccionada por su SHA-256
-- (`shared/drive/<hash>`, ver objectstore.PrefijoCompartido): dos organizaciones
-- que suben el mismo PDF guardan un objeto. Las filas de `files` y
-- `file_versions` siguen apuntando por `storage_key` como siempre; lo único
-- nuevo es que esa clave puede ser de todos.
--
-- Borrar un blob compartido exige saber si ALGUNA organización lo usa aún, y
-- eso no se puede preguntar bajo el aislamiento por tenant: se pregunta desde
-- el mantenimiento (política `app.maintenance`, migración 0009) con estos
-- índices GLOBALES por clave, sin el tenant delante.
CREATE INDEX files_storage_key_global_idx ON files (storage_key);
CREATE INDEX file_versions_storage_key_global_idx ON file_versions (storage_key);

-- Candidatos a huérfano: claves compartidas que una purga dejó de referenciar.
-- No se borran en el acto: el barrido las mira pasado un margen, comprueba que
-- nadie las use y entonces borra. Sin columna tenant, sin RLS: una clave es un
-- hash, no un dato de nadie.
CREATE TABLE blobs_candidatos (
    storage_key text        PRIMARY KEY,
    desde       timestamptz NOT NULL DEFAULT now()
);
