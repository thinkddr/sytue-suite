# Sytue Chat (`services/chat`)

Mensajería en tiempo real **Sytue-native** de la suite **Sytue One**: equipos
(teams) por tenant, canales, mensajes, hilos, reacciones, menciones y adjuntos.
Reimplementación **clean-room** sobre estándares abiertos (**WebSocket — RFC
6455** + API REST propia); modelo y protocolo propios. Código propietario ©
Sytue; dependencias de terceros solo permisivas (MIT/BSD/Apache). **Mattermost y
demás motores AGPL quedan FUERA.**

- **Puerto:** `8085` (HTTP + WebSocket) · **Subdominio:** `rooms.sytue.eu`
- **Datastore:** PostgreSQL (teams, channels, members, messages, threads,
  reactions, read_state, outbox) — única fuente de verdad. **Sin Redis/Valkey.**
- **Tiempo real:** WebSocket (`coder/websocket`, MIT) con **fan-out por
  LISTEN/NOTIFY** sobre el outbox de PostgreSQL (`platform/pkg/events`).
- **Auth:** JWT de **Sytue ID** validado vía JWKS (`platform/pkg/authn`).
- **Adjuntos:** referencian ficheros de **Sytue Drive** (cliente REST que propaga
  el JWT del usuario; Drive aplica su propia autorización/RLS).
- **Multi-tenant:** aislamiento por **RLS** en PostgreSQL (`app.tenant`).

## Capacidades

- **Teams:** crear, listar (los del usuario), gestionar miembros (admin/member).
  El creador queda como `admin`.
- **Canales:** `public` (visibles para el team), `private` y `direct` (DM, solo
  miembros). Crear, renombrar, retematizar, archivar, unirse/abandonar. Auto-unión
  al publicar en un canal público del team ("join on first post").
- **Mensajes:** publicar, editar (autor), borrar (autor o admin), con
  **menciones** (`mentions[]` de subs de Sytue ID) y **adjuntos** (`file_ids[]` de
  Drive, denormalizados a nombre/tipo/tamaño).
- **Hilos:** respuestas con `parent_id`; `reply_count` mantenido
  transaccionalmente; lectura de hilo (raíz + respuestas).
- **Reacciones:** añadir/retirar por emoji; se agregan por mensaje (sin N+1).
- **Estado de lectura:** marcar leído por canal; contadores de **no leídos**
  (badges) por team en una sola consulta.
- **Búsqueda:** por subcadena en mensajes de canales accesibles del team.
- **Paginación keyset** (`before`/`created_at`) para el feed de mensajes.

## Tiempo real (WebSocket `/ws`)

Conexión: `wss://rooms.sytue.eu/ws` con el JWT por cabecera `Authorization: Bearer`
o por query `?access_token=` (el navegador no puede fijar cabeceras en el
handshake). Orígenes aceptados: `*.sytue.eu` / `*.sytue.localhost`.

Protocolo JSON (cliente → servidor):

```jsonc
{ "type": "subscribe",   "channel_id": "<uuid>" }   // suscribirse a un canal
{ "type": "unsubscribe", "channel_id": "<uuid>" }
{ "type": "typing",      "channel_id": "<uuid>" }    // "está escribiendo…"
{ "type": "presence",    "status": "online|away" }
{ "type": "ping" }
```

Servidor → cliente: `ready`, `message`, `message_edited`, `message_deleted`,
`reaction`, `typing`, `presence`, `error`, `pong`. Los eventos de mensajes y
reacciones se originan en la API REST, se persisten + publican al **outbox** y un
**relay** (`events.Dispatcher`, LISTEN/NOTIFY) los difunde a las salas de **todas
las réplicas** del servicio. `typing`/`presence` son efímeros (no persisten).

## Arquitectura

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

El fan-out cross-réplica usa `events.Dispatcher` con semántica *at-least-once*
(LISTEN/NOTIFY + poll de respaldo). El `Hub` es un multiplexor en memoria por
proceso; las réplicas convergen vía el outbox de PostgreSQL — **sin Redis/Valkey**.

## Configuración (variables de entorno)

| Variable | Default (dev) | Descripción |
|---|---|---|
| `PORT` | `8085` | Puerto HTTP/WebSocket |
| `DATABASE_URL` | — (obligatoria) | DSN PostgreSQL |
| `DB_MAX_CONNS` | `10` | Tamaño del pool pgx |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Issuer de Sytue ID |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | JWKS |
| `OIDC_AUDIENCE` | `sytue-chat` | Audiencia esperada del JWT |
| `DRIVE_URL` | `http://drive:8082` | Base de la API de Sytue Drive |
| `WS_ALLOWED_ORIGINS` | `*.sytue.eu,*.sytue.localhost` | Orígenes WS aceptados |
| `MAX_MESSAGE_KB` | `64` | Tamaño máximo del cuerpo de un mensaje |
| `MESSAGE_PAGE_SIZE` | `50` | Tamaño de página por defecto |
| `PRESENCE_TTL` | `45s` | Ventana de presencia "en línea" |
| `PUBLIC_URL` | `https://chat.sytue.localhost` | URL pública |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | Logging (`slog`) |

Secretos **solo** por entorno / Secret Manager — nunca en git (CONVENTIONS §10).

## Ejecución

```sh
# Migrar y arrancar (las migraciones también se aplican al arrancar el server).
go run ./services/chat/cmd/chat migrate
go run ./services/chat/cmd/chat

# Imagen (contexto = raíz del repo, módulo único git.sytue.eu/sytue/one).
docker build -f services/chat/Dockerfile -t sytue-chat .
```

## Seguridad y RGPD

Autorización **deny-by-default** (RLS por tenant + pertenencia a team/canal),
consultas **parametrizadas** (sin SQL dinámico), validación de entrada, límites
de tamaño de mensaje y de suscripciones por conexión (anti-abuso), orígenes WS
acotados. Logs sin datos personales. Detalle transversal en
[`docs/security.md#1-seguridad-y-rgpd`](../../docs/security.md#1-seguridad-y-rgpd).

## API

Especificación completa en [`openapi.yaml`](./openapi.yaml).
