# GitHub (github.com) — alta en Sytue Platform
**Qué conecta**: repos, organizaciones y gists del usuario que conecta (scopes repo/read:org/gist).
1. github.com → **Settings → Developer settings → OAuth Apps → New OAuth App**.
2. Application name «Sytue», homepage `https://sytue.eu`, **Authorization callback URL**: `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. Copia **Client ID** y genera **Client secret**.
4. Sytue Platform → Integraciones → **GitHub (github.com)**: pega ambos → «Conectar» → autoriza con los scopes `read:user user:email repo read:org gist`.
**Verificar**: la federación de búsqueda devuelve tus repos privados.
**Nota**: si la organización tiene **SSO SAML obligatorio**, autoriza además el token para cada org (Settings → Organizations → «Authorize»).
