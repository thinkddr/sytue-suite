# Sytue Drive (`services/drive`)

**Sytue-native** file server for the **Sytue One** suite: file storage,
sync and sharing. A **clean-room** reimplementation on
open standards (**WebDAV — RFC 4918**) with its **own REST API**. Code
Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr); third-party dependencies are permissive only (BSD/MIT/Apache).

- **Port:** `8082` (HTTP + WebDAV) · **Subdomain:** `drive.sytue.eu`
- **Datastore:** PostgreSQL (metadata, quotas, event outbox) — single source
  of truth. **No Redis/Valkey.**
- **Blobs:** S3 Object Storage (Scaleway prod / MinIO dev), bucket `drive-blobs`,
  isolated by **tenant prefix**.
- **Auth:** **Sytue ID** JWT validated via JWKS (`platform/pkg/authn`).
- **Multi-tenant:** isolation through **RLS** in PostgreSQL (`app.tenant`) + per-tenant S3
  prefix.

<a id="capacidades"></a>
## Capabilities

- **Folders:** per-tenant tree, self-managed root, create / rename / move
  (with cycle detection) / delete (soft, whole subtree).
- **Files:** upload (streaming to S3, SHA-256 checksum, quota reservation),
  download (stream or redirect to a **presigned** URL), rename, move,
  delete, **versioning** on replace.
- **Group folders:** organization-level shared folders (`is_group`).
- **Sharing:** public links with a high-entropy **token** (optional
  expiry), and by Sytue ID **user**/**group**, with `read`/`write` permission.
- **Search:** by file name (case-insensitive) within the tenant.
- **Per-tenant quotas:** limit and usage maintained transactionally; readable by
  any member, limit changes only by `admin`/`owner`. The plan quota is
  **one for the whole suite** and Drive owns it: Sytue Cast declares what it takes up in
  each organization via `PUT /internal/quota/external/cast` (shared secret
  `CAST_INTERNAL_SECRET`, absolute and idempotent value) and `used_bytes` adds it, with
  the breakdown in `external_usage`.
- **WebDAV (RFC 4918):** mounted at `/dav` with the same Bearer JWT
  (`golang.org/x/net/webdav`, BSD-3-Clause).
- **Events:** published to the **outbox** on PostgreSQL (`platform/pkg/events`).

<a id="estructura"></a>
## Structure

```
services/drive/
├── cmd/drive/main.go            # binario (serve | migrate | healthcheck)
├── internal/
│   ├── config/                  # carga de configuración (env, 12-factor)
│   ├── domain/                  # tipos y reglas de negocio (sin transporte)
│   ├── store/                   # persistencia pgx + RLS por tenant
│   ├── service/                 # casos de uso (store + S3 + eventos) y rutas WebDAV
│   ├── http/                    # router chi, handlers REST, descarga pública
│   └── webdav/                  # webdav.FileSystem propio sobre el servicio
├── db/migrations/               # migraciones golang-migrate (.up/.down.sql)
├── openapi.yaml                 # contrato de la API REST
└── Dockerfile                   # multi-stage golang:1.26 -> distroless/static
```

<a id="configuración-variables-de-entorno"></a>
## Configuration (environment variables)

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8082` | HTTP/WebDAV port |
| `PUBLIC_URL` | `https://drive.sytue.localhost` | Public URL (sharing links) |
| `DATABASE_URL` | — (required) | PostgreSQL DSN |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Sytue ID issuer |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | JWKS endpoint |
| `OIDC_AUDIENCE` | `sytue-drive` | Expected token audience |
| `S3_ENDPOINT` | `minio:9000` | S3 endpoint (accepts `http(s)://host:port`) |
| `S3_REGION` | `fr-par` | S3 region (EU only) |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | — (required) | S3 credentials |
| `S3_BUCKET` | `sytue-dev-drive-blobs` | Blob bucket |
| `S3_USE_SSL` | `false` | TLS to S3 (`true` in prod) |
| `MAX_UPLOAD_MB` | `5120` | Maximum upload size per file |
| `DEFAULT_QUOTA_GB` | `10` | Initial quota per tenant |
| `CAST_INTERNAL_SECRET` | dev: `dev-cast-internal-secret`; elsewhere, empty | Secret shared with Sytue Cast to declare its quota usage (optional; empty = the door does not open) |
| `PRESIGN_TTL` | `15m` | Validity of presigned URLs |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | Structured logging |

There are no secrets in the repository: credentials arrive via the environment
(Scaleway Secret Manager / Sealed Secrets in prod).

<a id="ejecución"></a>
## Running

```sh
# Migraciones + servidor (las migraciones se aplican también al arrancar).
go run ./services/drive/cmd/drive

# Solo migrar (Job de despliegue).
go run ./services/drive/cmd/drive migrate
```

In the full suite it is brought up with `make dev-up` (Docker Compose); the image build
context is the **repo root** (single module `git.sytue.eu/sytue/one`).

## Endpoints

REST under `/api/v1` (see [`openapi.yaml`](./openapi.yaml)); WebDAV under `/dav`;
public link download at `/public/{token}`; `/healthz` and `/readyz` probes.

<a id="seguridad"></a>
## Security

Deny-by-default: the whole API requires a Sytue ID JWT; isolation between tenants
is guaranteed by RLS in PostgreSQL (plus the S3 prefix). Queries are always
parameterized; bodies are size-limited; `X-Content-Type-Options: nosniff` headers
on downloads; link tokens with 256 bits of entropy, resolved through a
narrowly scoped `SECURITY DEFINER` function. Public links can expire and
be revoked.
