<a id="paquetes-y-manifiestos-de-tiendas-y-marketplaces-oficiales"></a>
# Packages and Manifests for Official Stores and Marketplaces

This directory contains the application packages and manifests ready to publish in the official third-party stores for Sytue Rooms and Sytue One:

- **Slack App Directory**: `slack/manifest.json`
- **Microsoft Teams / AppSource**: `teams/manifest.json`
- **Google Workspace Marketplace**: `google-workspace/manifest.json`
- **Zoom App Marketplace**: `zoom/manifest.json`

<a id="flujo-de-trabajo-para-organizaciones-y-usuarios"></a>
## Workflow for Organizations and Users

1. **Global Configuration in Sytue Admin**:
   - The organization administrator or the individual user opens **Sytue Admin** (`https://console.sytue.eu/admin/apps`).
   - Clicks **Conectar** (Connect) on the desired application (Slack, Microsoft Teams, Google Workspace / Meet / Chat, Zoom).
   - Authorizes with a single click via OAuth2. The credentials are stored securely and centrally.

2. **Use in Sytue Rooms**:
   - Any team member opens the **Integraciones** (Integrations) tab or clicks **Conectar con otras Apps** (Connect with other apps).
   - Selects the integration already configured in Admin.
   - The channel or video-call room is linked automatically and bidirectionally, with no need to copy and paste webhooks by hand.

<a id="guía-de-publicación-oficial-por-plataforma"></a>
## Official Publishing Guide by Platform

### 1. Slack (`slack/manifest.json`)
1. Sign in at [api.slack.com/apps](https://api.slack.com/apps).
2. Click **Create New App** → **From an app manifest**.
3. Select the development workspace and paste the contents of `slack/manifest.json`.
4. Under **OAuth & Permissions**, check that the redirect URL points to `https://integrations.sytue.eu/api/v1/oauth/callback`.
5. Request approval for the Slack App Directory from the **Manage Distribution** section.

### 2. Microsoft Teams (`teams/manifest.json`)
1. Open the **Microsoft Teams Developer Portal** ([dev.teams.microsoft.com](https://dev.teams.microsoft.com/)).
2. Select **Apps** → **Import app** and upload the zip package with `manifest.json`, `color.png` (192x192) and `outline.png` (32x32).
3. Configure the Azure AD application ID under **Single Sign-On / App Registration**.
4. Publish the app to the organization's tenant catalog or submit it for Microsoft AppSource certification.

### 3. Google Workspace (`google-workspace/manifest.json`)
1. Go to [Google Cloud Console](https://console.cloud.google.com/) in the Sytue One project.
2. Enable the **Google Chat API** and the **Google Workspace Marketplace SDK**.
3. In the add-on or script configuration, import the `google-workspace/manifest.json` manifest.
4. In the Marketplace SDK, fill in the public listing details (graphics, privacy policy) and submit it for review.

### 4. Zoom (`zoom/manifest.json`)
1. Go to [marketplace.zoom.us](https://marketplace.zoom.us/develop/create).
2. Create a new app of type **OAuth App** (Account-level or User-level app).
3. Enter the redirect URLs (`https://integrations.sytue.eu/api/v1/oauth/callback`) and add the scopes declared in the manifest.
4. Submit it for Zoom's technical and security review.
