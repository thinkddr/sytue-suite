# Sytue Contacts (`services/contacts`)

**Sytue-native** contacts server of the **Sytue One** suite: address
books, contacts and groups. A **clean-room** reimplementation on open standards
(**vCard 4.0 — RFC 6350** and **CardDAV — RFC 6352**) with its **own REST
API**. Code Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr); permissive third-party dependencies only
(BSD/MIT/Apache): `emersion/go-webdav` (CardDAV) and `emersion/go-vcard` (MIT).

- **Port:** `8088` (HTTP + CardDAV) · **Subdomain:** `contacts.sytue.eu`
- **Datastore:** PostgreSQL (address books, cards, contacts, groups, event
  outbox) — single source of truth. **No Redis/Valkey. No Object Storage**
  (photos are referenced by URL, not embedded).
- **Auth:** **Sytue ID** JWT validated via JWKS (`platform/pkg/authn`).
- **Multi-tenant:** isolation via **RLS** in PostgreSQL (`app.tenant`).

<a id="capacidades"></a>
## Capabilities

- **Address books (CardDAV collections):** per user/tenant, a self-managed default
  address book, create / rename / delete (soft); `ctag` +
  `sync_token` for incremental synchronization (RFC 6578).
- **Contacts (vCard 4.0):** create / read / update (conditional on
  **If-Match**/ETag) / delete; normalized projection (N, FN, ORG, TITLE,
  NICKNAME, BDAY, NOTE) plus **emails** and **phones** with `TYPE`/`PREF`. A BDAY
  without a year (`--0215`, or Apple's placeholder year) goes in `birthday_no_year`. Web
  sites (`urls`, http/https only), related people (`related`, text RELATED) and
  anniversary (`anniversary`, full date); whatever is not projected is preserved.
  Export of the canonical vCard (`text/vcard; version=4.0`).
- **Groups (vCard `KIND=group`):** members by `UID` (`MEMBER` property,
  RFC 6350 6.6.5), create / read / update / delete.
- **Search:** full-text (`tsvector` GIN) over name, organization, nickname and
  notes, within the tenant (optionally scoped to one address book).
- **CardDAV (RFC 6352):** mounted at `/dav` with the same Bearer JWT, on top of
  `emersion/go-webdav/carddav`. Discovery via `/.well-known/carddav`
  (RFC 6764). Compatible with standard clients (iOS/macOS, Thunderbird, Evolution,
  DAVx⁵).
- **Events:** publishing to the **outbox** on PostgreSQL (`platform/pkg/events`).

<a id="estructura"></a>
## Structure

```
services/contacts/
├── cmd/contacts/main.go         # binario (serve | migrate | healthcheck)
├── internal/
│   ├── config/                  # carga de configuración (env, 12-factor)
│   ├── domain/                  # tipos, reglas y códec vCard 4.0 (sin transporte)
│   ├── store/                   # persistencia pgx + RLS por tenant; CardDAV store
│   ├── carddav/                 # carddav.Backend (emersion/go-webdav) sobre store
│   └── http/                    # router chi, handlers REST, export vCard
├── db/migrations/               # migraciones golang-migrate (.up/.down.sql)
├── openapi.yaml                 # contrato de la API REST
└── Dockerfile                   # multi-stage golang:1.26 -> distroless/static
```

<a id="configuración-variables-de-entorno"></a>
## Configuration (environment variables)

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8088` | HTTP/CardDAV port |
| `PUBLIC_URL` | `https://contacts.sytue.localhost` | Public URL |
| `DATABASE_URL` | — (required) | PostgreSQL DSN |
| `DB_MAX_CONNS` | `10` | Maximum pgx pool size |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Sytue ID issuer |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | JWKS endpoint |
| `OIDC_AUDIENCE` | `sytue-contacts` | Expected token audience |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | Structured logging |

There are no secrets in the repository: credentials arrive via the environment
(Scaleway Secret Manager / Sealed Secrets in prod).

<a id="ejecución"></a>
## Running

```sh
# Migraciones + servidor (las migraciones se aplican también al arrancar).
go run ./services/contacts/cmd/contacts

# Solo migrar (Job de despliegue).
go run ./services/contacts/cmd/contacts migrate
```

In the full suite it is brought up with `make dev-up` (Docker Compose); the image's build
context is the **repo root** (single module `git.sytue.eu/sytue/one`).

## Endpoints

REST under `/api/v1` (see [`openapi.yaml`](./openapi.yaml)); CardDAV under `/dav`;
discovery at `/.well-known/carddav`; `/healthz` and `/readyz` probes.

### CardDAV (RFC 6352)

The resource tree follows the standard model:

```
/dav/principals/{sub}/                      # principal del usuario
/dav/addressbooks/{sub}/                     # home set de libretas
/dav/addressbooks/{sub}/{libreta}/           # colección (libreta)
/dav/addressbooks/{sub}/{libreta}/{uid}.vcf  # recurso vCard (contacto o grupo)
```

Supports `PROPFIND`, `REPORT` (`addressbook-query`, `addressbook-multiget`,
`sync-collection`), `GET`/`PUT`/`DELETE` with `If-Match`/`If-None-Match`, and
extended `MKCOL` to create address books.

<a id="seguridad"></a>
## Security

Deny-by-default: the whole API and the CardDAV tree require a Sytue ID JWT;
isolation between tenants is guaranteed by **RLS** in PostgreSQL (session variable
`app.tenant` set per transaction). Queries are always parameterized; bodies are
limited; maximum vCard size is capped (`max-resource-size`); writes are
conditional on ETag. No personal data in logs.
