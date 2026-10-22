-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Federación OIDC upstream: Sytue ID actúa de broker/RP para que una empresa
-- inicie sesión en la suite con su IdP corporativo (Google Workspace, Microsoft
-- EntraID, cualquier OIDC). Tres tablas:
--
--   identity_providers   configuración por-organización de un IdP OIDC upstream.
--   federated_identities enlace (provider, sub externo) ⇄ usuario Sytue.
--   federated_auth_states estado efímero del flujo Authorization Code (RP).
--
-- AISLAMIENTO POR TENANT: igual que el resto del esquema de Sytue ID (mig 0001:
-- «Sytue ID es la fuente de verdad; RLS aguas arriba en los consumidores»), el
-- aislamiento se aplica a NIVEL DE APLICACIÓN — cada consulta se acota por org_id
-- y la autorización admin se re-verifica por-org (fail-closed, anti-IDOR), igual
-- que memberships/groups/org_invitations/audit_log. No se usa RLS de Postgres en
-- este servicio (ningún otro objeto lo hace; introducirla exigiría GUCs de sesión
-- por-conexión que el pool compartido no fija). El FK org_id + el UNIQUE(org,slug)
-- + el scoping por org de la capa store SON la barrera de tenant, con tests que
-- prueban la denegación cross-org.
--
-- SEGURIDAD: client_secret se cifra en reposo con la KEK/AES-256-GCM existente
-- (keycrypto, encv1:…) — la columna guarda el sobre, NUNCA el secreto en claro; el
-- endpoint admin solo lo ESCRIBE, jamás lo devuelve. Los endpoints descubiertos se
-- cachean tras un discovery SSRF-safe (solo https, IP pública).

-- Proveedor de identidad OIDC upstream configurado por una organización.
CREATE TABLE identity_providers (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    type          text NOT NULL DEFAULT 'oidc' CHECK (type IN ('oidc')),
    name          text NOT NULL,
    -- slug legible por-org (único por organización, ver índice abajo). El handle
    -- público del flujo RP es el UUID (globalmente inequívoco), no el slug.
    slug          text NOT NULL
                  CHECK (slug ~ '^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$'),
    issuer        text NOT NULL,
    client_id     text NOT NULL,
    -- Sobre AES-256-GCM (encv1:…) del client_secret; '' para client público (PKCE
    -- puro). NUNCA texto plano. Se descifra solo en el intercambio de código.
    client_secret text NOT NULL DEFAULT '',
    scopes        text NOT NULL DEFAULT 'openid email profile',
    -- Mapeo de atributos: nombres de claim del upstream para email/name/groups…
    attribute_mapping jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- Dominios de correo permitidos: acota a qué emails puede aprovisionar/enlazar
    -- este provider (defensa anti-abuso). Vacío ⇒ sin restricción de dominio.
    allowed_email_domains text[] NOT NULL DEFAULT '{}',
    -- Endpoints cacheados del discovery (<issuer>/.well-known/openid-configuration).
    authorization_endpoint text NOT NULL DEFAULT '',
    token_endpoint         text NOT NULL DEFAULT '',
    userinfo_endpoint      text NOT NULL DEFAULT '',
    jwks_uri               text NOT NULL DEFAULT '',
    -- El upstream anuncia PKCE S256 (code_challenge_methods_supported).
    pkce          boolean NOT NULL DEFAULT true,
    enabled       boolean NOT NULL DEFAULT false,
    created_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
-- slug único POR ORGANIZACIÓN (dos orgs pueden usar el mismo slug legible).
CREATE UNIQUE INDEX identity_providers_org_slug_key ON identity_providers (org_id, slug);
-- Enrutado de login por dominio: resolver el provider activo cuyo
-- allowed_email_domains contiene el dominio del email (operador @>), por Index
-- Scan GIN sobre el array, sin Seq Scan, a escala.
CREATE INDEX identity_providers_domains_gin
    ON identity_providers USING gin (allowed_email_domains) WHERE enabled;

-- Enlace entre el sujeto externo del IdP y el usuario Sytue. Único por
-- (provider, sujeto externo): un mismo sub del upstream mapea a un solo usuario.
CREATE TABLE federated_identities (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id      uuid NOT NULL REFERENCES identity_providers (id) ON DELETE CASCADE,
    external_subject text NOT NULL,
    user_id          uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, external_subject)
);
-- Búsqueda inversa (usuario ⇒ sus identidades federadas) y cascada por usuario.
CREATE INDEX federated_identities_user_idx ON federated_identities (user_id);

-- Estado efímero del flujo Authorization Code contra el IdP upstream. Solo se
-- persiste el HASH SHA-256 del state (el valor viaja en cookie httpOnly + URL,
-- doble control anti-CSRF). Lleva el nonce y el code_verifier PKCE (un solo uso,
-- vida corta) y el return_to del flujo OIDC en curso. Se BORRA al consumirlo
-- (anti-replay), como los códigos de autorización.
CREATE TABLE federated_auth_states (
    state_hash    text PRIMARY KEY,
    provider_id   uuid NOT NULL REFERENCES identity_providers (id) ON DELETE CASCADE,
    nonce         text NOT NULL,
    code_verifier text NOT NULL DEFAULT '',
    return_to     text NOT NULL DEFAULT '',
    expires_at    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
-- Purga periódica de estados caducados por Index Scan (CleanupExpired).
CREATE INDEX federated_auth_states_expires_idx ON federated_auth_states (expires_at);
