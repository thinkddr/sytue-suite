# Sytue Chat

Real-time messaging for the **Sytue One** suite in **Astro 7 + Preact 10**,
re-pointed to the **in-house Go service Sytue Chat** (REST + WebSocket; PostgreSQL as
the only datastore). A **Sytue-native**, original and **proprietary** product (© Sytue):
a _clean-room_ reimplementation on open standards and permissive libraries —
no third-party AGPL engines and no Redis. 100% EU sovereignty (Scaleway).

Channels (public / private / DMs), channel view with history and composing.
**Sytue** visual language, light/dark mode and WCAG AA from the design system. It mounts the
**Sytue Shell** (`@sytue/shell`) and uses `@sytue/ui` exclusively.

<a id="arquitectura"></a>

## Architecture

- **BFF + OIDC/PKCE** (`@sytue/auth`): `src/pages/auth/*` (confidential client
  `sytue-chat` of **Sytue ID**, standard OIDC discovery). httpOnly session cookie on
  `.sytue.eu`. The client secret and the access token live only on the server.
- **REST BFF** (`src/pages/api/*`): same-origin (cookie) proxy → Sytue Chat Go
  service with the access token (`src/lib/chat.ts`). Endpoints: `channels`, `messages`
  (history + pagination), `send` (publish), `ws-ticket` (issues a short-lived
  ticket for the WebSocket). The token never reaches the browser as data.
- **Real time** (`src/client/chatApi.ts`): direct browser ↔ Go service WebSocket
  (`/ws`), authorized with the BFF ticket; reconnection with backoff.
- **Preact islands** (`src/components/*`): `ChatApp` (root: channel sidebar +
  channel panel + realtime + optimistic sending), `MessageView` (grouped history),
  `Composer` (composing, Enter sends / Shift+Enter inserts a line break).

<a id="desarrollo"></a>

## Development

Dev port `4330` (see `docs/CONVENTIONS.md` §6); domain `rooms.sytue.localhost`
(Sytue Rooms = Chat+Meet unified; `chat.*` and `meet.*` respond with a permanent 301).
Sytue Chat Go backend on `:8085` (REST + WS).

```bash
bun run --filter @sytue/chat dev
```

Copy `.env.example` to `.env` (placeholders only; real secrets stay out of git).

<a id="producción"></a>

## Production

Multi-stage `Dockerfile` with **Bun** (Astro `standalone` output). Deployed by
the `sytue-one` umbrella chart on Kapsule (Scaleway, EU regions).
