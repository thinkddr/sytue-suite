# Jira (Atlassian Cloud) — alta en Sytue Platform
**Qué conecta**: lectura de incidencias y usuarios (read:jira-work, read:jira-user) por OAuth de Atlassian.
1. <https://developer.atlassian.com/console/myapps/> → **OAuth 2.0 (3LO) apps → Create**.
2. Nombre «Sytue», **Redirect URL**: `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. En **Permissions** añade la API **Jira platform REST API** con `read:jira-work` y `read:jira-user`.
4. Copia **Client ID** y **Client secret**.
5. Sytue Platform → Integraciones → **Jira**: rellena **Site** (`SITIO.atlassian.net`) y, si usas token en vez de consentimiento, el **email** de la cuenta API.
6. «Conectar» → consiente en Atlassian.
**Verificar**: búsqueda federada devuelve incidencias del sitio.
**Errores**: `Grant type not authorized` = la 3LO está apagada en la app; sitios vacíos = falta añadir el sitio a la app en «Authorization».
