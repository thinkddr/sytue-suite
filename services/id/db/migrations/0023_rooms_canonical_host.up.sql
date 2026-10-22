-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue Rooms pasa a tener HOST CANÓNICO PROPIO: rooms.sytue.{eu,localhost}.
-- Hasta ahora Rooms (Chat+Meet unificados) se servía sobre el host heredado de
-- Chat, así que la URL seguía diciendo «chat» mientras el producto ya era Rooms.
--
-- El cliente OIDC `sytue-chat` es el de Rooms: se le renombra a «Sytue Rooms» y se
-- le AÑADEN los callbacks/post-logout de rooms.*. Los de chat.* se CONSERVAN a
-- propósito: Traefik responde 301 desde chat.* a rooms.*, pero un cliente con una
-- sesión a medias puede volver al callback antiguo, y mantenerlos evita un
-- `invalid_redirect_uri` durante la transición (y permite revertir sin migración).
--
-- El cliente `sytue-meet` queda como legado (la app Astro de Meet ya se retiró y
-- meet.* redirige a Rooms): no se borra para no romper un despliegue a medio migrar,
-- pero se marca en el nombre para que la consola no invite a usarlo.

UPDATE oauth_clients
SET name = 'Sytue Rooms',
    redirect_uris = ARRAY[
        'https://rooms.sytue.eu/auth/callback',
        'https://rooms.sytue.localhost/auth/callback',
        'https://chat.sytue.eu/auth/callback',
        'https://chat.sytue.localhost/auth/callback',
        'http://localhost:4330/auth/callback'
    ],
    post_logout_redirect_uris = ARRAY[
        'https://rooms.sytue.eu/',
        'https://rooms.sytue.localhost/',
        'https://chat.sytue.eu/',
        'http://localhost:4330/'
    ],
    updated_at = now()
WHERE id = 'sytue-chat';

UPDATE oauth_clients
SET name = 'Sytue Meet (legado — unificado en Rooms)',
    updated_at = now()
WHERE id = 'sytue-meet';
