# Sytue Drive (`drive.sytue.eu`)

**Sytue One**'s own file client (Astro 7 + Preact 10) on top of the
**Sytue Drive Go service** via its REST API + **WebDAV** (`@sytue/api-clients`
→ `DriveClient`). **Clean-room** implementation, with no third-party engines. UX and
branding 100 % Sytue.

<a id="funcionalidad"></a>
## Features

- Folder explorer with **breadcrumb** and navigation.
- **List** and **grid** views.
- **Upload** files (multi-file), **create folder**, **download**, **delete**.
- **Share** via public link (the Go service's shares API).
- Section sidebar (Mi unidad, Compartido, Recientes, Favoritos, Papelera — My drive, Shared, Recent, Favorites, Trash).

<a id="diseno-e-integracion"></a>
## Design and integration

- `@sytue/ui` + `@sytue/shell`, light/dark mode, WCAG AA.
- **Sytue Shell** (`@sytue/shell`) mounted: top bar, app switcher, search,
  notifications, presence and account.
- Identity: OIDC/PKCE against **Sytue ID** (client `sytue-drive`) via the
  `@sytue/auth` BFF; host-only session cookie `__Host-`, httpOnly. The Sytue
  ID access token authorizes the REST/WebDAV calls to the Go service.

<a id="desarrollo"></a>
## Development

Dev port **4326** (`docs/CONVENTIONS.md` §6). Sytue Drive Go service on
`8082`.

```bash
bun run --filter @sytue/drive dev       # http://localhost:4326
bun run --filter @sytue/drive build
bun run --filter @sytue/drive typecheck
```

<a id="variables-de-entorno-bff-solo-servidor"></a>
## Environment variables (BFF, server only)

`SYTUE_OIDC_ISSUER` (Sytue ID issuer, without `/realms/`),
`SYTUE_OIDC_CLIENT_ID=sytue-drive`, `SYTUE_OIDC_CLIENT_SECRET`,
`SYTUE_OIDC_REDIRECT_URI`, `SYTUE_SESSION_SECRET`,
`PUBLIC_SYTUE_HUB_BASE_URL`,
`PUBLIC_SYTUE_DRIVE_BASE_URL` (Sytue Drive Go service), `SYTUE_SECURE_COOKIES`.
Secrets are never included in git (Scaleway Secret Manager / Sealed Secrets).
