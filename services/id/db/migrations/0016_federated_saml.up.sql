-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Federación SAML 2.0: Sytue ID actúa de Service Provider (SP) para que una
-- empresa con EntraID/ADFS/Okta federe su login corporativo por SAML. REUSA el
-- modelo OIDC de la mig 0015 (identity_providers + federated_identities +
-- federated_auth_states + el find-or-JIT con anti-takeover): solo se añade lo
-- ESPECÍFICO de SAML como columnas nullable-con-default, y se admite type='saml'.
-- Los providers OIDC existentes dejan estas columnas en '' (sin efecto).

-- 1) Admitir type='saml' además de 'oidc' (la mig 0015 fijó CHECK (type IN ('oidc'))).
ALTER TABLE identity_providers DROP CONSTRAINT identity_providers_type_check;
ALTER TABLE identity_providers
    ADD CONSTRAINT identity_providers_type_check CHECK (type IN ('oidc', 'saml'));

-- 2) Columnas específicas de SAML.
ALTER TABLE identity_providers
    -- EntityID del IdP == Issuer de la assertion (crewjam lo valida contra esto).
    ADD COLUMN idp_entity_id   text NOT NULL DEFAULT '',
    -- SingleSignOnService del IdP: destino del AuthnRequest.
    ADD COLUMN idp_sso_url     text NOT NULL DEFAULT '',
    -- Certificado de FIRMA del IdP en PEM. Es PÚBLICO (no un secreto): valida la
    -- firma XML-DSig de la Response/Assertion. Se guarda en claro (a diferencia del
    -- client_secret OIDC, que sí se cifra) porque no confiere capacidad ninguna.
    ADD COLUMN idp_certificate text NOT NULL DEFAULT '',
    -- EntityID/audience que el SP presenta. Vacío ⇒ por defecto la metadata URL del
    -- provider (<issuer>/saml/{id}/metadata), estable y derivada del id.
    ADD COLUMN sp_entity_id    text NOT NULL DEFAULT '',
    -- Binding del AuthnRequest hacia el IdP: HTTP-Redirect (defecto) o HTTP-POST.
    ADD COLUMN saml_binding    text NOT NULL DEFAULT 'redirect'
                               CHECK (saml_binding IN ('redirect', 'post')),
    -- NameIDPolicy solicitado (urn…:emailAddress / persistent / …). Vacío ⇒ el
    -- por defecto de la librería (transient), interoperable con la mayoría de IdP.
    ADD COLUMN name_id_format  text NOT NULL DEFAULT '';
