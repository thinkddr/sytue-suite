-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Listado admin de usuarios (Console) con paginación keyset (created_at, id)
-- DESC: sin este índice el ORDER BY + WHERE (created_at, id) < ($ts, $id)
-- degeneraría en Seq Scan + Sort a cientos de miles de usuarios. El índice
-- compuesto sirve ambos como Index Scan (sin OFFSET).

CREATE INDEX IF NOT EXISTS users_created_at_id_desc_idx ON users (created_at DESC, id DESC);
