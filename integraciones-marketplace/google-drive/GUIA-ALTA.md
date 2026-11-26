<a id="guía-de-alta--google-drive--sytue"></a>
# Onboarding guide — Google Drive ⇄ Sytue

Time: ~15 min. Permissions: access to your organization's Google Cloud Console.
Result: new files in Sytue Drive are saved to your Google Drive.

<a id="1-crear-el-proyecto-oauth"></a>
## 1. Create the OAuth project

1. <https://console.cloud.google.com> → project selector → “New project”
   → name “Sytue Integration Gateway”.
2. “APIs & Services” → “Library” → search for **Google Drive API** → “Enable”.
3. “OAuth consent screen”: type **Internal** (Workspace) or **External**;
   fill in the app name “Sytue”, the support email and the domain `sytue.eu`.
   Add the scope `.../auth/drive.file` to the declared list.
4. If it is External and in “Testing”, add the test users.

<a id="2-crear-el-cliente-oauth"></a>
## 2. Create the OAuth client

1. “Credentials” → “Create credentials” → “OAuth client ID” → **Web
   application**.
2. “Authorized redirect URIs” → add EXACTLY:
   `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. Create it and copy the **Client ID** and **Client secret**.

<a id="3-conectar-desde-sytue-platform"></a>
## 3. Connect from Sytue Platform

1. Sytue Platform → Integrations → “Google Drive”.
2. Paste the Client ID and Client Secret → “Connect”.
3. Google asks for consent with `access_type=offline` and `prompt=consent`
   (already configured): the gateway stores a refresh token and the connection
   stays active without re-authorizing.

<a id="4-verificar-y-operar"></a>
## 4. Verify and operate

- In Sytue Drive, use “upload copy to Google Drive” on a file; check that it
  appears in the root folder of the connected user's Drive.
- The integration page shows the last sync.
- To revoke: Google Account → Security → Third-party access, or “Remove”
  on the integration page (the gateway calls the seed's revoke_url).

<a id="problemas-frecuentes"></a>
## Common problems

- `redirect_uri_mismatch`: the URI does not match byte for byte (no trailing slash).
- `access_denied`: the app is in Testing and the user is not a tester.
