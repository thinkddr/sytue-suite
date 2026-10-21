# Sytue Drive (`drive.sytue.eu`)

Cliente propio de archivos de **Sytue One** (Astro 7 + Preact 10) sobre el
**servicio Go de Sytue Drive** via su API REST + **WebDAV** (`@sytue/api-clients`
→ `DriveClient`). Implementacion **clean-room**, sin motores de terceros. UX y
marca 100 % Sytue.

## Funcionalidad

- Explorador de carpetas con **breadcrumb** y navegacion.
- Vistas **lista** y **rejilla**.
- **Subir** archivos (multiarchivo), **crear carpeta**, **descargar**, **eliminar**.
- **Compartir** mediante enlace publico (API de shares del servicio Go).
- Sidebar de secciones (Mi unidad, Compartido, Recientes, Favoritos, Papelera).

## Diseno e integracion

- `@sytue/ui` + `@sytue/shell`, modo claro/oscuro, WCAG AA.
- **Sytue Shell** (`@sytue/shell`) montada: topbar, app switcher, busqueda,
  notificaciones, presencia y cuenta.
- Identidad: OIDC/PKCE contra **Sytue ID** (client `sytue-drive`) via el BFF de
  `@sytue/auth`; cookie de sesión host-only `__Host-`, httpOnly. El access token de Sytue
  ID autoriza las llamadas REST/WebDAV al servicio Go.

## Desarrollo

Puerto dev **4326** (`docs/CONVENTIONS.md` §6). Servicio Go de Sytue Drive en
`8082`.

```bash
bun run --filter @sytue/drive dev       # http://localhost:4326
bun run --filter @sytue/drive build
bun run --filter @sytue/drive typecheck
```

## Variables de entorno (BFF, solo servidor)

`SYTUE_OIDC_ISSUER` (issuer de Sytue ID, sin `/realms/`),
`SYTUE_OIDC_CLIENT_ID=sytue-drive`, `SYTUE_OIDC_CLIENT_SECRET`,
`SYTUE_OIDC_REDIRECT_URI`, `SYTUE_SESSION_SECRET`,
`PUBLIC_SYTUE_HUB_BASE_URL`,
`PUBLIC_SYTUE_DRIVE_BASE_URL` (servicio Go de Sytue Drive), `SYTUE_SECURE_COOKIES`.
Nunca se incluyen secretos en git (Scaleway Secret Manager / Sealed Secrets).
