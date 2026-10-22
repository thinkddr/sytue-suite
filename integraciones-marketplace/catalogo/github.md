# GitHub self-hosted (GitHub Enterprise Server) — alta en Sytue Platform
**Qué conecta**: tu instancia GHES por OIDC/identidad y API.
1. En GHES: **Admin → OAuth Apps → New OAuth App** con callback `https://integrations.sytue.eu/api/v1/oauth/callback`.
2. Copia Client ID/Secret de GHES.
3. Sytue Platform → Integraciones → **GitHub (self-hosted)**: rellena **Host** (`https://gh.tuempresa.com`), Client ID y Secret → «Conectar».
**Verificar**: la ficha resuelve tu usuario de GHES.
**Errores**: TLS no confiable = el certificado de GHES no está en el almacén del gateway (añádelo al bundle de CA).
