-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Vuelve a exigir el token en claro. ⚠️ Los enlaces creados mientras 0019 estaba
-- aplicada solo tienen hash, así que esta reversión FALLA si existe alguno: es
-- deliberado —borrar esas filas para que la restricción entre sería tirar
-- enlaces vivos sin avisar—. Para revertir de verdad hay que ir antes a 0018,
-- que sí repuebla `token`... y tampoco puede recuperarlo de su hash.

ALTER TABLE shares DROP CONSTRAINT IF EXISTS shares_link_token;
ALTER TABLE shares ADD CONSTRAINT shares_link_token
    CHECK (kind <> 'link' OR token IS NOT NULL);
