# Sytue One — Portal (`one.sytue.eu`)

Unified portal and launcher of the **Sytue One** suite. It is the face of the suite:
a personalized greeting, a grid of launchers to every product, cross-app quick
links and federated recent activity from the **Sytue Hub**. Built on top of
the **Sytue Shell** (global topbar, 9-dot app switcher, global search,
notifications, presence and account).

- **Stack:** Astro 7 + Preact 10 (islands), Tailwind v4 with tokens from `@sytue/ui`.
  Runtime / build / package manager with **Bun** (replaces Node.js + pnpm).
- **Design:** Sytue visual language, light/dark mode, WCAG AA.
- **Identity:** OIDC Authorization Code + PKCE against **Sytue ID**
  (confidential client `sytue-one`) through the `@sytue/auth` BFF. Host-only
  `__Host-` cookie, httpOnly; the IdP provides SSO across subdomains via OAuth.

<a id="paquetes-consumidos"></a>

## Packages consumed

- `@sytue/ui` — design system (AppGrid, Card, EmptyState, Spinner, Icon…).
- `@sytue/shell` — Sytue Shell mounted in the layout.
- `@sytue/auth` — OIDC/PKCE BFF (endpoints `src/pages/auth/*`).
- `@sytue/api-clients` — `HubClient` for recent activity.
- `@sytue/config` — tokens, Tailwind v4 theme, strict tsconfig.

<a id="desarrollo"></a>

## Development

Dev port **4321** (`*.sytue.localhost` via Traefik, see `docs/CONVENTIONS.md` §6).

```bash
bun run --filter @sytue/portal dev        # http://localhost:4321
bun run --filter @sytue/portal build
bun run --filter @sytue/portal typecheck
```

<a id="variables-de-entorno-bff-solo-servidor--nunca-secretos-en-git"></a>

## Environment variables (BFF, server only — never secrets in git)

| Variable                    | Example                                                  |
| --------------------------- | -------------------------------------------------------- |
| `SYTUE_OIDC_ISSUER`         | `https://id.sytue.eu` (dev `https://id.sytue.localhost`) |
| `SYTUE_OIDC_CLIENT_ID`      | `sytue-one`                                              |
| `SYTUE_OIDC_CLIENT_SECRET`  | _(Scaleway Secret Manager / Sealed Secrets)_             |
| `SYTUE_OIDC_REDIRECT_URI`   | `https://one.sytue.eu/auth/callback`                     |
| `SYTUE_SESSION_SECRET`      | _(>= 32 characters)_                                     |
| `PUBLIC_SYTUE_HUB_BASE_URL` | `https://hub.sytue.eu`                                   |
| `SYTUE_SECURE_COOKIES`      | `false` only in local dev over http                      |

<a id="endpoints-bff"></a>

## BFF endpoints

`GET /auth/login` · `GET /auth/callback` · `POST|GET /auth/logout` ·
`GET /auth/session` · `POST /auth/refresh` · `GET /auth/token`.
