-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Sytue ID — esquema nuclear de identidad (clean-room, sobre OIDC/OAuth2/SCIM).
-- Datastore único: PostgreSQL (sin Redis/Valkey). Multi-tenant por columna
-- tenant + RLS aguas arriba en los servicios consumidores. Aquí Sytue ID es la
-- fuente de verdad de organizaciones, usuarios, credenciales, clients y grants.

-- Extensiones (idempotentes). pgcrypto: gen_random_uuid + funciones de cifrado.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Organizaciones (tenants). slug ⇄ grupo `org/<slug>` en Sytue ID.
CREATE TABLE organizations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE
                CHECK (slug ~ '^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$'),
    name        text NOT NULL,
    status      text NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'suspended', 'deleted')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Usuarios. email único de forma global (login en toda la suite).
-- password_hash: Argon2id (codificación PHC). Nunca texto plano.
CREATE TABLE users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- email se guarda como text y se normaliza a minúsculas en la aplicación;
    -- la unicidad case-insensitive la garantiza users_email_lower_key (abajo).
    email           text NOT NULL,
    email_verified  boolean NOT NULL DEFAULT false,
    name            text NOT NULL DEFAULT '',
    given_name      text NOT NULL DEFAULT '',
    family_name     text NOT NULL DEFAULT '',
    locale          text NOT NULL DEFAULT 'es',
    password_hash   text,
    mfa_enabled     boolean NOT NULL DEFAULT false,
    status          text NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'suspended', 'deleted')),
    -- external_id: identificador del proveedor SCIM (idempotencia de provisioning).
    external_id     text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
-- email se normaliza a minúsculas en la aplicación; índice único case-insensitive.
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));
CREATE UNIQUE INDEX users_external_id_key ON users (external_id) WHERE external_id IS NOT NULL;

-- Membresía usuario↔organización con rol. Un usuario puede pertenecer a varias
-- organizaciones con distinto rol. Roles canónicos: owner|admin|member|billing.
CREATE TABLE memberships (
    org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role        text NOT NULL DEFAULT 'member'
                CHECK (role IN ('owner', 'admin', 'member', 'billing')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);
CREATE INDEX memberships_user_idx ON memberships (user_id);

-- Grupos arbitrarios dentro de una organización (más allá del rol primario).
CREATE TABLE groups (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name        text NOT NULL,
    external_id text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, name)
);

CREATE TABLE group_members (
    group_id    uuid NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);

-- Clients OIDC/OAuth2 (BFF confidenciales por superficie). secret_hash en
-- Argon2id; los públicos (PKCE puro) pueden no tener secreto.
CREATE TABLE oauth_clients (
    id                       text PRIMARY KEY,           -- client_id estable (p. ej. "sytue-one")
    secret_hash              text,                       -- NULL para clients públicos
    name                     text NOT NULL,
    type                     text NOT NULL DEFAULT 'confidential'
                             CHECK (type IN ('confidential', 'public')),
    redirect_uris            text[] NOT NULL DEFAULT '{}',
    post_logout_redirect_uris text[] NOT NULL DEFAULT '{}',
    grant_types              text[] NOT NULL DEFAULT '{authorization_code,refresh_token}',
    response_types           text[] NOT NULL DEFAULT '{code}',
    scopes                   text[] NOT NULL DEFAULT '{openid,profile,email}',
    require_pkce             boolean NOT NULL DEFAULT true,
    token_endpoint_auth_method text NOT NULL DEFAULT 'client_secret_basic'
                             CHECK (token_endpoint_auth_method IN
                                    ('client_secret_basic', 'client_secret_post', 'none')),
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now()
);

