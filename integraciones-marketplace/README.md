# Paquetes para los marketplaces de los vendors

Lo que un administrador de cliente sube al portal de CADA vendor para conectar
esa plataforma con Sytue (Integration Gateway, `services/integrations`). Los
valores derivan de las semillas del propio catálogo
(`services/integrations/db/seeds/templates/*.json`): mismos endpoints OAuth y
mismos scopes, para que lo que el vendor acepta sea exactamente lo que el
gateway espera.

**Redirect URI común** (la que cada vendor pide pegar):

    https://integrations.sytue.eu/api/v1/oauth/callback

**Guías de alta**: cada vendor lleva su `GUIA-ALTA.md` (paso a paso completo,
con rutas exactas de cada portal, problemas frecuentes y verificación final)
DENTRO del zip. Las siete de arriba están escritas y **el RESTO del catálogo
también**: una guía paso a paso por conector en `catalogo/<clave>.md`
(33 ficheros, uno por semilla de
`services/integrations/db/seeds/templates/`), derivada de los scopes/fields
reales de cada semilla.

**Publicación**: `bash scripts/empaquetar-marketplace.sh` produce
`dist-marketplace/<vendor>.zip`; el workflow `release-marketplace.yml` los sube
como assets de una release de GitHub (`publicar-asset.sh`), de donde los sirve
Sytue Platform. Un bundle se vuelve a generar en cualquier momento; no se
editan los zip a mano.

| Vendor | Paquete | Dónde se carga |
|---|---|---|
| Slack | `slack/app-manifest.yml` | api.slack.com/apps → «Create from manifest» |
| Microsoft Teams | `microsoft-teams/` (zip con manifest + iconos) | admin.teams.com → Teams apps → «Upload» |
| Google Drive | `google-drive/` | console.cloud.google.com (OAuth client) |
| OneDrive | `onedrive/` | portal.azure.com → Entra → App registrations |
| Zoom | `zoom/` | marketplace.zoom.us → «Develop → Build App» |
| Google Meet | `google-meet/` | console.cloud.google.com (OAuth client) |
| Google Chat | `google-chat/` | console.cloud.google.com → Chat API config |
