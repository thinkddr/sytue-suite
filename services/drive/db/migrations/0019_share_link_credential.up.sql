-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El CHECK del enlace público se quedó pidiendo el secreto EN CLARO.
--
-- 0001 lo escribió como «un enlace siempre trae token»:
--     CHECK (kind <> 'link' OR token IS NOT NULL)
-- Entonces `token` era la única forma de guardar la credencial, así que la
-- restricción y el invariante coincidían. 0018 dejó de escribirla en claro y pasó
-- a guardar solo `token_hash` —esa era su razón de ser— pero NO tocó el CHECK: el
-- «expand» quedó a medias y desde entonces CADA enlace público nuevo muere con
-- 23514 (`shares_link_token`). No es un test frágil: es que compartir por enlace
-- no funciona en cuanto 0018 está aplicada.
--
-- Aquí NO se relaja el invariante, se reescribe con la columna que hoy lleva la
-- credencial: un share kind='link' sigue estando obligado a traer una —la nueva
-- (hash) o la vieja (claro), porque las filas anteriores a 0018 conservan `token`
-- y el binario N-1 todavía lo escribe durante el relevo—. Lo que deja de valer,
-- igual que antes, es un enlace SIN credencial: eso sería una URL pública que
-- abre cualquiera.
--
-- La columna `token` sigue sin borrarse: el «contract» de 0018 no cambia de
-- fecha. Cuando llegue, esta restricción se queda tal cual y basta con quitar el
-- `OR token IS NOT NULL`.

ALTER TABLE shares DROP CONSTRAINT IF EXISTS shares_link_token;
ALTER TABLE shares ADD CONSTRAINT shares_link_token
    CHECK (kind <> 'link' OR token IS NOT NULL OR token_hash IS NOT NULL);
