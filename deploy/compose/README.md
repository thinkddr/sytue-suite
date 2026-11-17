<a id="sytue-one--entorno-de-desarrollo-local-docker-compose"></a>
# Sytue One — LOCAL development environment (Docker Compose)

Brings up **the whole Sytue One suite** on your machine with a single command, under
`*.sytue.localhost` with local TLS (mkcert) via Traefik.

**Single stack:** Go (backend) + Astro 7 / Preact 10.29 with Bun (frontend) + **PostgreSQL**
(state, queues and cache) + **S3** (blobs; SeaweedFS in dev). Each product is an
**in-house Go service** (clean-room, Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr)) in `services/<svc>`.

**No AGPL engines** (Keycloak/Nextcloud/OnlyOffice/Mattermost/Stalwart are
**out**). **No Redis/Valkey.** The only third-party app: **LiveKit** (Apache-2.0)
as the media SFU of *Sytue Meet*. Dev/prod parity: same OIDC clients,
logical domains and managed services.

100% permissive · 100% EU. No real secrets: everything via `.env` (placeholders).

<a id="requisitos"></a>
## Requirements

- Docker + Docker Compose v2
- [`mkcert`](https://github.com/FiloSottile/mkcert) (trusted local TLS)
- `*.sytue.localhost` entries resolving to `127.0.0.1`. On most
  systems `*.localhost` already resolves to loopback; if not, add to `/etc/hosts`
  the hosts from the URL table (or use `dnsmasq`).

<a id="arranque-en-un-comando"></a>
## One-command startup

From the repo root:

```bash
make dev-up
```

In order, that does the equivalent of:

```bash
cd deploy/compose

# 1) Variables locales
cp -n .env.example .env            # rellena los valores changeme-*

# 2) Certificado TLS local de confianza (mkcert)
mkcert -install
mkcert -cert-file traefik/certs/sytue-localhost.pem \
       -key-file  traefik/certs/sytue-localhost-key.pem \
       "sytue.localhost" "*.sytue.localhost"

# 3) Levantar toda la pila (build local de servicios Go + frontends Astro)
docker compose up -d --build
```

Useful commands:

| Action | Command |
|---|---|
| Bring everything up | `make dev-up` |
| See status/health | `docker compose ps` |
| Logs of a service | `docker compose logs -f id` |
| Seed data (dev seed) | `make dev-seed` |
| Stop | `make dev-down` |
| Delete volumes (full reset) | `make dev-clean` |

> `make` is defined in the integration phase (repo root). If it does not exist yet,
> use the equivalent `docker compose` commands shown above.

<a id="memoria-local-perfiles-y-frontends-compilados-dev-only"></a>
## Local memory: profiles and compiled frontends (DEV-ONLY)

These settings are **development only**: they cap RAM so that the FULL
suite fits on an **8 GB** local machine (VM ~6 GiB). They **do not apply in prod** — prod
is deployed via **Helm** (`deploy/helm`) with its own *requests/limits*. The
Go runtime values (`GOMEMLIMIT`, `GOGC`, `GOMAXPROCS`, `DB_MAX_CONNS`) are
**environment variables**, not compiled in: the same image serves prod with
other values.

**1) `make dev-up` stays the same** (it brings up the full suite), but now each
container has a `mem_limit` and the Go runtime is tuned. `dev-up`/`dev-down`
read `COMPOSE_PROFILES` from `.env` (default `full`).

**2) On-demand profiles** — bring up only what you need. The **core**
(traefik, postgres, s3, dragonfly, id, hub) ALWAYS runs; each app hangs off its
profile:

| Profile | Services it adds to the core |
|---|---|
| `mail` | mail + mail-web |
| `drive` | drive + drive-web |
| `docs` | docs + docus-web + calcus-web + diapos-web + certipdf-web + ai |
| `calendar` | calendar + calendar-web |
| `contacts` | contacts + contacts-web |
| `rooms` | chat + chat-web + meet + livekit |
| `cloud` | cloud-api + cloud-builder + cloud-controller + postgrest + cloud-dashboard |
| `portal` | portal |
| `console` | console |
| `www` | www |
| `full` | **all** of the above (full suite) |

```bash
make dev-app P=docs,mail            # núcleo + docs + mail
COMPOSE_PROFILES=docs make dev-up   # equivalente para una sola app
```

**3) Compiled frontends (`frontlite.yml`)** — the big lever. In dev the 10
frontends run as `astro dev` (~450 MiB each ≈ 4.4 GiB). The
`frontlite.yml` overlay serves them **compiled** (SSR `runtime`, ~60-130 MiB each,
`mem_limit` 256m):

```bash
docker compose -f docker-compose.yml -f frontlite.yml up -d --build
```

It compiles 7 of the 10 (portal, mail, drive, calendar, contacts, docs, chat).
`console`, `cloud-dashboard` and `www` are **excluded** (they stay in dev mode): their
`@sytue/app-*` package name does not match the single `ARG APP` of the Dockerfile (which
reuses it for the bun filter **and** the `apps/<APP>/dist` path), and `www` is
`output: 'static'` (no SSR server). The header of `frontlite.yml` details
the usage and the *caveat* about the `PUBLIC_*` variables (they require declaring `ARG PUBLIC_*` in the
Dockerfile to be baked into the client; today it only declares `ARG APP`).

## URLs (dev, `*.sytue.localhost`)

All frontends are Astro+Preact; all backends are **Go services**.

