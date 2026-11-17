# GitHub (github.com) — setup in Sytue Platform
**What it connects**: repos, organizations and gists of the connecting user (scopes repo/read:org/gist).
1. github.com → **Settings → Developer settings → OAuth Apps → New OAuth App**.
2. Application name “Sytue”, homepage `https://sytue.eu`, **Authorization callback URL**: `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. Copy the **Client ID** and generate a **Client secret**.
4. Sytue Platform → Integrations → **GitHub (github.com)**: paste both → “Connect” → authorize with the scopes `read:user user:email repo read:org gist`.
**Verify**: search federation returns your private repos.
**Note**: if the organization has **mandatory SAML SSO**, also authorize the token for each org (Settings → Organizations → “Authorize”).
