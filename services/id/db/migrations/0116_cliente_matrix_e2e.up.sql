-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- Synapse is an explicitly approved separate transport (ADR 0077). It uses
-- Sytue ID; no separate end-user passwords or server-side message keys.
INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes, redirect_uris)
VALUES ('sytue-matrix', 'Sytue Rooms Matrix', 'confidential', true, 'client_secret_basic',
        '{authorization_code}', '{code}', '{openid,profile}',
        '{https://matrix.sytue.eu/_synapse/client/oidc/callback}')
ON CONFLICT (id) DO NOTHING;

-- nextval is deliberately not rolled back: a failed revocation must not allow
-- a subsequent activation to reuse its fence value.
CREATE SEQUENCE IF NOT EXISTS matrix_access_version_seq AS bigint;
