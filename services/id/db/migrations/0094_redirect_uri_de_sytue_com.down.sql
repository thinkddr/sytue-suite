-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Quita las direcciones de `sytue.com` y deja las demás intactas.
--
-- Es el camino de vuelta del ADR 0049: con esto aplicado, parado el compose del
-- frontal y devueltos los routers de `sytue-gio`, `sytue.com` vuelve a ser
-- exactamente lo que era.
BEGIN;

UPDATE oauth_clients AS c
SET redirect_uris = array_remove(c.redirect_uris, viejo.uri)
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
) AS viejo(client_id, uri)
WHERE c.id = viejo.client_id
  AND viejo.uri = ANY (c.redirect_uris);

UPDATE oauth_clients AS c
SET post_logout_redirect_uris = array_remove(c.post_logout_redirect_uris, 'https://www.sytue.com/')
WHERE 'https://www.sytue.com/' = ANY (c.post_logout_redirect_uris);

COMMIT;
