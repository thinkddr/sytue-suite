-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Sin la política, el barrido no ve nada caducado y no borra nada: la papelera
-- vuelve a no caducar, que es como estaba antes de la 0014.
DROP INDEX IF EXISTS documents_papelera_idx;
DROP POLICY IF EXISTS documents_papelera_caducada ON documents;
