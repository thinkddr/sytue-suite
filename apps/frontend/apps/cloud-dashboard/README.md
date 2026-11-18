# Sytue DevX — dashboard (`cloud-dashboard`)

The surface of the **Sytue One PaaS** (`cloud.sytue.eu`): the DX of **Vercel**, the
managed services of **Supabase** and the simplicity that **Google Cloud** does not
have. **Astro 7 + Preact 10**, SSR on **Bun** (runtime + bundler), **Tailwind
v4** and brand tokens. It mounts the **Sytue Shell** and uses **only** `@sytue/ui`
(Sytue visual language). **100% EU** sovereignty. A **Sytue-native**,
**proprietary** product (© Sytue) — `MIT` license.

> Contract: [`docs/CONVENTIONS.md`](../../../../docs/CONVENTIONS.md) §14.
> Dev port: **4323**. OIDC client: **`sytue-cloud`**.

<a id="qué-incluye"></a>

## What it includes

Project-centric (Vercel style): few screens, plain language.

- **Projects** (`/`) — cards with repo, production URL and services.
- **Overview** (`/projects/:slug`) — current production, usage, services and latest deployments.
- **Deployments** (`/projects/:slug/deployments`) — per-PR previews, promote to production and **rollback**.
- **Database** (`/projects/:slug/database`) — **Sytue DB Studio** (Supabase style):
  - **Table explorer** — list by schema, data and column definitions (PK/FK/types).
  - **SQL editor** — a real editor with snippets, `Ctrl/Cmd+Enter` and a results table.
  - **API and credentials** — automatic REST endpoint (PostgREST), masked keys, cURL/JS examples.
- **Storage** (`/projects/:slug/storage`) — an S3 bucket per project (Object Storage / MinIO).
- **Functions** (`/projects/:slug/functions`) — serverless (Sytue Functions).
- **Domains** (`/projects/:slug/domains`) — CNAME to `ingress.sytue.eu` + automatic TLS.
- **Variables and secrets** (`/projects/:slug/env`) — per-service injection; secrets in Secret Manager.
- **Logs** (`/projects/:slug/logs`) — build/runtime logs + metrics in plain language.

<a id="autenticación-bff"></a>

## Authentication (BFF)

Astro endpoints in `src/pages/auth/` (`@sytue/auth/server`): `login`, `callback`,
`logout`, `session`, `refresh`. They issue a host-only `__Host-`, httpOnly cookie
after OIDC Authorization Code + PKCE. **No secrets on the client.**

<a id="variables-de-entorno"></a>

## Environment variables

Injected per environment (Scaleway Secret Manager / Sealed Secrets) — **never** in git:
`SYTUE_OIDC_ISSUER`, `SYTUE_OIDC_CLIENT_ID`, `SYTUE_OIDC_CLIENT_SECRET`,
`SYTUE_OIDC_REDIRECT_URI`, `SYTUE_SESSION_SECRET`,
`SYTUE_HUB_URL`, `SYTUE_CLOUD_API_URL`.

## Scripts

```bash
bun run --filter @sytue/app-cloud-dashboard dev    # http://localhost:4323
bun run --filter @sytue/app-cloud-dashboard build
bun run --filter @sytue/app-cloud-dashboard typecheck
```

> The real control plane is served by the **Sytue DevX API**; while it is being integrated, the
> views use sample data (`src/lib/data.ts`) with no real secrets.
