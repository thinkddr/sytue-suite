# Sytue Calendar (`services/calendar`)

Sytue's own **Go** calendar service for **Sytue One**. A **clean-room** implementation
on top of open standards: **CalDAV** (RFC 4791), **iCalendar** (RFC 5545),
**scheduling** iTIP/iMIP (RFC 5546 + RFC 6638) and **sync-collection** (RFC 6578),
with permissive libraries (`emersion/go-webdav/caldav` BSD, `emersion/go-ical` MIT,
`pgx`, `chi`). State in **PostgreSQL** (single source of truth; no Redis/Valkey).
Code **MIT, (c) 2026 Daniel Delgado Ramos**. Subdomain: `calendar.sytue.eu` · Dev port: **8087**.

> Part of the `git.sytue.eu/sytue/one` monorepo. No third-party AGPL engine is
> deployed. Authentication is provided by **Sytue ID** (OIDC/JWT validated via JWKS).

<a id="que-hace"></a>
## What it does

- **CalDAV server (RFC 4791)** under `/dav`: principal and `calendar-home-set`
  discovery, calendar collections, `GET`/`PUT`/`DELETE` of iCalendar
  resources, `calendar-query` (range filter), `ETag` for conditional
  operations and `sync-token` for incremental synchronization.
- **Own REST API** under `/api/v1`: calendars, events (single and
  recurring), expansion of instances within a range, attendees/RSVP,
  free/busy and invitations (scheduling inbox).
- **RFC 5545 recurrences**: in-house parser and expander for `RRULE`
  (`FREQ`/`INTERVAL`/`COUNT`/`UNTIL`/`BYMONTH`/`BYMONTHDAY`/`BYDAY`/`WKST`), with
  `RDATE`/`EXDATE` and `RECURRENCE-ID` overrides. Hard cap on instances.
- **Free/Busy** (RFC 4791 free-busy-query / RFC 5545 `FREEBUSY`): expands the
  series, discards `TRANSPARENT`/`CANCELLED` and merges the busy periods.
- **iTIP invitations** (RFC 5546 / RFC 6638): delivers `REQUEST`/`CANCEL` to the
  scheduling inbox of the tenant's attendees and `REPLY` to the organizer; external
  sending (iMIP/SMTP) is done by **Sytue Mail** consuming the outbox.
- **Multi-tenant** through **RLS** in PostgreSQL (`app.tenant`), isolated per tenant.

## Layout

```
services/calendar/
├── cmd/calendar/main.go        # binario: wiring, migraciones, healthcheck
├── internal/
│   ├── config/                 # configuracion 12-factor (env)
│   ├── domain/                 # tipos, recurrencia RFC5545, free/busy (+ tests)
│   ├── store/                  # persistencia pgx + RLS (calendars/objects/events/...)
│   ├── caldav/                 # backend CalDAV (go-webdav) + puente iCalendar (go-ical)
│   └── http/                   # router chi + handlers REST
├── db/
│   ├── migrations.go           # //go:embed de las migraciones
│   └── migrations/             # 0001_init.up.sql / .down.sql (RLS, indices)
├── openapi.yaml                # contrato OpenAPI 3.1 de la API REST
├── Dockerfile                  # multi-stage golang -> distroless/static (nonroot)
└── README.md
```

Layers: `http`/`caldav` (transport) -> `service` (use cases) -> `store`
(PostgreSQL). The model and the rules (recurrence, free/busy) live in `domain`,
with no transport or storage dependencies.

<a id="configuracion-variables-de-entorno"></a>
## Configuration (environment variables)

| Variable | Default | Description |
|---|---|---|
| `PORT` / `HTTP_ADDR` | `8087` / `:8087` | Listen port/address. |
| `PUBLIC_URL` | `https://calendar.sytue.localhost` | Public URL. |
| `DATABASE_URL` | — (**required**) | PostgreSQL DSN (`postgres://...`). |
| `DB_MAX_CONNS` | `10` | pgx pool size. |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Sytue ID issuer. |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | JWKS endpoint. |
| `OIDC_AUDIENCE` | `sytue-calendar` | Expected JWT audience. |
| `CALDAV_PREFIX` | `/dav` | Mount prefix of the CalDAV server. |
| `MAX_FREEBUSY_WINDOW` | `8784h` (366d) | Maximum free/busy window. |
| `MAX_RANGE_WINDOW` | `8784h` (366d) | Maximum calendar-query window. |
| `MAX_OBJECT_KB` | `1024` | Maximum size of an iCalendar resource (KiB). |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | Structured logging (slog). |

No secrets in the code: everything via environment / Secret Manager.

<a id="ejecucion"></a>
## Running

Local (dev): the service starts with the rest of the suite via
`deploy/compose/docker-compose.yml` (`make dev-up`). It applies its migrations on
startup and listens on `https://calendar.sytue.localhost` (Traefik).

```bash
# Build del binario (contexto = raiz del repo)
go build ./services/calendar/cmd/calendar

# Tests del dominio (recurrencia, free/busy)
go test ./services/calendar/...
```

The Docker `HEALTHCHECK` invokes `/app/calendar healthcheck`, which queries the
process's own `/healthz`.

<a id="api-resumen"></a>
## API (summary)

REST (`/api/v1`, Sytue ID JWT Bearer):

- `GET/POST /calendars`, `GET/PATCH/DELETE /calendars/{id}`
- `POST /calendars/{id}/events` · `GET /calendars/{id}/instances?start&end`
- `GET/PUT/DELETE /events/{id}` · `PUT /events/{id}/attendees/partstat`
- `GET /freebusy?start&end[&user]`
- `GET /invitations[?pending=true]` · `POST /invitations/{id}/respond`

CalDAV (`/dav`, same JWT): `PROPFIND`/`REPORT`/`GET`/`PUT`/`DELETE` per
RFC 4791 on `calendars/<sub>/<calendario>/<recurso>.ics`.

Full contract in [`openapi.yaml`](./openapi.yaml).

<a id="notas-de-cumplimiento"></a>
## Compliance notes

- **Clean-room**: implemented from the RFCs and the public API of the permissive
  libraries; no AGPL code is copied or ported.
- **Secure by default**: deny-by-default + per-tenant RLS, parameterized
  queries (no dynamic SQL), `ETag`/`If-Match` for concurrency,
  rootless distroless container, secrets only via environment.
- **No Redis/Valkey**: event queues/outbox on PostgreSQL
  (`platform/pkg/events`). Calendar is PG-only (it does not use S3 Object Storage).
