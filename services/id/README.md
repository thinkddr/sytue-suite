<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos -->

# Sytue ID

**Core** identity provider of **Sytue One** (`id.sytue.eu`, dev port **8081**).
Sytue's own **Go** service implementing **OIDC/OAuth2**, **SCIM 2.0** and **MFA/WebAuthn**
on top of **PostgreSQL** (single datastore, no Redis/Valkey).

> ⚠️ **SECURITY NOTICE — MANDATORY AUDIT BEFORE PRODUCTION.**
> Sytue ID is the suite's critical authentication component. In accordance with
> `docs/CONVENTIONS.md` §0.7 / §8 / §10, **it must pass an independent security
> audit** (OWASP ASVS, threat modeling, review of the MFA state binding and of
> the full WebAuthn attestation verification) **before being deployed to
> production**. This implementation is clean-room and functional, but it is NOT
> audited yet.

<a id="alcance-clean-room-estándares-abiertos"></a>
## Scope (clean-room, open standards)

Original reimplementation on top of **OIDC Core 1.0**, **OAuth2** (RFC 6749/6750/7636/7009),
**OIDC Discovery** (RFC 8414), **JOSE** (RFC 7515/7517/7519), **SCIM 2.0**
(RFC 7643/7644) and **WebAuthn L2**, with **permissive** libraries (jwx — MIT;
pgx — MIT; chi — MIT; x/crypto — BSD). It does **not** derive from Keycloak or from any
AGPL engine (`docs/CONVENTIONS.md` §3, §12). SPDX header Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr).

<a id="funcionalidad"></a>
### Features

- **OAuth2 / OIDC**
  - Authorization Code + **PKCE** (S256), `refresh_token` with **rotation and
    reuse detection** (revokes the whole family), `client_credentials`.
  - Signed JWT `id_token` and `access_token` (RS256) with **JWKS** and key rotation.
  - `/.well-known/openid-configuration`, `/.well-known/jwks.json`, `userinfo`,
    `revoke` (RFC 7009), `logout` (RP-Initiated).
- **Identity / tenancy**: users (global login by email), organizations
  (`org/<slug>`), **roles** `owner|admin|member|billing`, groups. MFA is mandatory
  for `owner`/`admin` (step-up).
- **SCIM 2.0**: `User` resource (list/create/get/replace/patch/delete) +
  `ServiceProviderConfig`/`ResourceTypes`/`Schemas`, protected by a static bearer.
- **MFA / WebAuthn (passkeys)**: registration (`begin`/`finish`) with verification of
  `clientDataJSON`, origin, `rpIdHash`, flags and CBOR parsing of the `attestationObject`.

<a id="estructura"></a>
## Structure

```
services/id/
├── cmd/id/main.go              # entrypoint (migraciones + bootstrap claves + HTTP)
├── internal/
│   ├── domain/                 # tipos e invariantes (Argon2id, PKCE, tokens)
│   ├── store/                  # PostgreSQL con pgx (consultas parametrizadas)
│   ├── oidc/                   # claves/JWKS, emisión de tokens, discovery
│   ├── scim/                   # SCIM 2.0
│   ├── webauthn/               # parseo WebAuthn + CBOR mínimo
│   └── http/                   # router chi, endpoints, middlewares, sesión SSO
├── db/migrations/              # 0001 esquema, 0002 seed de clients OIDC
├── openapi.yaml
├── Dockerfile                  # multi-stage golang:1.26 -> distroless/static
└── README.md
```

<a id="clients-oidc-sembrados-dbmigrations0002_seed_clientsupsql"></a>
## Seeded OIDC clients (`db/migrations/0002_seed_clients.up.sql`)

`sytue-one`, `sytue-console`, `sytue-cloud` (+`sytue-cloud-api`), `sytue-mail`,
`sytue-drive`, `sytue-calendar`, `sytue-contacts`, `sytue-docus`, `sytue-calcus`, `sytue-diapos`, `sytue-certipdf`, `sytue-chat`,
`sytue-meet`, `sytue-hub` — with `redirect_uri` to `*.sytue.eu`, `*.sytue.localhost`
and `localhost:43xx`. **Secrets are NOT seeded** (never secrets in git): the
`secret_hash` stays `NULL` and the token endpoint rejects the client until the
deploy sets its secret (Secret Manager / console).

