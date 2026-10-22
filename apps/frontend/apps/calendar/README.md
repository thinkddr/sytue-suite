# Sytue Calendar

Calendario propio de la suite **Sytue One** en **Astro 7 + Preact 10** sobre el
servicio Go propio **Sytue Calendar** (estándar **CalDAV** RFC 4791 + API REST;
estado en PostgreSQL; soberanía 100 % UE, Scaleway). Vistas **mes / semana / día**,
crear y editar eventos. Lenguaje visual **Sytue**, modo claro/oscuro y WCAG AA del
design system.

Monta la **Sytue Shell** (`@sytue/shell`) y usa exclusivamente `@sytue/ui`.

## Arquitectura

- **BFF + OIDC/PKCE** (`@sytue/auth`): `src/pages/auth/*` (client `sytue-calendar`
  contra **Sytue ID**). Cookie de sesión httpOnly `.sytue.eu`.
- **BFF CalDAV** (`src/pages/api/*`): proxy same-origin (cookie) → servicio Go
  **Sytue Calendar** con el access token Bearer de Sytue ID, vía
  `@sytue/api-clients/caldav`. Endpoints: `calendars`, `events`, `event`
  (POST crear/editar, DELETE eliminar). El cuerpo iCalendar (VEVENT) lo genera el
  servidor (`src/lib/ical.ts`), no el cliente.
- **Islas Preact** (`src/components/*`): `CalendarApp` (raíz), `CalendarGrid`
  (mes/semana/día), `EventModal` (crear/editar/eliminar), utilidades de fecha
  (`dates.ts`, es_ES, semana en lunes).

## Desarrollo

Puerto dev `4327` (ver `docs/CONVENTIONS.md` §6); dominio `calendar.sytue.localhost`.
Runtime/instalación/bundler con **Bun** (sustituye a Node.js + pnpm).

```bash
bun install
bun --filter @sytue/calendar dev
```

Copia `.env.example` a `.env` (sólo placeholders; secretos reales fuera de git).

## Producción

`Dockerfile` multi-stage (Bun, salida `standalone` de Astro). Desplegado por el
umbrella chart `sytue-one` en Kapsule (Scaleway, regiones UE).