| Surface | URL (Astro client) | Go service (API) | Go port |
|---|---|---|---|
| Sytue One (portal) | https://one.sytue.localhost | (uses Sytue ID + Hub) | — |
| Sytue ID (login/admin) | https://id.sytue.localhost | Sytue ID (OIDC/SCIM) | 8081 |
| Sytue Drive | https://drive.sytue.localhost | https://drive-dav.sytue.localhost (WebDAV) | 8082 |
| Sytue Docs | https://docs.sytue.localhost | https://docs-api.sytue.localhost (collab WS) | 8083 |
| **Sytue Rooms** (messaging + calls) | https://rooms.sytue.localhost | https://rooms-api.sytue.localhost (WS) | 8085 |
| Sytue Mail | https://mail.sytue.localhost | https://mail-api.sytue.localhost (JMAP) · https://mx.sytue.localhost | 8086 |
| Sytue Calendar | https://calendar.sytue.localhost | https://calendar-dav.sytue.localhost (CalDAV) | 8087 |
| Sytue Contacts | https://contacts.sytue.localhost | https://contacts-dav.sytue.localhost (CardDAV) | 8088 |
| Sytue Cloud (dashboard) | https://cloud.sytue.localhost | https://cloud-api.sytue.localhost | 8090 |
| Sytue Cloud builder | — (internal) | — | 8091 |
| Sytue Cloud DB (PostgREST) | https://db.sytue.localhost | PostgREST | — |
| Sytue Admin | https://admin.sytue.localhost | (admin on top of Sytue ID + Hub) | — |
| Sytue Hub | — | https://hub.sytue.localhost | 8092 |
| Public site (www) | https://sytue.localhost · https://www.sytue.localhost | — | — |
| Traefik dashboard | https://traefik.sytue.localhost | :8080 | — |
| S3 (SeaweedFS) | https://s3.sytue.localhost | :9000 | — |

**LiveKit (Sytue Meet media SFU, Apache-2.0):** WS signaling at
https://rooms-sfu.sytue.localhost. **Legacy hosts**: `chat.sytue.localhost` and
`meet.sytue.localhost` respond with a **permanent 301** to `rooms.sytue.localhost`,
preserving the path (Sytue Rooms unifies both products).
Ports published directly on the host (no Traefik):
`7880` (WS), `7881` (TCP media) and `50000-50100/udp` (WebRTC media).

Other ports published directly on the host: PostgreSQL `5432`, Sytue Mail SMTP/IMAP
`25/465/587/993`, Cloud builder `8091`. (Ports per `docs/CONVENTIONS.md §6`.)

> **Dev/prod parity.** Each product's **Astro+Preact client** occupies the canonical
> subdomain (`drive/calendar/contacts/...sytue.localhost`), just as in prod (`drive.sytue.eu`),
> and its BFF talks to the Go service over the internal network. To test the DAV protocols
> directly, each backend also exposes `*-dav.sytue.localhost` (WebDAV/CalDAV/CardDAV);
> the other APIs use their own subdomain (`docs-api`, `rooms-api`/`rooms-sfu`, `mail-api`/`mx`).

<a id="arquitectura-del-directorio"></a>
## Directory layout

```
deploy/compose/
├── docker-compose.yml         # toda la pila (servicios Go + frontends + infra)
├── frontlite.yml              # overlay DEV-ONLY: frontends SSR compilados (−RAM)
├── docker-compose.override.yml # LOCAL, no versionado (p.ej. fijar imagen de postgres)
├── .env.example               # placeholders dev (copiar a .env)
├── traefik/
│   ├── traefik.yml            # config estática (entrypoints, docker provider)
│   ├── dynamic/tls.yml        # TLS mkcert + middlewares
│   └── certs/                 # certificados locales (no versionados)
├── postgres/init/             # creación de UNA BD/rol por servicio Go
├── minio/minio-init.sh        # buckets S3 dev (drive/mail/docs blobs, cloud…)
└── livekit/livekit.yaml       # config del SFU LiveKit (Sytue Meet)
```

<a id="identidad-sytue-id-es-nuclear"></a>
## Identity (Sytue ID is CORE)

**Sytue ID** is an **in-house Go service** (`services/id`) that implements
OIDC/OAuth2 (Authorization Code + PKCE, refresh, JWKS, discovery), SCIM and
MFA/WebAuthn. Each Go service **validates Sytue ID JWTs** via JWKS
(`authn` middleware from `platform`). The frontends use `@sytue/auth` (OIDC+PKCE)
with a single session in an httpOnly cookie on the `.sytue.localhost` domain.

**Sytue Meet:** the `meet` BFF mints the LiveKit JWT (API key/secret from
`.env`) **after** validating the OIDC session.

<a id="datos-y-blobs"></a>
## Data and blobs

- **PostgreSQL** is the only datastore: state, queues and cache (LISTEN/NOTIFY,
  `pgmq`/`riverqueue`). One **DB + role per service** (isolation); multi-tenant
  via schema/RLS inside each DB. **No Redis/Valkey.**
- **S3 (SeaweedFS in dev)** only for large blobs: Drive files, Mail
  attachments, Docs documents, PaaS build cache, backups.

<a id="notas"></a>
## Notes

- **TLS**: install the mkcert CA (`mkcert -install`) so the browser
  trusts `*.sytue.localhost`.
- **Go service builds**: the context is the **repo root** (single module
  `git.sytue.eu/sytue/one`); the `Dockerfile` lives in each `services/<svc>/`
  (multi-stage `golang:1.23` → `distroless/static`).
- **Reset**: `make dev-clean` deletes volumes (DB, buckets). After a reset, the
  per-service DBs and the buckets are recreated on startup.
```
