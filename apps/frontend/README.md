# Sytue One — Frontend (monorepo)

Core of the **Sytue One** frontend: the design system, the common shell, the
OIDC authentication and the SDKs for the **in-house Go services** (Sytue-native).
**Astro 7 + Preact 10.29**, a monorepo with **Bun** (runtime/install/bundler;
replaces Node + pnpm), **Tailwind CSS 4**. **Sytue-native** visual language.
**100% EU** sovereignty. Code **Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr)**.

> Source of truth for the contract: [`docs/CONVENTIONS.md`](../../docs/CONVENTIONS.md).
> Design: [`docs/design.md#2-sistema-de-diseno`](../../docs/design.md#2-sistema-de-diseno) and `branding/`.

<a id="paquetes-compartidos-packages"></a>

## Shared packages (`packages/`)

| Package                  | Role                                                                                                                                                                                                                                                                                                             |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **`@sytue/config`**      | Strict tsconfig, flat ESLint, Prettier and the **Tailwind v4 theme layer** that imports the brand tokens from `branding/tokens/`. Tokens typed in TS.                                                                                                                                                            |
| **`@sytue/ui`**          | **Design system** (Preact): `Button`, `Card`, `Sidebar`, `TopBar`, `AppGrid`/Launcher (9 dots), `Modal`, `Avatar`, `List`, `Table`, `Toast`, `Tabs`, _form fields_, `ThemeProvider` (light/dark), `Badge`, `Dropdown`, `Tooltip`, `EmptyState`, `Skeleton`, `AppShell`. Tokens and logos/icons from `branding/`. |
| **`@sytue/shell`**       | The **Sytue Shell**: global top bar, **app switcher**, **global search** (Sytue Hub), **notification center** (SSE), **presence** and **account menu** (SSO logout). It is mounted in EVERY app.                                                                                                                 |
| **`@sytue/auth`**        | **OIDC Authorization Code + PKCE** client against **Sytue ID** (Go service; standard OIDC discovery `/.well-known/openid-configuration`, no `/realms/`), **BFF** (host-only `__Host-` cookie, httpOnly), session guard and refresh.                                                                              |
| **`@sytue/api-clients`** | Typed SDKs for the **in-house Go services**: `id`, `drive` (REST+WebDAV), `docs` (REST + collab WS), `mail` (JMAP), `calendar` (CalDAV), `contacts` (CardDAV), `chat` (REST+WS), `meet` (REST + LiveKit), `hub`, `cloud`.                                                                                        |

## Apps (`apps/`)

`portal`, `console`, `cloud-dashboard`, `mail`, `drive`, `calendar`,
`contacts`, `docs`, `chat`, `meet`, `www`. All of them mount `@sytue/shell`, use
`@sytue/ui` (no ad-hoc UI) and consume the Go services via `@sytue/api-clients`.

<a id="reglas-del-design-system"></a>

## Design system rules

1. Components **only** from `@sytue/ui`; shell **only** from `@sytue/shell`.
2. No ad-hoc UI: if a variant is missing, it is added to `@sytue/ui`.
3. Only **tokens** (`branding/tokens/`), never loose HEX or px values.
4. The **shell** is mounted on every web surface.
5. Brand (logos/icons) from `branding/`.
6. Light and dark always; **WCAG AA** accessibility.

## Scripts

```bash
bun install           # instala el workspace (Bun workspaces)
bun run build         # compila los paquetes compartidos
bun run typecheck     # TypeScript estricto en todo el monorepo
bun run lint          # ESLint (TS + a11y + Astro)
bun run format        # Prettier
```

<a id="uso-de-la-shell-en-una-app-astro"></a>

## Using the shell in an Astro app

```astro
---
// src/pages/index.astro (extracto)
import {
  createBff,
  resolveOidcConfig,
  resolveSessionSecret,
  requireSession,
} from '@sytue/auth/server';

const bff = createBff({
  config: resolveOidcConfig(import.meta.env),
  sessionSecret: resolveSessionSecret(import.meta.env),
});
const guard = await requireSession(bff, {
  request: Astro.request,
  cookies: Astro.cookies,
  url: Astro.url,
});
if (guard instanceof Response) return guard;
const { session } = guard;
---

<html lang="es">
  <head>
    <script set:html={themeInitScript}></script>
  </head>
  <body>
    <SytueShell
      client:load
      currentApp="mail"
      user={session.user}
      accessToken={session.tokens.accessToken}
    />
  </body>
</html>
```

> The OIDC variables (`SYTUE_OIDC_*`, `SYTUE_SESSION_SECRET`)
> are injected per environment from Scaleway Secret Manager / Sealed Secrets — they are **never**
> written to the repository.