<a id="configuración-entorno-prefijo-sytue_id_"></a>
## Configuration (environment, `SYTUE_ID_` prefix)

| Variable | Default | Description |
|---|---|---|
| `SYTUE_ID_DATABASE_DSN` | — (required) | PostgreSQL DSN |
| `SYTUE_ID_HTTP_ADDR` | `:8081` | listen address |
| `SYTUE_ID_ISSUER` | `https://id.sytue.localhost` | OIDC issuer (no trailing slash) |
SSO cookies are host-only (`__Host-` over HTTPS). `SYTUE_ID_COOKIE_DOMAIN` is
rejected: sharing a cookie with `.sytue.eu` would expose it to Cloud publications.
| `SYTUE_ID_COOKIE_SECURE` | `true` | Secure flag (false only in http dev) |
| `SYTUE_ID_TRUSTED_PROXIES` | private networks | CIDR/IP (CSV) whose `X-Forwarded-For` is trusted; `none` = none |
| `SYTUE_ID_ACCESS_TOKEN_TTL` | `15m` | access token lifetime |
| `SYTUE_ID_ID_TOKEN_TTL` | `1h` | ID token lifetime |
| `SYTUE_ID_REFRESH_TOKEN_TTL` | `720h` | refresh token lifetime |
| `SYTUE_ID_AUTH_CODE_TTL` | `60s` | authorization code lifetime |
| `SYTUE_ID_SCIM_BEARER_TOKEN` | — | SCIM provisioner token (Secret Manager) |
| `SYTUE_ID_WEBAUTHN_RP_ID` | `sytue.localhost` | Relying Party ID |
| `SYTUE_ID_WEBAUTHN_RP_ORIGINS` | issuer | allowed origins (CSV) |

No secrets in code or logs (`docs/CONVENTIONS.md` §0.7). In prod, all
secrets come from **Scaleway Secret Manager**.

<a id="seguridad-por-defecto"></a>
## Secure by default

Argon2id for passwords and client secrets; opaque tokens **hashed**
(SHA-256) at rest (only the hash is persisted); mandatory PKCE S256; **exact**
validation of `redirect_uri` and `post_logout_redirect_uri` (anti open-redirect,
also in dynamic registration); `return_to` restricted to internal paths
—rejecting `\` and control characters, which the browser reinterprets—;
`state`/`nonce`; refresh rotation with reuse detection; host-only
`HttpOnly`/`Secure`/`SameSite=Lax` cookies; origin verification
(`Sec-Fetch-Site`/`Origin`) on **all** form POSTs, login included;
strict CSP and security headers; immutable audit log; parameterized
queries.

**Client IP.** `X-Forwarded-For` is only trusted when the connecting peer is in
`SYTUE_ID_TRUSTED_PROXIES`, and within the header it is read from right to left
(see `domain.TrustedProxies`). The per-IP buckets of the login and of
`/oauth2/*`, the per-IP Conditional Access conditions and the IP in the audit
log all depend on it: reading the header blindly made them spoofable from outside.

<a id="notas-pendientes-para-la-auditoría"></a>
## Pending notes for the audit

- Complete the WebAuthn **attestation** formats (`packed`/`tpm`/…); the
  current registration uses `attestation: none`. **Assertion** verification
  (login with a passkey) is complete: type, challenge, origin, `rpIdHash`, flags,
  `signCount` and ES256/RS256 signature.
- The second-factor state binding (`/login/mfa`), encryption at rest of
  `signing_keys.private_pem` and `totp_secrets.secret`, and rate limiting of
  `/login` and `/oauth2/token` **are already implemented** (migrations 0003 and 0005;
  `beginMFAStepUp`/`ConsumeMFAContinuation`; `pg.NewRateLimiter`).
