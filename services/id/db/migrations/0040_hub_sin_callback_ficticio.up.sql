-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Hub no tiene flujo de navegador: quitarle el callback que nunca existió.
--
-- El seed 0002 le registró `https://hub.<dominio>/auth/callback`, pero Hub es el
-- backend de la shell —una API Go— y esa ruta responde 404 en producción
-- (verificado). El flujo que sí usa es `client_credentials`, que no necesita
-- redirect_uri.
--
-- Un callback que no existe no es inofensivo: el barrido de cierre de sesión
-- deriva a quién preguntar de los redirect_uri registrados, así que Hub entraba
-- en la lista y sumaba un 404 a CADA cierre de sesión de la plataforma. Y
-- `authorization_code` se va con él: sin destino registrado ese flujo ya era
-- imposible, y anunciar un grant que no se puede ejercer solo confunde a quien
-- lea el client.
UPDATE oauth_clients
SET redirect_uris = '{}',
    grant_types = array_remove(grant_types, 'authorization_code'),
    updated_at = now()
WHERE id = 'sytue-hub';
