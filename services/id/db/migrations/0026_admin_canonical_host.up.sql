-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- «Sytue Console» pasa a llamarse «Sytue Admin» y a tener HOST CANÓNICO PROPIO:
-- admin.sytue.{eu,localhost}. El nombre anterior se confundía con la consola de
-- Sytue Cloud y con la de MinIO; «Admin» dice lo que es: la administración de la
-- organización.
--
-- Mismo patrón que 0023 (Rooms): el cliente OIDC `sytue-console` NO se renombra
-- —es un identificador, no un rótulo— pero se le AÑADEN los callbacks de admin.*
-- y se le cambia el nombre visible. Los de console.* se CONSERVAN a propósito:
-- Traefik responde 301 desde console.* a admin.*, pero un cliente con una sesión
-- a medias puede volver al callback antiguo, y mantenerlos evita un
-- `invalid_redirect_uri` durante la transición (y permite revertir sin migración).

UPDATE oauth_clients
SET name = 'Sytue Admin',
    redirect_uris = ARRAY[
        'https://admin.sytue.eu/auth/callback',
        'https://admin.sytue.localhost/auth/callback',
        'https://console.sytue.eu/auth/callback',
        'https://console.sytue.localhost/auth/callback',
        'http://localhost:4322/auth/callback'
    ],
    post_logout_redirect_uris = ARRAY[
        'https://admin.sytue.eu/',
        'https://admin.sytue.localhost/',
        'https://console.sytue.eu/',
        'http://localhost:4322/'
    ],
    updated_at = now()
WHERE id = 'sytue-console';
