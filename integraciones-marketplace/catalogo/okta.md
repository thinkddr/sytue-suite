# Okta — OIDC
1. Okta Admin → **Applications → Create App Integration → OIDC - OpenID Connect → Web Application**.
2. Redirect URI: `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. Assign users/groups; copy the **Client ID** and **Client secret**.
4. Your Okta **domain** (`https://TUORG.okta.com` or a custom org domain) is the only thing the detail card asks for.
5. Sytue Platform → Integraciones (Integrations) → **Okta**: domain + credentials → «Conectar» (Connect).
**Verify**: the gateway discovers `/.well-known/openid-configuration` for your domain.
