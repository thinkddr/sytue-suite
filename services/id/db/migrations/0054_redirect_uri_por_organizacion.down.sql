-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Retira los callbacks por organización y deja solo los de subdominio.
--
-- Ojo al orden si se revierte de verdad: primero hay que devolver el chart a
-- anunciar el subdominio en `SYTUE_OIDC_REDIRECT_URI`, y DESPUÉS quitar la URL de
-- aquí. Al revés deja a las nueve superficies anunciando un callback que ya no
-- está registrado, o sea sin login.
-- OJO: la columna del identificador de cliente se llama `id`, NO `client_id`
-- (ver 0001_core_identity.up.sql:80). Escribir `c.client_id` costó una migración
-- sucia en producción y el IdP en CrashLoopBackOff: `pg.MigrateUp` corre al
-- arrancar, así que un SQL malo no falla en el despliegue — falla al levantar, y
-- deja la base marcada como dirty hasta que alguien la fuerce a mano.
BEGIN;

UPDATE oauth_clients AS c
SET redirect_uris = array_remove(c.redirect_uris, viejo.uri)
FROM (VALUES
    ('sytue-calendar',  'https://corporate.sytue.eu/calendar/auth/callback'),
    ('sytue-chat',      'https://corporate.sytue.eu/rooms/auth/callback'),
    ('sytue-cloud',     'https://corporate.sytue.eu/cloud/auth/callback'),
    ('sytue-console',   'https://corporate.sytue.eu/admin/auth/callback'),
    ('sytue-contacts',  'https://corporate.sytue.eu/contacts/auth/callback'),
    ('sytue-docs',      'https://corporate.sytue.eu/docs/auth/callback'),
    ('sytue-drive',     'https://corporate.sytue.eu/drive/auth/callback'),
    ('sytue-mail',      'https://corporate.sytue.eu/mail/auth/callback'),
    ('sytue-one',       'https://corporate.sytue.eu/one/auth/callback')
) AS viejo(client_id, uri)
WHERE c.id = viejo.client_id;

COMMIT;
