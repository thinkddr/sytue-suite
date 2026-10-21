-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Avatar propio del usuario (predefinido o foto), self-service desde /account.
--
-- FUERA de `users` porque nunca hace falta en el SELECT de siempre (SCIM, listas
-- de admin, claims del token): son hasta 256 KiB de data URI que ningún llamante
-- de esos necesita, y colarlos en userCols habría inflado cada respuesta que lee
-- un usuario. Tabla 1:1 aparte, con su propio ciclo de vida.
--
-- avatar_updated_at es el etag de caché del byte-serving endpoint (mismo papel
-- que el etag de una tarjeta en Contactos): cambia con cada subida y así la URL
-- servida puede llevar cache-control agresivo sin servir una foto vieja.
CREATE TABLE user_avatars (
    user_id    uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    data_uri   text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
