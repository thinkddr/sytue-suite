<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos -->

# Sytue ID

Proveedor de identidad **nuclear** de **Sytue One** (`id.sytue.eu`, puerto dev **8081**).
Servicio **Go** propio que implementa **OIDC/OAuth2**, **SCIM 2.0** y **MFA/WebAuthn**
sobre **PostgreSQL** (datastore único, sin Redis/Valkey).

> ⚠️ **AVISO DE SEGURIDAD — AUDITORÍA OBLIGATORIA ANTES DE PRODUCCIÓN.**
> Sytue ID es la pieza de autenticación crítica de la suite. Conforme a
> `docs/CONVENTIONS.md` §0.7 / §8 / §10, **debe superar una auditoría de
> seguridad independiente** (OWASP ASVS, modelado de amenazas, revisión del
> binding de estado MFA y de la verificación completa de attestation WebAuthn)
> **antes de desplegarse en producción**. Esta implementación es clean-room y
> funcional, pero NO está aún auditada.

## Alcance (clean-room, estándares abiertos)

Reimplementación original sobre **OIDC Core 1.0**, **OAuth2** (RFC 6749/6750/7636/7009),
**OIDC Discovery** (RFC 8414), **JOSE** (RFC 7515/7517/7519), **SCIM 2.0**
(RFC 7643/7644) y **WebAuthn L2**, con librerías **permisivas** (jwx — MIT;
pgx — MIT; chi — MIT; x/crypto — BSD). **No** deriva de Keycloak ni de ningún
motor AGPL (`docs/CONVENTIONS.md` §3, §12). Cabecera SPDX Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr).

### Funcionalidad

- **OAuth2 / OIDC**
  - Authorization Code + **PKCE** (S256), `refresh_token` con **rotación y
    detección de reuso** (revoca la familia completa), `client_credentials`.
  - `id_token` y `access_token` JWT firmados (RS256) con **JWKS** y rotación de claves.
  - `/.well-known/openid-configuration`, `/.well-known/jwks.json`, `userinfo`,
    `revoke` (RFC 7009), `logout` (RP-Initiated).
- **Identidad / tenancy**: usuarios (login global por email), organizaciones
  (`org/<slug>`), **roles** `owner|admin|member|billing`, grupos. MFA obligatorio
  para `owner`/`admin` (step-up).
- **SCIM 2.0**: recurso `User` (list/create/get/replace/patch/delete) +
  `ServiceProviderConfig`/`ResourceTypes`/`Schemas`, protegido por bearer estático.
- **MFA / WebAuthn (passkeys)**: registro (`begin`/`finish`) con verificación de
  `clientDataJSON`, origen, `rpIdHash`, flags y parseo CBOR del `attestationObject`.

## Estructura

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

## Clients OIDC sembrados (`db/migrations/0002_seed_clients.up.sql`)

`sytue-one`, `sytue-console`, `sytue-cloud` (+`sytue-cloud-api`), `sytue-mail`,
`sytue-drive`, `sytue-calendar`, `sytue-contacts`, `sytue-docus`, `sytue-calcus`, `sytue-diapos`, `sytue-certipdf`, `sytue-chat`,
`sytue-meet`, `sytue-hub` — con `redirect_uri` a `*.sytue.eu`, `*.sytue.localhost`
y `localhost:43xx`. **Los secretos NO se siembran** (nunca secretos en git): el
`secret_hash` queda `NULL` y el token endpoint rechaza al client hasta que el
deploy fije su secreto (Secret Manager / consola).

## Configuración (entorno, prefijo `SYTUE_ID_`)

| Variable | Por defecto | Descripción |
|---|---|---|
| `SYTUE_ID_DATABASE_DSN` | — (obligatorio) | DSN PostgreSQL |
| `SYTUE_ID_HTTP_ADDR` | `:8081` | dirección de escucha |
| `SYTUE_ID_ISSUER` | `https://id.sytue.localhost` | issuer OIDC (sin barra final) |
Las cookies SSO son host-only (`__Host-` en HTTPS). `SYTUE_ID_COOKIE_DOMAIN` se
rechaza: compartir una cookie con `.sytue.eu` la expondría a publicaciones Cloud.
| `SYTUE_ID_COOKIE_SECURE` | `true` | flag Secure (false solo dev http) |
| `SYTUE_ID_TRUSTED_PROXIES` | redes privadas | CIDR/IP (CSV) cuyo `X-Forwarded-For` se cree; `none` = ninguno |
| `SYTUE_ID_ACCESS_TOKEN_TTL` | `15m` | vida del access token |
| `SYTUE_ID_ID_TOKEN_TTL` | `1h` | vida del ID token |
| `SYTUE_ID_REFRESH_TOKEN_TTL` | `720h` | vida del refresh token |
| `SYTUE_ID_AUTH_CODE_TTL` | `60s` | vida del código de autorización |
| `SYTUE_ID_SCIM_BEARER_TOKEN` | — | token del provisioner SCIM (Secret Manager) |
| `SYTUE_ID_WEBAUTHN_RP_ID` | `sytue.localhost` | Relying Party ID |
| `SYTUE_ID_WEBAUTHN_RP_ORIGINS` | issuer | orígenes permitidos (CSV) |

Sin secretos en código ni logs (`docs/CONVENTIONS.md` §0.7). En prod, todos los
secretos provienen de **Scaleway Secret Manager**.

## Seguridad por defecto

Argon2id para contraseñas y secretos de client; tokens opacos **hasheados**
(SHA-256) en reposo (solo el hash se persiste); PKCE S256 obligatorio; validación
**exacta** de `redirect_uri` y de `post_logout_redirect_uri` (anti open-redirect,
también en el registro dinámico); `return_to` restringido a rutas internas
—rechazando `\` y caracteres de control, que el navegador reinterpreta—;
`state`/`nonce`; rotación de refresh con detección de reuso; cookies
`HttpOnly`/`Secure`/`SameSite=Lax` host-only; verificación de origen
(`Sec-Fetch-Site`/`Origin`) en **todos** los POST de formulario, login incluido;
CSP estricta y cabeceras de seguridad; auditoría inmutable; consultas
parametrizadas.

**IP del cliente.** `X-Forwarded-For` solo se cree cuando quien conecta está en
`SYTUE_ID_TRUSTED_PROXIES`, y dentro de la cabecera se lee de derecha a izquierda
(ver `domain.TrustedProxies`). De ahí cuelgan los buckets por IP del login y de
`/oauth2/*`, las condiciones por IP del Acceso Condicional y la IP del registro de
auditoría: leer la cabecera sin más los hacía falsificables desde fuera.

## Notas pendientes para la auditoría

- Completar los formatos de **attestation** WebAuthn (`packed`/`tpm`/…); el
  registro actual usa `attestation: none`. La verificación de **assertion**
  (login con passkey) sí está completa: tipo, reto, origen, `rpIdHash`, flags,
  `signCount` y firma ES256/RS256.
- El binding de estado del segundo factor (`/login/mfa`), el cifrado en reposo de
  `signing_keys.private_pem` y `totp_secrets.secret`, y el rate-limiting de
  `/login` y `/oauth2/token` **ya están implementados** (migraciones 0003 y 0005;
  `beginMFAStepUp`/`ConsumeMFAContinuation`; `pg.NewRateLimiter`).
