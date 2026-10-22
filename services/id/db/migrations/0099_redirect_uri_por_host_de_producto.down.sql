-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Camino de vuelta de la 0099: retira los callbacks por host de producto con
-- prefijo. Los de organización (0054 y hermanas) no se tocan: sin ellos, un
-- despliegue anterior del chart —que anunciaba `corporate.sytue.eu/...`—
-- dejaría de poder hacer login.
BEGIN;

UPDATE oauth_clients AS c
SET redirect_uris = array_remove(c.redirect_uris, viejo.uri)
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
) AS viejo(client_id, uri)
WHERE c.id = viejo.client_id
  AND viejo.uri = ANY (c.redirect_uris);

COMMIT;
