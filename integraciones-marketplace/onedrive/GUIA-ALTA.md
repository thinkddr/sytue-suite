<a id="guía-de-alta--microsoft-onedrive--sytue"></a>
# Onboarding guide — Microsoft OneDrive ⇄ Sytue

Time: ~15 min. Permissions: Application Administrator in Entra. Result: the
Sytue Drive copies are stored in the connected user's OneDrive.

<a id="1-registrar-la-aplicación-en-entra-id"></a>
## 1. Register the application in Entra ID

1. <https://portal.azure.com> → “Microsoft Entra ID” → “App registrations” →
   “New registration”.
2. Name “Sytue Integration Gateway”.
3. Account types: “Accounts in any organizational directory (Multitenant)”
   — this is what the seed expects in order to work with any tenant.
4. Redirect URI (**Web** platform):
   `https://integrations.sytue.eu/api/v1/oauth/callback`.

<a id="2-credenciales-y-permisos"></a>
## 2. Credentials and permissions

1. “Certificates & secrets” → “New client secret” → copy the VALUE now
   (it cannot be seen afterwards).
2. “API permissions” → “Microsoft Graph” → “Delegated permissions”:
   - `Files.ReadWrite`
   - `offline_access`
3. “Grant admin consent” if your tenant requires it.

<a id="3-conectar-desde-sytue-platform"></a>
## 3. Connect from Sytue Platform

1. Sytue Platform → Integraciones (Integrations) → “Microsoft OneDrive”.
2. Paste the **Application (client) ID** and the secret → «Conectar» (Connect).
3. Authorize with the Microsoft account that owns the target OneDrive.

<a id="4-verificar-y-operar"></a>
## 4. Verify and operate

- Upload a copy from Sytue Drive and check `Archivos → Sytue` in OneDrive.
- `AADSTS7000215` when connecting: you pasted the “Secret ID” instead of the “Value”.

<a id="nota-de-seguridad"></a>
## Security note

The gateway only requests delegated permissions of the connecting user (not the
application-level `Files.ReadWrite.All`), so the scope is limited to what
that user can touch in their own OneDrive.
