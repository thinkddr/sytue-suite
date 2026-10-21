-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Identidad SALIENTE (Sytue Integration Gateway · Ola 2A): Sytue ID actúa de IdP
-- hacia SaaS de terceros (SSO 1-click). Tres piezas:
--
--   saml_sps         SP SAML de terceros registrados POR una organización (Sytue
--                    ID emite Assertions firmadas hacia ellos).
--   saml_idp_keys    material de firma del IdP POR organización (clave privada
--                    SELLADA en reposo, cert público). Cada tenant es su propio IdP.
--   oauth_clients    gana org_id + managed_by para clients OIDC por-org y registro
--                    dinámico (RFC 7591); dcr_initial_tokens guarda los tokens de
--                    acceso inicial del DCR.
--
-- AISLAMIENTO POR TENANT (igual que mig 0001/0015, a NIVEL DE APLICACIÓN): FK org_id
-- + UNIQUE(org,…) + scoping por org en cada query, DENY-BY-DEFAULT. Un SP de la org A
-- jamás recibe una Assertion emitida en nombre de la org B. Sin RLS de Postgres
-- (ningún objeto del servicio la usa; el pool compartido no fija GUCs por-conexión).
--
-- SEGURIDAD: saml_idp_keys.private_pem se sella con la KEK/AES-256-GCM existente
-- (keycrypto, encv1:…) — la columna guarda el sobre, NUNCA la clave en claro, igual
-- que signing_keys. cert_pem e idp/sp entityIDs son PÚBLICOS.

-- 1) SP SAML de terceros registrados por una organización.
CREATE TABLE saml_sps (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                text NOT NULL,
    -- EntityID/audience del SP == Issuer de su AuthnRequest. Casa el flujo SP-initiated
    -- con el SP registrado, ACOTADO a la org de la sesión (aislamiento tenant).
    entity_id           text NOT NULL,
    -- Assertion Consumer Service (HTTP-POST): destino de la Response firmada.
    acs_url             text NOT NULL,
    -- Certificado del SP en PEM (opcional): para cifrar la Assertion / verificar. Es
    -- PÚBLICO (no un secreto). '' ⇒ Assertion firmada sin cifrar (interoperable).
    cert_pem            text NOT NULL DEFAULT '',
    -- NameIDPolicy a emitir (…:emailAddress / persistent / transient). '' ⇒ emailAddress.
    name_id_format      text NOT NULL DEFAULT '',
    -- Mapeo de atributos: nombre del atributo SAML para email/name/groups del sujeto.
    attr_mapping        jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- El SP admite flujo IdP-initiated (1-click desde el portal).
    idp_initiated       boolean NOT NULL DEFAULT false,
    relay_state_default text NOT NULL DEFAULT '',
    enabled             boolean NOT NULL DEFAULT false,
    created_by          uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
-- EntityID único POR ORGANIZACIÓN: resuelve el issuer del AuthnRequest dentro de la
-- org de la sesión (dos orgs pueden registrar el mismo SP de forma independiente).
CREATE UNIQUE INDEX saml_sps_org_entity_key ON saml_sps (org_id, entity_id);
-- Listado admin por org con keyset (created_at,id) DESC, sin Seq Scan a escala.
CREATE INDEX saml_sps_org_idx ON saml_sps (org_id, created_at DESC, id DESC);

CREATE TRIGGER saml_sps_set_updated_at BEFORE UPDATE ON saml_sps
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 2) Clave de firma del IdP POR organización (Sytue ID como IdP). Se genera perezosa
-- la 1ª vez que la org emite/expone metadata y se conserva. private_pem SELLADO
-- (encv1:… AES-256-GCM) como signing_keys; cert_pem es público.
CREATE TABLE saml_idp_keys (
    org_id      uuid PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    private_pem text NOT NULL,   -- sobre encv1:… de la clave RSA (PKCS#1 PEM)
    cert_pem    text NOT NULL,   -- certificado X.509 autofirmado (PEM), PÚBLICO
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- 3) oauth_clients: scoping por org + procedencia. Los clients first-party (mig 0002)
-- quedan con org_id NULL (globales) y managed_by='static'.
ALTER TABLE oauth_clients
    ADD COLUMN org_id     uuid REFERENCES organizations (id) ON DELETE CASCADE,
    ADD COLUMN managed_by text NOT NULL DEFAULT 'static'
               CHECK (managed_by IN ('static', 'dcr', 'admin'));
-- Listar los clients de una org (Console) sin tocar los globales.
CREATE INDEX oauth_clients_org_idx ON oauth_clients (org_id) WHERE org_id IS NOT NULL;

-- 4) Tokens de acceso inicial del registro dinámico (RFC 7591). Solo el HASH SHA-256
-- (el valor viaja como bearer), acotado a una org y con caducidad. Reutilizable
-- dentro de su ventana (un admin emite uno y registra N apps hasta que expira).
CREATE TABLE dcr_initial_tokens (
    token_hash text PRIMARY KEY,
    org_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    created_by uuid REFERENCES users (id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX dcr_initial_tokens_org_idx ON dcr_initial_tokens (org_id);
CREATE INDEX dcr_initial_tokens_expires_idx ON dcr_initial_tokens (expires_at);

-- Client máquina del Sytue Integration Gateway (client_credentials). El secreto se
-- fija en el deploy (Secret Manager): secret_hash queda NULL hasta entonces (el token
-- endpoint rechaza un client confidencial con secret_hash NULL). Los scopes habilitan
-- los endpoints de directorio (lectura) y gestión de apps de las olas W2B/W3.
INSERT INTO oauth_clients
    (id, name, type, require_pkce, token_endpoint_auth_method,
     grant_types, response_types, scopes, redirect_uris, post_logout_redirect_uris,
     managed_by)
VALUES
    ('sytue-gateway', 'Sytue Integration Gateway', 'confidential', false, 'client_secret_basic',
     '{client_credentials}', '{}', '{id.directory.read,id.apps.manage}',
     ARRAY[]::text[], ARRAY[]::text[], 'static')
ON CONFLICT (id) DO NOTHING;
