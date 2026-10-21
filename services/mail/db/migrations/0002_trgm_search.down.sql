-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS messages_subject_trgm_idx;
DROP INDEX IF EXISTS messages_from_trgm_idx;

-- La extensión pg_trgm se conserva: puede estar en uso por otros índices/esquemas
-- de la misma base de datos.
