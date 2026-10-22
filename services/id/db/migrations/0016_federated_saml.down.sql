-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

ALTER TABLE identity_providers
    DROP COLUMN idp_entity_id,
    DROP COLUMN idp_sso_url,
    DROP COLUMN idp_certificate,
    DROP COLUMN sp_entity_id,
    DROP COLUMN saml_binding,
    DROP COLUMN name_id_format;

ALTER TABLE identity_providers DROP CONSTRAINT identity_providers_type_check;
ALTER TABLE identity_providers
    ADD CONSTRAINT identity_providers_type_check CHECK (type IN ('oidc'));