-- Códigos de autorización (Authorization Code + PKCE). Un solo uso, corta vida.
CREATE TABLE auth_codes (
    code_hash               text PRIMARY KEY,            -- SHA-256 del código (nunca el código en claro)
    client_id               text NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
    user_id                 uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    org_id                  uuid REFERENCES organizations (id) ON DELETE SET NULL,
    redirect_uri            text NOT NULL,
    scope                   text NOT NULL DEFAULT '',
    nonce                   text NOT NULL DEFAULT '',
    code_challenge          text NOT NULL DEFAULT '',
    code_challenge_method   text NOT NULL DEFAULT '',    -- S256 | plain
    auth_time               timestamptz NOT NULL,
    expires_at              timestamptz NOT NULL,
    consumed_at             timestamptz,
    created_at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_codes_expires_idx ON auth_codes (expires_at);

-- Sesiones de navegador (cookie httpOnly del Authorization Server). Soportan SSO.
CREATE TABLE sessions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash      text NOT NULL UNIQUE,                -- SHA-256 del token de sesión
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    org_id          uuid REFERENCES organizations (id) ON DELETE SET NULL,
    auth_time       timestamptz NOT NULL,
    amr             text[] NOT NULL DEFAULT '{pwd}',      -- métodos de autenticación (pwd, mfa, webauthn)
    user_agent      text NOT NULL DEFAULT '',
    ip              inet,
    expires_at      timestamptz NOT NULL,
    revoked_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- Refresh tokens (rotación obligatoria). token_hash = SHA-256. La rotación
-- enlaza el token sucesor para detectar reutilización (token replay).
CREATE TABLE refresh_tokens (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash      text NOT NULL UNIQUE,
    client_id       text NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    org_id          uuid REFERENCES organizations (id) ON DELETE SET NULL,
    session_id      uuid REFERENCES sessions (id) ON DELETE SET NULL,
    scope           text NOT NULL DEFAULT '',
    auth_time       timestamptz NOT NULL,
    rotated_to      uuid REFERENCES refresh_tokens (id) ON DELETE SET NULL,
    expires_at      timestamptz NOT NULL,
    revoked_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);
CREATE INDEX refresh_tokens_expires_idx ON refresh_tokens (expires_at);

-- Claves de firma JWKS (rotación). private_pem cifrado en reposo a nivel de
-- columna en prod (pgcrypto/KMS); aquí se almacena la clave activa por kid.
CREATE TABLE signing_keys (
    kid         text PRIMARY KEY,
    algorithm   text NOT NULL DEFAULT 'RS256',
    public_jwk  jsonb NOT NULL,
    private_pem text NOT NULL,                            -- cifrar en reposo (KMS/pgcrypto) en prod
    status      text NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'next', 'retired')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    retired_at  timestamptz
);

-- Factores MFA / credenciales WebAuthn (passkeys). cred_id es el credential id
-- del autenticador; public_key en COSE/JWK; sign_count para anti-clonado.
CREATE TABLE webauthn_credentials (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    cred_id         bytea NOT NULL UNIQUE,
    public_key      bytea NOT NULL,                       -- clave pública COSE serializada
    aaguid          bytea,
    sign_count      bigint NOT NULL DEFAULT 0,
    transports      text[] NOT NULL DEFAULT '{}',
    name            text NOT NULL DEFAULT 'Passkey',
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_used_at    timestamptz
);
CREATE INDEX webauthn_credentials_user_idx ON webauthn_credentials (user_id);

-- Retos WebAuthn (registro/login) de un solo uso y corta vida.
CREATE TABLE webauthn_challenges (
    challenge   bytea PRIMARY KEY,
    user_id     uuid REFERENCES users (id) ON DELETE CASCADE,
    purpose     text NOT NULL CHECK (purpose IN ('register', 'login')),
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Secretos TOTP (MFA por app autenticadora) cifrados en reposo en prod.
CREATE TABLE totp_secrets (
    user_id     uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    secret      bytea NOT NULL,                           -- cifrar en reposo en prod
    confirmed   boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Auditoría inmutable (append-only). Sin datos personales sensibles en claro.
CREATE TABLE audit_log (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor_id    uuid,
    org_id      uuid,
    action      text NOT NULL,
    target      text,
    ip          inet,
    detail      jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_id);
CREATE INDEX audit_log_org_idx ON audit_log (org_id);

-- updated_at automático.
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER organizations_set_updated_at BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER oauth_clients_set_updated_at BEFORE UPDATE ON oauth_clients
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
