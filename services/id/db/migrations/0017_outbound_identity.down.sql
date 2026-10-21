-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DELETE FROM oauth_clients WHERE id = 'sytue-gateway';

DROP TABLE IF EXISTS dcr_initial_tokens;

-- oauth_clients_org_idx cae con la columna org_id.
ALTER TABLE oauth_clients
    DROP COLUMN IF EXISTS managed_by,
    DROP COLUMN IF EXISTS org_id;

DROP TABLE IF EXISTS saml_idp_keys;

-- saml_sps_* (índices, trigger) caen con la tabla.
DROP TABLE IF EXISTS saml_sps;
