<a id="guía-de-alta--microsoft-teams--sytue"></a>
# Onboarding guide — Microsoft Teams ⇄ Sytue

Time: ~15 min. Permissions: Teams Administrator (or Global Admin) of the Entra
tenant. Result: “Sytue” app in Teams with the suite panel.

<a id="1-preparar-el-paquete"></a>
## 1. Prepare the package

The release zip already includes `manifest.json`, `color.png` and `outline.png`.
If you build it yourself: `bash scripts/empaquetar-marketplace.sh`.

<a id="2-fijar-el-id-de-la-app-una-sola-vez"></a>
## 2. Set the app ID (only once)

1. Generate a GUID (`uuidgen`).
2. In `manifest.json`, replace the value `"id": "00000000-…"` with your GUID.
   This ID is what Teams uses as the app's identity; do not change it again.

<a id="3-subir-a-teams"></a>
## 3. Upload to Teams

- **Admin center**: <https://admin.teams.com> → “Teams apps” →
  “Manage apps” → “Upload new app” → upload the zip → “Publish”.
- With custom app policies, also add the app to your organization's
  permission policies.

<a id="4-oauth-de-la-conexión-integration-gateway"></a>
## 4. Connection OAuth (Integration Gateway)

1. Go to <https://portal.azure.com> → “Microsoft Entra ID” → “App
   registrations” → “New registration”.
2. Name “Sytue Integration Gateway”, accounts “Any organizational directory”.
3. “Add a Redirect URI” (Web):
   `https://integrations.sytue.eu/api/v1/oauth/callback`.
4. Copy the **Application (client) ID** and create a **Client secret**.
5. Delegated permissions (Microsoft Graph): `User.Read`, `Team.ReadBasic.All`,
   `Channel.ReadBasic.All`, `ChannelMessage.Read.All` → “Grant admin consent”.

<a id="5-conectar-desde-sytue-platform"></a>
## 5. Connect from Sytue Platform

1. Sytue Platform → Integrations → “Microsoft Teams”.
2. Fill in **Tenant** (contoso.onmicrosoft.com or the GUID), **Client ID** and
   **Client Secret** from step 4. “Connect” and authorize.

<a id="6-verificar"></a>
## 6. Verify

- Teams → “Apps” → “Built for your org” → “Sytue” → the tab opens the gateway
  panel with your session.
- In Sytue Platform the integration page switches to “active”.

<a id="problemas-frecuentes"></a>
## Common problems

- “App not allowed”: your tenant's permission policy blocks third-party/uploaded
  apps — assign it a policy in “Permission policies”.
- `AADSTS50011`: the registered redirect URI does not match byte for byte.
