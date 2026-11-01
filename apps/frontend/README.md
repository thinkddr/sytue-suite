# Sytue One — Frontend (monorepo)

Núcleo del frontend de **Sytue One**: el design system, la shell común, la
autenticación OIDC y los SDKs a los **servicios Go propios** (Sytue-native).
**Astro 7 + Preact 10.29**, monorepo con **Bun** (runtime/instalación/bundler;
sustituye a Node + pnpm), **Tailwind CSS 4**. Lenguaje visual **Sytue-native**.
Soberanía **100 % UE**. Código **Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr)**.

> Fuente de verdad del contrato: [`docs/CONVENTIONS.md`](../../docs/CONVENTIONS.md).
> Diseño: [`docs/design.md#2-sistema-de-diseno`](../../docs/design.md#2-sistema-de-diseno) y `branding/`.

## Paquetes compartidos (`packages/`)

| Paquete                  | Rol                                                                                                                                                                                                                                                                                                                  |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **`@sytue/config`**      | tsconfig estricto, ESLint plano, Prettier y la **capa de tema Tailwind v4** que importa los tokens de marca de `branding/tokens/`. Tokens tipados en TS.                                                                                                                                                             |
| **`@sytue/ui`**          | **Design system** (Preact): `Button`, `Card`, `Sidebar`, `TopBar`, `AppGrid`/Launcher (9 puntos), `Modal`, `Avatar`, `List`, `Table`, `Toast`, `Tabs`, _form fields_, `ThemeProvider` (claro/oscuro), `Badge`, `Dropdown`, `Tooltip`, `EmptyState`, `Skeleton`, `AppShell`. Tokens y logos/iconos desde `branding/`. |
| **`@sytue/shell`**       | La **Sytue Shell**: topbar global, **app switcher**, **búsqueda global** (Sytue Hub), **centro de notificaciones** (SSE), **presencia** y **menú de cuenta** (logout SSO). Se monta en CADA app.                                                                                                                     |
| **`@sytue/auth`**        | Cliente **OIDC Authorization Code + PKCE** contra **Sytue ID** (servicio Go; discovery OIDC estándar `/.well-known/openid-configuration`, sin `/realms/`), **BFF** (cookie host-only `__Host-`, httpOnly), guard de sesión y refresh.                                                                                |
| **`@sytue/api-clients`** | SDKs tipados a los **servicios Go propios**: `id`, `drive` (REST+WebDAV), `docs` (REST + colab WS), `mail` (JMAP), `calendar` (CalDAV), `contacts` (CardDAV), `chat` (REST+WS), `meet` (REST + LiveKit), `hub`, `cloud`.                                                                                             |

## Apps (`apps/`)

`portal`, `console`, `cloud-dashboard`, `mail`, `drive`, `calendar`,
`contacts`, `docs`, `chat`, `meet`, `www`. Todas montan `@sytue/shell` y usan
`@sytue/ui` (sin UI ad-hoc) y consumen los servicios Go vía `@sytue/api-clients`.

## Reglas del design system

1. Componentes **solo** desde `@sytue/ui`; shell **solo** desde `@sytue/shell`.
2. Sin UI ad-hoc: si falta una variante, se añade a `@sytue/ui`.
3. Solo **tokens** (`branding/tokens/`), nunca HEX o px sueltos.
4. La **shell** se monta en toda superficie web.
5. Marca (logos/iconos) desde `branding/`.
6. Claro y oscuro siempre; accesibilidad **WCAG AA**.

## Scripts

```bash
bun install           # instala el workspace (Bun workspaces)
bun run build         # compila los paquetes compartidos
bun run typecheck     # TypeScript estricto en todo el monorepo
bun run lint          # ESLint (TS + a11y + Astro)
bun run format        # Prettier
```

## Uso de la shell en una app Astro

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

> Las variables OIDC (`SYTUE_OIDC_*`, `SYTUE_SESSION_SECRET`)
> se inyectan por entorno desde Scaleway Secret Manager / Sealed Secrets — **nunca**
> se escriben en el repositorio.
