-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Los redirect_uri por HOST DE PRODUCTO CON PREFIJO, que desde ahora son las
-- canónicas: https://mail.sytue.eu/mail/auth/callback y sus doce hermanas.
--
-- POR QUÉ. Hasta hoy la canónica era la de organización
-- (`corporate.sytue.eu/<app>/auth/callback`, migración 0054): el chart la
-- anunciaba en `SYTUE_OIDC_REDIRECT_URI` y el BFF —que empieza el flujo SIEMPRE
-- en el host del callback— redirigía ahí aunque el navegador estuviera en
-- `mail.sytue.eu`. Ese rodeo es la cadena de redirecciones que se vive al
-- entrar: host de producto → host de organización → IdP → host de organización.
-- La organización NO vive en el host —viaja firmada en el token (claim
-- `tenant`)—, así que nada de la sesión la exige; lo único que ataba la
-- canónica al host de organización era el propio `redirect_uri`.
--
-- EL PRECEDENTE. Los tres entornos ya loguean por host de producto: dev por la
-- forma raíz (`mail.sytue.localhost/auth/callback`, 0002) y el frente
-- `sytue.com` (ADR 0049) por la forma CON PREFIJO, exactamente la de esta
-- migración (0094). En `.com` lleva en producción desde su alta.
--
-- Se AÑADEN, no se sustituyen: los callbacks de organización (0054 y hermanas)
-- siguen valiendo durante la transición; el chart los deja de anunciar, pero un
-- login empezado en `corporate.sytue.eu` sigue funcionando hasta que ese host
-- se convierta en 301. Un cliente puede tener varias direcciones.
--
-- La forma CON PREFIJO (`mail.sytue.eu/mail/...`) y no la raíz es la que sirve
-- la imagen: el `base` de Astro se hornea al construir y la imagen responde bajo
-- `/mail` en cualquier host. Los hosts de producto ya sirven ese MISMO prefijo
-- (ver el Ingress por app del chart de frontends).
--
-- `sytue-platform` va SIN prefijo: su superficie se sirve en la raíz de
-- `platform.sytue.eu` (`homePath`, values.yaml) y su callback ya estaba
-- registrado así. `sytue-account` tampoco va aquí: la cuenta es GLOBAL
-- (`id.<dominio>/account`, 0097) y su canónica no cambia. `sytue-www` no
-- tiene login.
--
-- OJO: la columna del identificador de cliente se llama `id`, NO `client_id`
-- (ver 0001_core_identity.up.sql:80). Escribir `c.client_id` costó una migración
-- sucia en producción y el IdP en CrashLoopBackOff (0094 lo documenta): un SQL
-- malo no falla en el despliegue — falla al levantar.
BEGIN;

-- `array_append` solo si no está ya: la migración tiene que poder repetirse sin
-- duplicar entradas (pg.MigrateUp corre al arrancar cada réplica).
UPDATE oauth_clients AS c
SET redirect_uris = c.redirect_uris || nuevo.uri
FROM (VALUES
    ('sytue-one',       'https://one.sytue.eu/one/auth/callback'),
    ('sytue-console',   'https://admin.sytue.eu/admin/auth/callback'),
    ('sytue-cloud',     'https://cloud.sytue.eu/cloud/auth/callback'),
    ('sytue-mail',      'https://mail.sytue.eu/mail/auth/callback'),
    ('sytue-drive',     'https://drive.sytue.eu/drive/auth/callback'),
    ('sytue-calendar',  'https://calendar.sytue.eu/calendar/auth/callback'),
    ('sytue-contacts',  'https://contacts.sytue.eu/contacts/auth/callback'),
    ('sytue-docs',      'https://docs.sytue.eu/docs/auth/callback'),
    ('sytue-notes',     'https://notes.sytue.eu/notes/auth/callback'),
    ('sytue-syphotos',  'https://photos.sytue.eu/photos/auth/callback'),
    ('sytue-syerp',     'https://erp.sytue.eu/erp/auth/callback'),
    ('sytue-chat',      'https://rooms.sytue.eu/rooms/auth/callback'),
    ('sytue-platform',  'https://platform.sytue.eu/auth/callback')
) AS nuevo(client_id, uri)
WHERE c.id = nuevo.client_id
  AND NOT (nuevo.uri = ANY (c.redirect_uris));

COMMIT;
