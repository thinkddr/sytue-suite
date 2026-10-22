-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Un client confidencial por cada app de la ofimática (decisión del dueño del
-- 2026-10-12): Sytue Docs se separa en Docus, Calcus, Diapos y CertiPDF, con host y
-- prefijo propios (`calcus.sytue.eu/calcus`) y una app de Astro cada uno. El secreto
-- se inyecta al desplegar (bootstrap), como en los demás BFF: aquí nacen sin él.
--
-- Callbacks con la forma de 0106: la CANÓNICA es host de producto con prefijo (la que
-- anuncia el chart de frontends y cruza TestRedirectUrisDeLosHostsDeSuiteCubrenElChart),
-- la raíz del host, la de la organización y las de desarrollo (puertos 4338-4341).
--
-- `sytue-docs` NO se toca aquí: sigue sirviendo `docs.sytue.eu/docs` hasta que la suite
-- apunte a los hosts nuevos, y se retira en una migración aparte.
--
-- La columna del identificador es `id`, NO `client_id` (ver 0001 y la nota de 0094).
-- Idempotente.
BEGIN;

INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes,
     redirect_uris, post_logout_redirect_uris)
VALUES
    ('sytue-docus', 'Sytue Docus', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
       'https://docus.sytue.eu/docus/auth/callback',
       'https://docus.sytue.eu/auth/callback',
       'https://corporate.sytue.eu/docus/auth/callback',
       'https://docus.sytue.localhost/auth/callback',
       'http://localhost:4338/auth/callback'
     ],
     ARRAY[
       'https://docus.sytue.eu/',
       'https://docus.sytue.eu/docus/',
       'https://corporate.sytue.eu/docus/',
       'http://localhost:4338/'
     ]),
    ('sytue-calcus', 'Sytue Calcus', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
       'https://calcus.sytue.eu/calcus/auth/callback',
       'https://calcus.sytue.eu/auth/callback',
       'https://corporate.sytue.eu/calcus/auth/callback',
       'https://calcus.sytue.localhost/auth/callback',
       'http://localhost:4339/auth/callback'
     ],
     ARRAY[
       'https://calcus.sytue.eu/',
       'https://calcus.sytue.eu/calcus/',
       'https://corporate.sytue.eu/calcus/',
       'http://localhost:4339/'
     ]),
    ('sytue-diapos', 'Sytue Diapos', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
       'https://diapos.sytue.eu/diapos/auth/callback',
       'https://diapos.sytue.eu/auth/callback',
       'https://corporate.sytue.eu/diapos/auth/callback',
       'https://diapos.sytue.localhost/auth/callback',
       'http://localhost:4340/auth/callback'
     ],
     ARRAY[
       'https://diapos.sytue.eu/',
       'https://diapos.sytue.eu/diapos/',
       'https://corporate.sytue.eu/diapos/',
       'http://localhost:4340/'
     ]),
    ('sytue-certipdf', 'Sytue CertiPDF', 'confidential', true, 'client_secret_basic',
     '{authorization_code,refresh_token}', '{code}', '{openid,profile,email,offline_access}',
     ARRAY[
       'https://certipdf.sytue.eu/certipdf/auth/callback',
       'https://certipdf.sytue.eu/auth/callback',
       'https://corporate.sytue.eu/certipdf/auth/callback',
       'https://certipdf.sytue.localhost/auth/callback',
       'http://localhost:4341/auth/callback'
     ],
     ARRAY[
       'https://certipdf.sytue.eu/',
       'https://certipdf.sytue.eu/certipdf/',
       'https://corporate.sytue.eu/certipdf/',
       'http://localhost:4341/'
     ])
ON CONFLICT (id) DO NOTHING;

COMMIT;
