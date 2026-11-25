<a id="paquetes-para-los-marketplaces-de-los-vendors"></a>
# Packages for the vendors' marketplaces

What a customer administrator uploads to EACH vendor's portal to connect
that platform with Sytue (Integration Gateway, `services/integrations`). The
values derive from the catalog's own seeds
(`services/integrations/db/seeds/templates/*.json`): same OAuth endpoints and
same scopes, so that what the vendor accepts is exactly what the
gateway expects.

**Common redirect URI** (the one each vendor asks you to paste):

    https://integrations.sytue.eu/api/v1/oauth/callback

**Onboarding guides**: each vendor carries its `GUIA-ALTA.md` (complete step-by-step,
with exact paths in each portal, common problems and final verification)
INSIDE the zip. The seven above are written and **so is the REST of the catalog**:
a step-by-step guide per connector in `catalogo/<clave>.md`
(33 files, one per seed in
`services/integrations/db/seeds/templates/`), derived from the real
scopes/fields of each seed.

**Publishing**: `bash scripts/empaquetar-marketplace.sh` produces
`dist-marketplace/<vendor>.zip`; the `release-marketplace.yml` workflow uploads them
as assets of a GitHub release (`publicar-asset.sh`), from where
Sytue Platform serves them. A bundle can be regenerated at any time; the zips
are never edited by hand.

| Vendor | Package | Where it is uploaded |
|---|---|---|
| Slack | `slack/app-manifest.yml` | api.slack.com/apps → “Create from manifest” |
| Microsoft Teams | `microsoft-teams/` (zip with manifest + icons) | admin.teams.com → Teams apps → “Upload” |
| Google Drive | `google-drive/` | console.cloud.google.com (OAuth client) |
| OneDrive | `onedrive/` | portal.azure.com → Entra → App registrations |
| Zoom | `zoom/` | marketplace.zoom.us → “Develop → Build App” |
| Google Meet | `google-meet/` | console.cloud.google.com (OAuth client) |
| Google Chat | `google-chat/` | console.cloud.google.com → Chat API config |
