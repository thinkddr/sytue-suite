# Sytue Hub (`services/hub`)

Backend de **integración seamless** de la suite **Sytue One** (el "shell backend"
que respalda a `@sytue/shell`). Reúne cuatro capacidades transversales sobre los
servicios de producto, **sin** datos propios de negocio: **búsqueda federada**,
**notificaciones unificadas**, **presencia** e **intents cross-app**.

Código Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr); dependencias de terceros solo permisivas
(MIT/BSD/Apache). Clean-room sobre estándares abiertos (REST, **SSE** —
WHATWG EventSource, **LISTEN/NOTIFY** de PostgreSQL). Sin motores AGPL; sin
Redis/Valkey.

- **Puerto:** `8092` (HTTP) · **Subdominio:** `hub.sytue.eu`
- **Datastore:** PostgreSQL (notificaciones, presencia, fan-out por LISTEN/NOTIFY)
  — única fuente de verdad. **Sin Redis/Valkey.**
- **Auth:** JWT de **Sytue ID** validado vía JWKS (`platform/pkg/authn`), client
  OIDC `sytue-hub`.
- **Multi-tenant:** aislamiento por **RLS** en PostgreSQL (`app.tenant`).

## Capacidades

### Búsqueda federada
Consulta en paralelo a **drive / mail / calendar / contacts / chat** mediante
conectores tipados (`internal/connectors/*`), **en nombre del usuario** (propaga
su Bearer JWT, de modo que cada servicio aplica su propia autorización). Cada
conector traduce el modelo de su servicio al modelo **canónico** `SearchHit`
(source, kind, id, title, snippet, url, icon, timestamp). Degrada con elegancia:
un origen lento o caído se reporta como **error parcial** por origen sin romper la
agregación; cada fan-out tiene su propio timeout.

### Notificaciones unificadas
Avisos por usuario, **durables** en PostgreSQL, con reparto en **tiempo real** por
**SSE** sobre **LISTEN/NOTIFY** (canal `sytue_hub_notifications`). La creación
inserta y emite `pg_notify` en la **misma transacción** (el aviso solo viaja si la
escritura hace commit). Un `Relay` por proceso escucha el canal y abanica a las
suscripciones SSE locales filtrando por `(tenant, subject)`. Endpoints de listado
(keyset), contador de no leídas, marcar leída / todas, y `GET .../stream` (SSE).

### Presencia
Heartbeat (upsert) por usuario; el estado **offline** se **deriva** al leer
comparando `last_seen` con la **TTL** de presencia (no requiere job para "caer").
Estado efímero en PostgreSQL (sin Redis/Valkey). Consulta agregada por lista de
subjects para que el shell pinte indicadores de presencia.

### Intents cross-app
Acciones que conectan dos productos y devuelven **deep-links** (el shell los
abre; Hub **no** ejecuta la acción — el servicio destino aplica autorización y
ejecución):
- `attach-drive-to-mail` — borrador de Mail con ficheros de Drive adjuntos.
- `create-event-from-mail` — compositor de Calendar prerellenado desde un correo.
- `share-to-chat` — compartir un recurso (Drive/Mail/Calendar/Contacts) en Chat.

## Estructura

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

## API (resumen)

| Método | Ruta | Descripción |
|---|---|---|
| `GET`  | `/api/v1/search?q=&sources=&limit=` | Búsqueda federada |
| `GET`  | `/api/v1/notifications` | Listar (keyset: `unread`, `before`, `limit`) |
| `GET`  | `/api/v1/notifications/unread_count` | Contador de no leídas |
| `POST` | `/api/v1/notifications` | Emitir (scope `notifications:write` para terceros) |
| `GET`  | `/api/v1/notifications/stream` | Stream SSE en tiempo real |
| `POST` | `/api/v1/notifications/{id}/read` | Marcar leída |
| `POST` | `/api/v1/notifications/read_all` | Marcar todas leídas |
| `GET`  | `/api/v1/presence?subjects=` | Consultar presencia |
| `PUT`  | `/api/v1/presence` | Heartbeat |
| `DELETE` | `/api/v1/presence` | Marcar offline |
| `POST` | `/api/v1/intents` | Resolver intent -> deep-links |
| `GET`  | `/healthz` · `/readyz` | Sondas |

## Configuración (variables de entorno)

| Variable | Defecto | Descripción |
|---|---|---|
| `PORT` | `8092` | Puerto HTTP |
| `DATABASE_URL` | — (obligatoria) | DSN PostgreSQL |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Issuer de Sytue ID |
| `OIDC_JWKS_URL` | derivada del issuer | Endpoint JWKS |
| `OIDC_AUDIENCE` | `sytue-hub` | Audiencia esperada (client) |
| `DRIVE_BASE_URL` … `CHAT_BASE_URL` | URLs internas de servicios | APIs upstream para la búsqueda |
| `DRIVE_PUBLIC_URL` … `CHAT_PUBLIC_URL` | `https://*.sytue.localhost` | Bases de deep-links |
| `SEARCH_TIMEOUT` | `2500ms` | Timeout por origen federado |
| `SEARCH_LIMIT` | `20` | Máximo de resultados por origen |
| `UPSTREAM_TIMEOUT` | `5s` | Timeout HTTP a upstreams |
| `PRESENCE_TTL` | `45s` | Ventana de obsolescencia de presencia |
| `NOTIFY_RETENTION` | `720h` | Retención de notificaciones leídas |

Sin secretos en el repo: todo por entorno / gestor de secretos.

## Ejecución

```sh
# Migraciones (también se aplican al arrancar)
go run ./services/hub/cmd/hub migrate

# Servidor (HTTP + relay SSE + purga periódica)
go run ./services/hub/cmd/hub
```
