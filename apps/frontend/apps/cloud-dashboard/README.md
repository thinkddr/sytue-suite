# Sytue DevX — dashboard (`cloud-dashboard`)

Superficie del **PaaS de Sytue One** (`cloud.sytue.eu`): la DX de **Vercel**, los
servicios gestionados de **Supabase** y la simplicidad que **Google Cloud** no
tiene. **Astro 7 + Preact 10**, SSR sobre **Bun** (runtime + bundler), **Tailwind
v4** y tokens de marca. Monta la **Sytue Shell** y usa **solo** `@sytue/ui`
(lenguaje visual Sytue). Soberanía **100 % UE**. Producto **Sytue-native**,
**propietario** (© Sytue) — licencia `MIT`.

> Contrato: [`docs/CONVENTIONS.md`](../../../../docs/CONVENTIONS.md) §14.
> Puerto dev: **4323**. Client OIDC: **`sytue-cloud`**.

## Qué incluye

Project-centric (estilo Vercel): pocas pantallas, lenguaje claro.

- **Proyectos** (`/`) — tarjetas con repo, URL de producción y servicios.
- **Resumen** (`/projects/:slug`) — producción actual, uso, servicios y últimos despliegues.
- **Despliegues** (`/projects/:slug/deployments`) — previews por PR, promover a producción y **rollback**.
- **Base de datos** (`/projects/:slug/database`) — **Sytue DB Studio** (estilo Supabase):
  - **Explorador de tablas** — lista por esquema, datos y definición de columnas (PK/FK/tipos).
  - **Editor SQL** — editor real con snippets, `Ctrl/Cmd+Enter` y tabla de resultados.
  - **API y credenciales** — endpoint REST automático (PostgREST), claves enmascaradas, ejemplos cURL/JS.
- **Almacenamiento** (`/projects/:slug/storage`) — bucket S3 por proyecto (Object Storage / MinIO).
- **Funciones** (`/projects/:slug/functions`) — serverless (Sytue Functions).
- **Dominios** (`/projects/:slug/domains`) — CNAME a `ingress.sytue.eu` + TLS automático.
- **Variables y secretos** (`/projects/:slug/env`) — inyección por servicio; secretos en Secret Manager.
- **Registros** (`/projects/:slug/logs`) — logs build/runtime + métricas en lenguaje claro.

## Autenticación (BFF)

Endpoints Astro en `src/pages/auth/` (`@sytue/auth/server`): `login`, `callback`,
`logout`, `session`, `refresh`. Emiten una cookie host-only `__Host-`, httpOnly,
tras OIDC Authorization Code + PKCE. **Sin secretos en el cliente.**

## Variables de entorno

Inyectadas por entorno (Scaleway Secret Manager / Sealed Secrets) — **nunca** en git:
`SYTUE_OIDC_ISSUER`, `SYTUE_OIDC_CLIENT_ID`, `SYTUE_OIDC_CLIENT_SECRET`,
`SYTUE_OIDC_REDIRECT_URI`, `SYTUE_SESSION_SECRET`,
`SYTUE_HUB_URL`, `SYTUE_CLOUD_API_URL`.

## Scripts

```bash
bun run --filter @sytue/app-cloud-dashboard dev    # http://localhost:4323
bun run --filter @sytue/app-cloud-dashboard build
bun run --filter @sytue/app-cloud-dashboard typecheck
```

> El control-plane real lo sirve la **Sytue DevX API**; mientras se integra, las
> vistas usan datos de ejemplo (`src/lib/data.ts`) sin secretos reales.
