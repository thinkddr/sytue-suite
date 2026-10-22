-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

-- Bajarla deja a quien editaba en solo ver; lo ya copiado sigue siendo de la dueña.
ALTER TABLE album_personas DROP COLUMN IF EXISTS puede_editar;
