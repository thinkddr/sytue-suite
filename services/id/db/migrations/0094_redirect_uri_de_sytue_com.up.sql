-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Los redirect_uri de `sytue.com`, que desde ahora sirve la suite Astro con un
-- SEGUNDO JUEGO DE IMÁGENES construido con `DOMINIO=sytue.com` y desplegado en
-- el frontal (ADR 0049). Hermana de la 0054, que hizo lo propio con el host de
-- organización, y por el mismo motivo: Sytue ID compara el `redirect_uri`
-- LITERAL contra la lista registrada, así que sin esta fila el login en
-- `sytue.com` falla con `invalid_request` y no hay nada en el despliegue que
-- avise. Es la clase de fallo que se descubre cuando alguien intenta entrar.
--
-- LOS DOS FRENTES COMPARTEN IdP, servicios y datos. Solo se duplica el
-- frontend, así que aquí no nacen clientes nuevos: se le añade una dirección
-- más a los que ya existen. El issuer sigue siendo `https://id.sytue.eu` y solo
-- ese: un IdP emite UN `iss`, y el frente en `.com` lo alcanza por su
-- `SYTUE_OIDC_ISSUER` de ejecución, que es una variable distinta de la
-- `PUBLIC_OIDC_ISSUER` con la que se hornean los enlaces.
--
-- La forma de las URL es la del subdominio por producto, que es como se sirve
-- ahí: `https://mail.sytue.com/mail/auth/callback`. El prefijo no sobra —cada
-- app se sirve bajo el suyo, igual que en `.eu`— y el host tampoco: el
-- `redirect_uri` tiene que ser el del dominio por el que entró la persona, o el
-- login la devuelve al otro frente y la sesión se queda donde no estaba.
--
-- Se AÑADEN, no se sustituyen: `sytue.eu` es producción y sus callbacks tienen
-- que seguir valiendo. Un cliente puede tener varias.
--
-- `sytue-gio` NO se toca. La suite en WebAssembly sigue registrada con
-- `https://sytue.com/auth/callback` (migración 0070) y sigue sirviendo hasta
-- que el relevo esté verificado con sesión; quitarle el callback antes es dejar
-- sin login lo único que hay ahí. Su baja va en una migración posterior.
--
-- Las COOKIES no necesitan nada: son `__Host-`, o sea host-only y sin atributo
-- `Domain` (`sessionCookieAttributes`), y `resolveOidcConfig` LANZA si alguien
-- define `SYTUE_COOKIE_DOMAIN`. Cada host tiene la suya y `.com` no depende de
-- que valga una de `.eu`. Lo que sí implica es que la sesión NO se comparte
-- entre los dos frentes: entrar en `.com` es un login propio, resuelto por el
-- SSO del IdP sin volver a pedir contraseña.
--
-- OJO: la columna del identificador de cliente se llama `id`, NO `client_id`
-- (ver 0001_core_identity.up.sql:80). Escribir `c.client_id` costó una migración
-- sucia en producción y el IdP en CrashLoopBackOff: `pg.MigrateUp` corre al
-- arrancar, así que un SQL malo no falla en el despliegue — falla al levantar.
BEGIN;

-- `array_append` solo si no está ya: la migración tiene que poder repetirse sin
-- duplicar entradas, porque `pg.MigrateUp` la aplica al arrancar cada réplica.
--
-- El subdominio y el prefijo de cada app salen de
-- `deploy/helm/charts/sytue-frontend/values.yaml` (`subdomain` y `path`) y el
-- identificador de su `clientId`, que es lo que reutiliza el frente en `.com`.
-- Si dejan de coincidir, el login de esa app falla SOLO en `sytue.com`: lo
-- vigila `TestRedirectUrisDeSytueComCubrenElCompose`, que compara esta lista
-- con la que anuncia `deploy/sytue-com/docker-compose.yml`.
UPDATE oauth_clients AS c
SET redirect_uris = c.redirect_uris || nuevo.uri
FROM (VALUES
    ('sytue-account',   'https://account.sytue.com/account/auth/callback'),
    ('sytue-calendar',  'https://calendar.sytue.com/calendar/auth/callback'),
    ('sytue-chat',      'https://rooms.sytue.com/rooms/auth/callback'),
    ('sytue-cloud',     'https://cloud.sytue.com/cloud/auth/callback'),
    ('sytue-console',   'https://admin.sytue.com/admin/auth/callback'),
    ('sytue-contacts',  'https://contacts.sytue.com/contacts/auth/callback'),
    ('sytue-docs',      'https://docs.sytue.com/docs/auth/callback'),
    ('sytue-drive',     'https://drive.sytue.com/drive/auth/callback'),
    ('sytue-mail',      'https://mail.sytue.com/mail/auth/callback'),
    ('sytue-notes',     'https://notes.sytue.com/notes/auth/callback'),
    ('sytue-one',       'https://one.sytue.com/one/auth/callback'),
    ('sytue-syerp',     'https://erp.sytue.com/erp/auth/callback'),
    ('sytue-syphotos',  'https://photos.sytue.com/photos/auth/callback')
) AS nuevo(client_id, uri)
WHERE c.id = nuevo.client_id
  AND NOT (nuevo.uri = ANY (c.redirect_uris));

-- El cierre de sesión devuelve al sitio público, y en `.com` ese es el suyo: sin
-- registrarlo, Sytue ID rechaza el `post_logout_redirect_uri` como
-- open-redirect y el logout muere en una pantalla de error (mismo criterio que
-- la 0033 y la 0059).
UPDATE oauth_clients AS c
SET post_logout_redirect_uris = c.post_logout_redirect_uris || ARRAY['https://www.sytue.com/']
WHERE c.id IN (
        'sytue-account', 'sytue-calendar', 'sytue-chat', 'sytue-cloud', 'sytue-console',
        'sytue-contacts', 'sytue-docs', 'sytue-drive', 'sytue-mail', 'sytue-notes',
        'sytue-one', 'sytue-syerp', 'sytue-syphotos')
  AND NOT ('https://www.sytue.com/' = ANY (c.post_logout_redirect_uris));

COMMIT;
