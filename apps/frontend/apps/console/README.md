# Sytue Admin (`console`)

Superficie de **administracion** de Sytue One (`admin.sytue.eu`): organizaciones
(tenants), usuarios, grupos/roles y auditoria, consumiendo la **API de
administracion de Sytue ID** (identidad/SCIM) y **Sytue Hub**. **Astro 7 + Preact
10**, SSR sobre **Bun**, **Tailwind v4** y tokens de marca. Monta la **Sytue
Shell** y usa **solo** `@sytue/ui`. Soberanía **100 % UE**. Código **propietario
© Sytue** (SPDX `MIT`).

> Contrato: [`docs/CONVENTIONS.md`](../../../../docs/CONVENTIONS.md) §8, §9, §11.
> Puerto dev: **4322**. Client OIDC: **`sytue-console`**. Acceso: rol `admin`/`owner`.

## Pantallas

- **Panel** (`/`) — métricas de identidad, organizaciones y actividad reciente.
- **Organizaciones** (`/tenants`, `/tenants/:slug`) — tenants ⇄ grupo `org/<slug>` ⇄ namespace; alta y ficha.
- **Usuarios** (`/users`) — directorio con roles, MFA y **aprovisionamiento por servicio Sytue** (SCIM), detalle y acciones.
- **Grupos y roles** (`/groups`) — jerarquía `org/<slug>` y mapeos a workspace de Sytue Chat, carpeta de grupo de Sytue Drive, dominio de correo y namespace.
- **Auditoría** (`/audit`) — eventos de Sytue ID / Hub / servicios Sytue (requisito RGPD).

## Autenticación (BFF)

Endpoints Astro en `src/pages/auth/` (`@sytue/auth/server`). El `ConsoleLayout`
aplica `requireSession(..., { roles: ['admin','owner'] })`. Cookie host-only
`__Host-`, httpOnly. Sin secretos en el cliente.

## Variables de entorno

Inyectadas por entorno (Secret Manager / Sealed Secrets) — **nunca** en git.
Ver `.env.example`: `SYTUE_OIDC_*`, `SYTUE_SESSION_SECRET`,
`SYTUE_HUB_URL`, `SYTUE_ADMIN_API_URL`.

## Scripts

Runtime/instalación/bundler con **Bun** (sustituye a Node.js + pnpm).

```bash
bun install
bun --filter @sytue/app-console dev        # http://localhost:4322
bun --filter @sytue/app-console build
bun --filter @sytue/app-console typecheck
```

> Mientras se integra el control-plane, las vistas usan datos de ejemplo
> (`src/lib/data.ts`) sin secretos reales.
