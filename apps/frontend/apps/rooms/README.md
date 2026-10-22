# Sytue Chat

Mensajería en tiempo real de la suite **Sytue One** en **Astro 7 + Preact 10**,
repuntada al servicio **Go propio Sytue Chat** (REST + WebSocket; PostgreSQL como
único datastore). Producto **Sytue-native**, original y **propietario** (© Sytue):
reimplementación _clean-room_ sobre estándares abiertos y librerías permisivas —
sin motores de terceros AGPL y sin Redis. Soberanía 100 % UE (Scaleway).

Canales (públicos / privados / DMs), vista de canal con histórico y redacción.
Lenguaje visual **Sytue**, modo claro/oscuro y WCAG AA del design system. Monta la
**Sytue Shell** (`@sytue/shell`) y usa exclusivamente `@sytue/ui`.

## Arquitectura

- **BFF + OIDC/PKCE** (`@sytue/auth`): `src/pages/auth/*` (client confidencial
  `sytue-chat` de **Sytue ID**, discovery OIDC estándar). Cookie de sesión httpOnly
  `.sytue.eu`. El secreto del client y el access token viven sólo en el servidor.
- **BFF REST** (`src/pages/api/*`): proxy same-origin (cookie) → servicio Go Sytue
  Chat con el access token (`src/lib/chat.ts`). Endpoints: `channels`, `messages`
  (histórico + paginación), `send` (publicar), `ws-ticket` (emite un ticket de
  corta duración para el WebSocket). El token nunca toca el navegador como dato.
- **Tiempo real** (`src/client/chatApi.ts`): WebSocket directo navegador ↔ servicio
  Go (`/ws`), autorizado con el ticket del BFF; reconexión con backoff.
- **Islas Preact** (`src/components/*`): `ChatApp` (raíz: sidebar de canales +
  panel de canal + realtime + envío optimista), `MessageView` (histórico agrupado),
  `Composer` (redacción, Enter envía / Shift+Enter salto de línea).

## Desarrollo

Puerto dev `4330` (ver `docs/CONVENTIONS.md` §6); dominio `rooms.sytue.localhost`
(Sytue Rooms = Chat+Meet unificados; `chat.*` y `meet.*` responden 301 permanente).
Backend Go Sytue Chat en `:8085` (REST + WS).

```bash
bun run --filter @sytue/chat dev
```

Copia `.env.example` a `.env` (sólo placeholders; secretos reales fuera de git).

## Producción

`Dockerfile` multi-stage con **Bun** (salida `standalone` de Astro). Desplegado por
el umbrella chart `sytue-one` en Kapsule (Scaleway, regiones UE).
