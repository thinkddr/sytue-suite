# Sytue Hub (`services/hub`)

**Seamless integration** backend for the **Sytue One** suite (the "shell backend"
that backs `@sytue/shell`). It brings together four cross-cutting capabilities on top of the
product services, **without** business data of its own: **federated search**,
**unified notifications**, **presence** and **cross-app intents**.

Code Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr); third-party dependencies are permissive only
(MIT/BSD/Apache). Clean-room on open standards (REST, **SSE** —
WHATWG EventSource, PostgreSQL **LISTEN/NOTIFY**). No AGPL engines; no
Redis/Valkey.

- **Port:** `8092` (HTTP) · **Subdomain:** `hub.sytue.eu`
- **Datastore:** PostgreSQL (notifications, presence, fan-out via LISTEN/NOTIFY)
  — single source of truth. **No Redis/Valkey.**
- **Auth:** **Sytue ID** JWT validated via JWKS (`platform/pkg/authn`), OIDC
  client `sytue-hub`.
- **Multi-tenant:** isolation through **RLS** in PostgreSQL (`app.tenant`).

<a id="capacidades"></a>
## Capabilities

<a id="búsqueda-federada"></a>
### Federated search
Queries **drive / mail / calendar / contacts / chat** in parallel through
typed connectors (`internal/connectors/*`), **on behalf of the user** (it forwards
their Bearer JWT, so each service applies its own authorization). Each
connector translates its service's model into the **canonical** `SearchHit` model
(source, kind, id, title, snippet, url, icon, timestamp). It degrades gracefully:
a slow or down source is reported as a per-source **partial error** without breaking the
aggregation; each fan-out has its own timeout.

<a id="notificaciones-unificadas"></a>
### Unified notifications
Per-user notices, **durable** in PostgreSQL, with **real-time** delivery via
**SSE** over **LISTEN/NOTIFY** (channel `sytue_hub_notifications`). Creation
inserts and emits `pg_notify` in the **same transaction** (the notice only travels if the
write commits). One `Relay` per process listens on the channel and fans out to the
local SSE subscriptions, filtering by `(tenant, subject)`. Endpoints for listing
(keyset), unread count, mark read / mark all, and `GET .../stream` (SSE).

<a id="presencia"></a>
### Presence
Per-user heartbeat (upsert); the **offline** state is **derived** at read time
by comparing `last_seen` with the presence **TTL** (no job is needed to "drop off").
Ephemeral state in PostgreSQL (no Redis/Valkey). Aggregated query by list of
subjects so the shell can paint presence indicators.

<a id="intents-cross-app"></a>
### Cross-app intents
Actions that connect two products and return **deep links** (the shell
opens them; Hub does **not** run the action — the target service applies authorization and
execution):
- `attach-drive-to-mail` — Mail draft with Drive files attached.
- `create-event-from-mail` — Calendar composer prefilled from an email.
- `share-to-chat` — share a resource (Drive/Mail/Calendar/Contacts) in Chat.

<a id="estructura"></a>
## Structure

```
services/hub/
├── cmd/hub/main.go              # binario (serve | migrate | healthcheck)
├── internal/
│   ├── config/                  # carga de configuración (env, 12-factor)
│   ├── domain/                  # tipos y reglas (search/notif/presence/intents)
│   ├── connectors/              # conectores tipados a drive/mail/calendar/...
│   ├── search/                  # fan-out federado + agregación/ranking
│   ├── notify/                  # creación durable + SSE Hub + Relay LISTEN/NOTIFY
│   ├── presence/                # heartbeat + consulta con derivación por TTL
│   ├── intents/                 # resolutor de intents -> deep-links
│   ├── store/                   # persistencia pgx (notifications, presence) + RLS
│   └── http/                    # router chi, handlers, stream SSE
├── db/migrations/               # migraciones (golang-migrate)
├── openapi.yaml                 # contrato de la API REST + SSE
└── Dockerfile                   # multi-stage -> distroless (rootless)
```

<a id="api-resumen"></a>
## API (summary)

| Method | Route | Description |
|---|---|---|
| `GET`  | `/api/v1/search?q=&sources=&limit=` | Federated search |
| `GET`  | `/api/v1/notifications` | List (keyset: `unread`, `before`, `limit`) |
| `GET`  | `/api/v1/notifications/unread_count` | Unread count |
| `POST` | `/api/v1/notifications` | Emit (scope `notifications:write` for third parties) |
| `GET`  | `/api/v1/notifications/stream` | Real-time SSE stream |
| `POST` | `/api/v1/notifications/{id}/read` | Mark read |
| `POST` | `/api/v1/notifications/read_all` | Mark all read |
| `GET`  | `/api/v1/presence?subjects=` | Query presence |
| `PUT`  | `/api/v1/presence` | Heartbeat |
| `DELETE` | `/api/v1/presence` | Mark offline |
| `POST` | `/api/v1/intents` | Resolve intent -> deep links |
| `GET`  | `/healthz` · `/readyz` | Probes |

<a id="configuración-variables-de-entorno"></a>
## Configuration (environment variables)

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8092` | HTTP port |
| `DATABASE_URL` | — (required) | PostgreSQL DSN |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Sytue ID issuer |
| `OIDC_JWKS_URL` | derived from the issuer | JWKS endpoint |
| `OIDC_AUDIENCE` | `sytue-hub` | Expected audience (client) |
| `DRIVE_BASE_URL` … `CHAT_BASE_URL` | Internal service URLs | Upstream APIs for search |
| `DRIVE_PUBLIC_URL` … `CHAT_PUBLIC_URL` | `https://*.sytue.localhost` | Deep-link bases |
| `SEARCH_TIMEOUT` | `2500ms` | Timeout per federated source |
| `SEARCH_LIMIT` | `20` | Maximum results per source |
| `UPSTREAM_TIMEOUT` | `5s` | HTTP timeout to upstreams |
| `PRESENCE_TTL` | `45s` | Presence staleness window |
| `NOTIFY_RETENTION` | `720h` | Retention of read notifications |

No secrets in the repo: everything via environment / secrets manager.

<a id="ejecución"></a>
## Running

```sh
# Migraciones (también se aplican al arrancar)
go run ./services/hub/cmd/hub migrate

# Servidor (HTTP + relay SSE + purga periódica)
go run ./services/hub/cmd/hub
```
