# Sytue Calendar (`services/calendar`)

Servicio **Go** propio de calendario de **Sytue One**. Implementacion **clean-room**
sobre estandares abiertos: **CalDAV** (RFC 4791), **iCalendar** (RFC 5545),
**scheduling** iTIP/iMIP (RFC 5546 + RFC 6638) y **sync-collection** (RFC 6578),
con librerias permisivas (`emersion/go-webdav/caldav` BSD, `emersion/go-ical` MIT,
`pgx`, `chi`). Estado en **PostgreSQL** (unica fuente de verdad; sin Redis/Valkey).
Codigo **MIT, (c) 2026 Daniel Delgado Ramos**. Subdominio: `calendar.sytue.eu` · Puerto dev: **8087**.

> Forma parte del monorepo `git.sytue.eu/sytue/one`. No se despliega ningun motor
> de terceros AGPL. La autenticacion la provee **Sytue ID** (OIDC/JWT validado por JWKS).

## Que hace

- **Servidor CalDAV (RFC 4791)** bajo `/dav`: descubrimiento de principal y
  `calendar-home-set`, colecciones-calendario, `GET`/`PUT`/`DELETE` de recursos
  iCalendar, `calendar-query` (filtro por rango), `ETag` para operaciones
  condicionales y `sync-token` para sincronizacion incremental.
- **API REST propia** bajo `/api/v1`: calendarios, eventos (puntuales y
  recurrentes), expansion de instancias en un rango, asistentes/RSVP,
  free/busy e invitaciones (scheduling inbox).
- **Recurrencias RFC 5545**: parser y expansor propio de `RRULE`
  (`FREQ`/`INTERVAL`/`COUNT`/`UNTIL`/`BYMONTH`/`BYMONTHDAY`/`BYDAY`/`WKST`), con
  `RDATE`/`EXDATE` y overrides `RECURRENCE-ID`. Cota dura de instancias.
- **Free/Busy** (RFC 4791 free-busy-query / RFC 5545 `FREEBUSY`): expande las
  series, descarta `TRANSPARENT`/`CANCELLED` y fusiona los periodos ocupados.
- **Invitaciones iTIP** (RFC 5546 / RFC 6638): entrega `REQUEST`/`CANCEL` a la
  scheduling inbox de los asistentes del tenant y `REPLY` al organizador; el
  envio externo (iMIP/SMTP) lo realiza **Sytue Mail** consumiendo el outbox.
- **Multi-tenant** por **RLS** en PostgreSQL (`app.tenant`), aislado por tenant.

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

Capas: `http`/`caldav` (transporte) -> `service` (casos de uso) -> `store`
(PostgreSQL). El modelo y las reglas (recurrencia, free/busy) viven en `domain`,
sin dependencias de transporte ni de almacenamiento.

## Configuracion (variables de entorno)

| Variable | Por defecto | Descripcion |
|---|---|---|
| `PORT` / `HTTP_ADDR` | `8087` / `:8087` | Puerto/direccion de escucha. |
| `PUBLIC_URL` | `https://calendar.sytue.localhost` | URL publica. |
| `DATABASE_URL` | — (**obligatoria**) | DSN PostgreSQL (`postgres://...`). |
| `DB_MAX_CONNS` | `10` | Tamano del pool pgx. |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Issuer de Sytue ID. |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | Endpoint JWKS. |
| `OIDC_AUDIENCE` | `sytue-calendar` | Audiencia esperada del JWT. |
| `CALDAV_PREFIX` | `/dav` | Prefijo de montaje del servidor CalDAV. |
| `MAX_FREEBUSY_WINDOW` | `8784h` (366d) | Ventana maxima de free/busy. |
| `MAX_RANGE_WINDOW` | `8784h` (366d) | Ventana maxima de calendar-query. |
| `MAX_OBJECT_KB` | `1024` | Tamano maximo de un recurso iCalendar (KiB). |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | Logging estructurado (slog). |

Sin secretos en el codigo: todo por entorno / Secret Manager.

## Ejecucion

Local (dev): el servicio arranca con el resto de la suite via
`deploy/compose/docker-compose.yml` (`make dev-up`). Aplica sus migraciones en
el arranque y queda escuchando en `https://calendar.sytue.localhost` (Traefik).

```bash
# Build del binario (contexto = raiz del repo)
go build ./services/calendar/cmd/calendar

# Tests del dominio (recurrencia, free/busy)
go test ./services/calendar/...
```

El `HEALTHCHECK` de Docker invoca `/app/calendar healthcheck`, que consulta
`/healthz` del propio proceso.

## API (resumen)

REST (`/api/v1`, JWT Bearer de Sytue ID):

- `GET/POST /calendars`, `GET/PATCH/DELETE /calendars/{id}`
- `POST /calendars/{id}/events` · `GET /calendars/{id}/instances?start&end`
- `GET/PUT/DELETE /events/{id}` · `PUT /events/{id}/attendees/partstat`
- `GET /freebusy?start&end[&user]`
- `GET /invitations[?pending=true]` · `POST /invitations/{id}/respond`

CalDAV (`/dav`, mismo JWT): `PROPFIND`/`REPORT`/`GET`/`PUT`/`DELETE` segun
RFC 4791 sobre `calendars/<sub>/<calendario>/<recurso>.ics`.

Contrato completo en [`openapi.yaml`](./openapi.yaml).

## Notas de cumplimiento

- **Clean-room**: implementado desde las RFC y la API publica de las librerias
  permisivas; no se copia ni porta codigo AGPL.
- **Seguridad por defecto**: deny-by-default + RLS por tenant, consultas
  parametrizadas (sin SQL dinamico), `ETag`/`If-Match` para concurrencia,
  contenedor distroless rootless, secretos solo por entorno.
- **Sin Redis/Valkey**: colas/outbox de eventos sobre PostgreSQL
  (`platform/pkg/events`). Calendar es PG-only (no usa Object Storage S3).
