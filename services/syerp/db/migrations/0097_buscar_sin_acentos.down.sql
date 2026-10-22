-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

CREATE INDEX IF NOT EXISTS records_etiqueta_trgm ON records USING gin (etiqueta gin_trgm_ops);
DROP INDEX IF EXISTS records_etiqueta_plegado_trgm;
DROP INDEX IF EXISTS records_ref_plegado_trgm;

-- plegar() y unaccent se quedan: la versión anterior no los usa y retirarlos
-- obligaría a soltar antes cualquier índice que dependa de ellos.
