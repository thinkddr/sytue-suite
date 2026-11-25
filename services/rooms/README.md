# Sytue Chat (`services/chat`)

Real-time **Sytue-native** messaging for the **Sytue One** suite: per-tenant teams,
channels, messages, threads, reactions, mentions and attachments.
**Clean-room** reimplementation on open standards (**WebSocket — RFC
6455** + our own REST API); our own model and protocol. Proprietary code ©
Sytue; third-party dependencies are permissive only (MIT/BSD/Apache). **Mattermost and
other AGPL engines are OUT.**

- **Port:** `8085` (HTTP + WebSocket) · **Subdomain:** `rooms.sytue.eu`
- **Datastore:** PostgreSQL (teams, channels, members, messages, threads,
  reactions, read_state, outbox) — the single source of truth. **No Redis/Valkey.**
- **Real time:** WebSocket (`coder/websocket`, MIT) with **fan-out via
  LISTEN/NOTIFY** over the PostgreSQL outbox (`platform/pkg/events`).
- **Auth:** **Sytue ID** JWT validated via JWKS (`platform/pkg/authn`).
- **Attachments:** reference **Sytue Drive** files (REST client that propagates
  the user's JWT; Drive applies its own authorization/RLS).
- **Multi-tenant:** isolation through **RLS** in PostgreSQL (`app.tenant`).

<a id="capacidades"></a>
## Capabilities

- **Teams:** create, list (the user's own), manage members (admin/member).
  The creator becomes `admin`.
- **Channels:** `public` (visible to the team), `private` and `direct` (DM, members
  only). Create, rename, change topic, archive, join/leave. Auto-join
  when posting in a public channel of the team ("join on first post").
- **Messages:** post, edit (author), delete (author or admin), with
  **mentions** (`mentions[]` of Sytue ID subs) and **attachments** (`file_ids[]` from
  Drive, denormalized to name/type/size).
- **Threads:** replies with `parent_id`; `reply_count` maintained
  transactionally; thread read (root + replies).
- **Reactions:** add/remove per emoji; aggregated per message (no N+1).
- **Read state:** mark as read per channel; **unread** counters
  (badges) per team in a single query.
- **Search:** substring search in messages of the team's accessible channels.
- **Keyset pagination** (`before`/`created_at`) for the message feed.

<a id="tiempo-real-websocket-ws"></a>
## Real time (WebSocket `/ws`)

Connection: `wss://rooms.sytue.eu/ws` with the JWT in the `Authorization: Bearer` header
or in the `?access_token=` query (the browser cannot set headers on the
handshake). Accepted origins: `*.sytue.eu` / `*.sytue.localhost`.

JSON protocol (client → server):

```jsonc
{ "type": "subscribe",   "channel_id": "<uuid>" }   // suscribirse a un canal
{ "type": "unsubscribe", "channel_id": "<uuid>" }
{ "type": "typing",      "channel_id": "<uuid>" }    // "está escribiendo…"
{ "type": "presence",    "status": "online|away" }
{ "type": "ping" }
```

Server → client: `ready`, `message`, `message_edited`, `message_deleted`,
`reaction`, `typing`, `presence`, `error`, `pong`. Message and
reaction events originate in the REST API, are persisted + published to the **outbox**, and a
**relay** (`events.Dispatcher`, LISTEN/NOTIFY) broadcasts them to the rooms of **all
replicas** of the service. `typing`/`presence` are ephemeral (not persisted).

<a id="arquitectura"></a>
## Architecture

```
internal/
  config/   carga 12-factor (PORT, DATABASE_URL, OIDC_*, DRIVE_URL, WS_*)
  domain/   tipos y reglas (teams, channels, messages, reactions, validación)
  store/    persistencia pgx + RLS por tenant (store/channels/messages/reactions)
  ws/       Hub + Client WebSocket (protocolo, salas por tenant+canal)
  http/     router chi, handlers REST, handler WS, middleware de propagación JWT
  service/  casos de uso, publicación de eventos, relay de fan-out, cliente Drive
db/migrations/  esquema + RLS + outbox (golang-migrate, embebidas)
cmd/chat/  binario (subcomandos: server | migrate | healthcheck)
```

Cross-replica fan-out uses `events.Dispatcher` with *at-least-once* semantics
(LISTEN/NOTIFY + fallback polling). The `Hub` is an in-memory multiplexer per
process; replicas converge via the PostgreSQL outbox — **no Redis/Valkey**.

<a id="configuración-variables-de-entorno"></a>
## Configuration (environment variables)

| Variable | Default (dev) | Description |
|---|---|---|
| `PORT` | `8085` | HTTP/WebSocket port |
| `DATABASE_URL` | — (required) | PostgreSQL DSN |
| `DB_MAX_CONNS` | `10` | pgx pool size |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Sytue ID issuer |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | JWKS |
| `OIDC_AUDIENCE` | `sytue-chat` | Expected JWT audience |
| `DRIVE_URL` | `http://drive:8082` | Sytue Drive API base |
| `WS_ALLOWED_ORIGINS` | `*.sytue.eu,*.sytue.localhost` | Accepted WS origins |
| `MAX_MESSAGE_KB` | `64` | Maximum size of a message body |
| `MESSAGE_PAGE_SIZE` | `50` | Default page size |
| `PRESENCE_TTL` | `45s` | "Online" presence window |
| `PUBLIC_URL` | `https://chat.sytue.localhost` | Public URL |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | Logging (`slog`) |

Secrets **only** via the environment / Secret Manager — never in git (CONVENTIONS §10).

<a id="ejecución"></a>
## Running

```sh
# Migrar y arrancar (las migraciones también se aplican al arrancar el server).
go run ./services/chat/cmd/chat migrate
go run ./services/chat/cmd/chat

# Imagen (contexto = raíz del repo, módulo único git.sytue.eu/sytue/one).
docker build -f services/chat/Dockerfile -t sytue-chat .
```

<a id="seguridad-y-rgpd"></a>
## Security and GDPR

**Deny-by-default** authorization (per-tenant RLS + team/channel membership),
**parameterized** queries (no dynamic SQL), input validation, limits on
message size and subscriptions per connection (anti-abuse), restricted WS
origins. Logs without personal data. Cross-cutting details in
[`docs/security.md#1-seguridad-y-rgpd`](../../docs/security.md#1-seguridad-y-rgpd).

## API

Full specification in [`openapi.yaml`](./openapi.yaml).
