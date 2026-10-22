-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Seed de los clients OIDC del SPEC (BFF confidencial por superficie). Los
-- secretos NO se siembran aquí (nunca secretos en git): se fijan en el deploy
-- vía Secret Manager / consola admin. secret_hash queda NULL hasta entonces;
-- el token endpoint rechaza autenticación de client con secret_hash NULL.
--
-- redirect_uris cubren: prod (*.sytue.eu), dev Traefik (*.sytue.localhost) y
-- dev Astro directo (localhost:43xx). El callback canónico del BFF es
-- /auth/callback (endpoints Astro de @sytue/auth).

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-one', 'Sytue One (portal)', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://one.sytue.eu/auth/callback',
        'https://one.sytue.localhost/auth/callback',
        'http://localhost:4321/auth/callback'
     ],
     ARRAY['https://one.sytue.eu/', 'http://localhost:4321/']),

    ('sytue-console', 'Sytue Console (admin)', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access,admin}',
     ARRAY[
        'https://console.sytue.eu/auth/callback',
        'https://console.sytue.localhost/auth/callback',
        'http://localhost:4322/auth/callback'
     ],
     ARRAY['https://console.sytue.eu/', 'http://localhost:4322/']),

    ('sytue-cloud', 'Sytue Cloud (dashboard)', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://cloud.sytue.eu/auth/callback',
        'https://cloud.sytue.localhost/auth/callback',
        'http://localhost:4323/auth/callback'
     ],
     ARRAY['https://cloud.sytue.eu/', 'http://localhost:4323/']),

    ('sytue-cloud-api', 'Sytue Cloud API (control plane)', 'confidential', false, 'client_secret_basic',
     '{client_credentials}', '{}', '{cloud.deploy,cloud.read}',
     ARRAY[]::text[], ARRAY[]::text[]),

    ('sytue-mail', 'Sytue Mail', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://mail.sytue.eu/auth/callback',
        'https://mail.sytue.localhost/auth/callback',
        'http://localhost:4325/auth/callback'
     ],
     ARRAY['https://mail.sytue.eu/', 'http://localhost:4325/']),

    ('sytue-drive', 'Sytue Drive', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://drive.sytue.eu/auth/callback',
        'https://drive.sytue.localhost/auth/callback',
        'http://localhost:4326/auth/callback'
     ],
     ARRAY['https://drive.sytue.eu/', 'http://localhost:4326/']),

    ('sytue-calendar', 'Sytue Calendar', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://calendar.sytue.eu/auth/callback',
        'https://calendar.sytue.localhost/auth/callback',
        'http://localhost:4327/auth/callback'
     ],
     ARRAY['https://calendar.sytue.eu/', 'http://localhost:4327/']),

    ('sytue-contacts', 'Sytue Contacts', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://contacts.sytue.eu/auth/callback',
        'https://contacts.sytue.localhost/auth/callback',
        'http://localhost:4328/auth/callback'
     ],
     ARRAY['https://contacts.sytue.eu/', 'http://localhost:4328/']),

    ('sytue-docs', 'Sytue Docs', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://docs.sytue.eu/auth/callback',
        'https://docs.sytue.localhost/auth/callback',
        'http://localhost:4329/auth/callback'
     ],
     ARRAY['https://docs.sytue.eu/', 'http://localhost:4329/']),

    ('sytue-chat', 'Sytue Chat', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://chat.sytue.eu/auth/callback',
        'https://chat.sytue.localhost/auth/callback',
        'http://localhost:4330/auth/callback'
     ],
     ARRAY['https://chat.sytue.eu/', 'http://localhost:4330/']),

    ('sytue-meet', 'Sytue Meet', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
        'https://meet.sytue.eu/auth/callback',
        'https://meet.sytue.localhost/auth/callback',
        'http://localhost:4331/auth/callback'
     ],
     ARRAY['https://meet.sytue.eu/', 'http://localhost:4331/']),

    ('sytue-hub', 'Sytue Hub (shell backend)', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token,client_credentials}', '{code}',
     '{openid,profile,email,offline_access,hub.read}',
     ARRAY[
        'https://hub.sytue.eu/auth/callback',
        'https://hub.sytue.localhost/auth/callback'
     ],
     ARRAY['https://hub.sytue.eu/'])
ON CONFLICT (id) DO NOTHING;
