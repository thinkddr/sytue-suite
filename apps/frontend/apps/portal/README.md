# Sytue One — Portal (`one.sytue.eu`)

Portal y lanzador unificado de la suite **Sytue One**. Es la cara de la suite:
saludo personalizado, rejilla de lanzadores hacia todos los productos, accesos
rapidos cross-app y actividad reciente federada del **Sytue Hub**. Montado sobre
la **Sytue Shell** (topbar global, app switcher de 9 puntos, busqueda global,
notificaciones, presencia y cuenta).

- **Stack:** Astro 7 + Preact 10 (islands), Tailwind v4 con tokens de `@sytue/ui`.
  Runtime / build / gestor de paquetes con **Bun** (sustituye a Node.js + pnpm).
- **Diseno:** lenguaje visual Sytue, modo claro/oscuro, WCAG AA.
- **Identidad:** OIDC Authorization Code + PKCE contra **Sytue ID**
  (client confidencial `sytue-one`) via el BFF de `@sytue/auth`. Cookie host-only
  `__Host-`, httpOnly; el IdP aporta el SSO entre subdominios mediante OAuth.

## Paquetes consumidos

- `@sytue/ui` — design system (AppGrid, Card, EmptyState, Spinner, Icon…).
- `@sytue/shell` — Sytue Shell montada en el layout.
- `@sytue/auth` — BFF OIDC/PKCE (endpoints `src/pages/auth/*`).
- `@sytue/api-clients` — `HubClient` para la actividad reciente.
- `@sytue/config` — tokens, tema Tailwind v4, tsconfig estricto.

## Desarrollo

Puerto dev **4321** (`*.sytue.localhost` via Traefik, ver `docs/CONVENTIONS.md` §6).

```bash
bun run --filter @sytue/portal dev        # http://localhost:4321
bun run --filter @sytue/portal build
bun run --filter @sytue/portal typecheck
```

## Variables de entorno (BFF, solo servidor — nunca secretos en git)

| Variable                    | Ejemplo                                                  |
| --------------------------- | -------------------------------------------------------- |
| `SYTUE_OIDC_ISSUER`         | `https://id.sytue.eu` (dev `https://id.sytue.localhost`) |
| `SYTUE_OIDC_CLIENT_ID`      | `sytue-one`                                              |
| `SYTUE_OIDC_CLIENT_SECRET`  | _(Scaleway Secret Manager / Sealed Secrets)_             |
| `SYTUE_OIDC_REDIRECT_URI`   | `https://one.sytue.eu/auth/callback`                     |
| `SYTUE_SESSION_SECRET`      | _(>= 32 caracteres)_                                     |
| `PUBLIC_SYTUE_HUB_BASE_URL` | `https://hub.sytue.eu`                                   |
| `SYTUE_SECURE_COOKIES`      | `false` solo en dev local sobre http                     |

## Endpoints BFF

`GET /auth/login` · `GET /auth/callback` · `POST|GET /auth/logout` ·
`GET /auth/session` · `POST /auth/refresh` · `GET /auth/token`.
