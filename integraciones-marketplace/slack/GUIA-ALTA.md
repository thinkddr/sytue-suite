<a id="guía-de-alta--slack--sytue"></a>
# Setup guide — Slack ⇄ Sytue

Time: ~10 min. Permissions: workspace admin. Result: Slack connected via
OAuth (channels, incoming notifications) and SAML/SCIM if your plan supports them.

<a id="1-crear-la-app-de-slack"></a>
## 1. Create the Slack app

1. Go to <https://api.slack.com/apps> with your Slack account.
2. “Create New App” → “From an app manifest” → choose your workspace.
3. Paste the contents of `app-manifest.yml` (in this same zip) → “Next” → “Create”.

The manifest already declares the scopes, the installation page and the
Integration Gateway redirect URI (`https://integrations.sytue.eu/api/v1/oauth/callback`).

<a id="2-copiar-las-credenciales"></a>
## 2. Copy the credentials

1. “Basic Information” → “App Credentials”.
2. Copy the **Client ID** and **Client Secret**.
3. In “OAuth & Permissions”, check that “Redirect URLs” contains exactly
   the URI from step 1 (the manifest fills it in; if Slack marks it as pending,
   click “Add” and “Save URLs”).

<a id="3-conectar-desde-sytue-platform"></a>
## 3. Connect from Sytue Platform

1. Go to Sytue Platform → Integrations → “Slack”.
2. Fill in the **Workspace** field (the one in `WORKSPACE.slack.com`) and the two
   values from step 2.
3. “Connect” takes you to Slack: review the permissions and authorize.

<a id="4-opcional-saml-sso-y-scim"></a>
## 4. (Optional) SAML SSO and SCIM

- **SAML**: in Slack, “Settings and Permissions” → “Sign in with Slack” /
  SAML: load the Sytue ID metadata and use the ACS
  `https://WORKSPACE.slack.com/sso/saml`, NameID `email`.
- **SCIM**: generate a SCIM API token in Slack and paste it into the integration's
  detail card in Sytue Platform; user and group provisioning goes
  through `https://api.slack.com/scim/v2`.

<a id="5-verificar"></a>
## 5. Verify

1. “OAuth & Permissions” → “Install to Workspace” if Slack asks for it.
2. In Sytue Platform, the Slack detail card switches to “active” with the latest sync.
3. Send a test notification from the suite; it must appear in the chosen channel.

<a id="problemas-frecuentes"></a>
## Common problems

- `redirect_uri_mismatch`: the Slack Redirect URL and the one on the detail card are not
  byte-for-byte identical (no trailing slash).
- The app does not appear under “From an app manifest”: the chosen workspace is not the
  right one; go back to step 1.
