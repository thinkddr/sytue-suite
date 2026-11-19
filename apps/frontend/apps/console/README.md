# Sytue Admin (`console`)

**Administration** surface of Sytue One (`admin.sytue.eu`): organizations
(tenants), users, groups/roles and audit, consuming the **Sytue ID
administration API** (identity/SCIM) and **Sytue Hub**. **Astro 7 + Preact
10**, SSR on **Bun**, **Tailwind v4** and brand tokens. It mounts the **Sytue
Shell** and uses **only** `@sytue/ui`. **100 % EU** sovereignty. **Proprietary**
code **© 2026 Daniel Delgado Ramos (thinkddr)** (Apache-2.0).

> Contract: [`docs/CONVENTIONS.md`](../../../../docs/CONVENTIONS.md) §8, §9, §11.
> Dev port: **4322**. OIDC client: **`sytue-console`**. Access: `admin`/`owner` role.

<a id="pantallas"></a>
## Screens

- **Dashboard** (`/`) — identity, organization and recent activity metrics.
- **Organizations** (`/tenants`, `/tenants/:slug`) — tenants ⇄ group `org/<slug>` ⇄ namespace; creation and detail page.
- **Users** (`/users`) — directory with roles, MFA and **per-Sytue-service provisioning** (SCIM), detail and actions.
- **Groups and roles** (`/groups`) — `org/<slug>` hierarchy and mappings to the Sytue Chat workspace, Sytue Drive group folder, mail domain and namespace.
- **Audit** (`/audit`) — events from Sytue ID / Hub / Sytue services (GDPR requirement).

<a id="autenticación-bff"></a>
## Authentication (BFF)

Astro endpoints in `src/pages/auth/` (`@sytue/auth/server`). The `ConsoleLayout`
applies `requireSession(..., { roles: ['admin','owner'] })`. Host-only cookie
`__Host-`, httpOnly. No secrets on the client.

<a id="variables-de-entorno"></a>
## Environment variables

Injected per environment (Secret Manager / Sealed Secrets) — **never** in git.
See `.env.example`: `SYTUE_OIDC_*`, `SYTUE_SESSION_SECRET`,
`SYTUE_HUB_URL`, `SYTUE_ADMIN_API_URL`.

## Scripts

Runtime/installation/bundler with **Bun** (replaces Node.js + pnpm).

```bash
bun install
bun --filter @sytue/app-console dev        # http://localhost:4322
bun --filter @sytue/app-console build
bun --filter @sytue/app-console typecheck
```

> While the control plane is being integrated, the views use sample data
> (`src/lib/data.ts`) with no real secrets.
