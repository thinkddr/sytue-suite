-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Lo que quede en la cola deja de llegar a Docs: la papelera de cada uno vuelve a
-- ir por su cuenta, que es como estaba antes de 0027.
DROP TRIGGER IF EXISTS files_docs_sync_purga ON files;
DROP TRIGGER IF EXISTS files_docs_sync_papelera ON files;
DROP FUNCTION IF EXISTS drive_docs_sync_encolar();
DROP TABLE IF EXISTS docs_sync_pending;
