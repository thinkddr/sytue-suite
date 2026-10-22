# Okta — OIDC
1. Okta Admin → **Applications → Create App Integration → OIDC - OpenID Connect → Web Application**.
2. Redirect URI: `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. Asigna usuarios/grupos; copia **Client ID** y **Client secret**.
4. Tu **dominio** de Okta (`https://TUORG.okta.com` u org custom) es lo único que pide la ficha.
5. Sytue Platform → Integraciones → **Okta**: dominio + credenciales → «Conectar».
**Verificar**: el gateway descubre `/.well-known/openid-configuration` de tu dominio.
