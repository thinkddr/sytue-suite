-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DROP INDEX IF EXISTS files_name_trgm_idx;
-- La extensión pg_trgm se deja instalada a propósito: es global y otros objetos
-- podrían depender de ella; su presencia es inocua.
