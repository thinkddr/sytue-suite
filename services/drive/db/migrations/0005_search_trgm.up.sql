-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Búsqueda por subcadena a escala. SearchFiles filtra con
-- `lower(name) LIKE '%q%'` (comodín inicial), que el índice btree
-- files_name_search_idx (tenant, lower(name)) NO puede servir: un patrón con
-- comodín a la izquierda fuerza un seq-scan de todos los ficheros vivos del
-- tenant. Con cientos de miles de ficheros eso no escala (CONVENTIONS §0.8:
-- índice para TODO WHERE; GIN pg_trgm en búsquedas por texto).
--
-- pg_trgm indexa trigramas de `lower(name)`, y el planificador usa este GIN para
-- LIKE/ILIKE '%...%' cuando el término tiene ≥3 caracteres (para 1-2 caracteres
-- cae a scan, aceptable). Parcial WHERE deleted_at IS NULL: no indexa la
-- papelera (la búsqueda solo mira ficheros vivos).
--
-- ponytail: GIN solo sobre lower(name); el `tenant=$1` se filtra tras el bitmap
-- del índice (+ RLS). Si el perfil lo pide, el paso siguiente es un GIN compuesto
-- (tenant, lower(name)) con la extensión btree_gin.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS files_name_trgm_idx
    ON files USING gin (lower(name) gin_trgm_ops)
    WHERE deleted_at IS NULL;
