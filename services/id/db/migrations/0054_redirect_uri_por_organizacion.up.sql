-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Los redirect_uri de la dirección por organización, que desde ahora es la
-- canónica: https://<slug>.sytue.eu/mail/auth/callback y sus ocho hermanas.
--
-- POR QUÉ HACE FALTA. Las dos direcciones son válidas —`mail.sytue.eu` no se
-- retira—, y el chart anuncia la canónica en `SYTUE_OIDC_REDIRECT_URI`. Sytue ID
-- compara ese valor LITERAL contra la lista registrada: si no está, el login
-- falla con `invalid_request` y no hay nada en el despliegue que avise. Es la
-- clase de fallo que se descubre cuando alguien intenta entrar.
--
-- Se AÑADEN, no se sustituyen: los subdominios siguen sirviendo y sus callbacks
-- tienen que seguir valiendo. Un cliente puede tener varias.
--
-- `corporate` es el slug de la organización que opera la plataforma
-- (`orgHost.canonical` del chart). Cuando haya más organizaciones no habrá que
-- volver aquí: la que se registra es la del host que sirve, y el comodín ya las
-- cubre todas a nivel de ingreso — pero OIDC sí exige la URL exacta, así que dar
-- de alta una organización con superficie propia implicará añadir su callback.
-- Ese es el precio de que OIDC compare literales, y es deliberado: un
-- `redirect_uri` con comodín es una vulnerabilidad conocida.
-- OJO: la columna del identificador de cliente se llama `id`, NO `client_id`
-- (ver 0001_core_identity.up.sql:80). Escribir `c.client_id` costó una migración
-- sucia en producción y el IdP en CrashLoopBackOff: `pg.MigrateUp` corre al
-- arrancar, así que un SQL malo no falla en el despliegue — falla al levantar, y
-- deja la base marcada como dirty hasta que alguien la fuerce a mano.
BEGIN;

-- `array_append` solo si no está ya: la migración tiene que poder repetirse sin
-- duplicar entradas, porque `pg.MigrateUp` la aplica al arrancar cada réplica y
-- un array con la misma URL dos veces no rompe nada pero ensucia la auditoría.
UPDATE oauth_clients AS c
SET redirect_uris = c.redirect_uris || nuevo.uri
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
) AS nuevo(client_id, uri)
WHERE c.id = nuevo.client_id
  AND NOT (nuevo.uri = ANY (c.redirect_uris));

COMMIT;
