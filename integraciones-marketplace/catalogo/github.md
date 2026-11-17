<a id="github-self-hosted-github-enterprise-server--alta-en-sytue-platform"></a>
# GitHub self-hosted (GitHub Enterprise Server) — setup in Sytue Platform
**What it connects**: your GHES instance via OIDC/identity and API.
1. In GHES: **Admin → OAuth Apps → New OAuth App** with callback `https://integrations.sytue.eu/api/v1/oauth/callback`.
2. Copy the GHES Client ID/Secret.
3. Sytue Platform → Integrations → **GitHub (self-hosted)**: fill in **Host** (`https://gh.tuempresa.com`), Client ID and Secret → “Connect”.
**Verify**: the listing resolves your GHES user.
**Errors**: untrusted TLS = the GHES certificate is not in the gateway's trust store (add it to the CA bundle).
