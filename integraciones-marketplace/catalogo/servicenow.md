# ServiceNow — alta en Sytue Platform
**Qué conecta**: identidad OAuth del usuario (`useraccount`) y lectura de registros.
1. En tu instancia: **System OAuth → Application Registry → New → Connect to a third party OAuth provider** (o «Create an OAuth 2.0 API endpoint for external clients»).
2. Redirect URI: `https://integrations.sytue.eu/api/v1/oauth/callback`. Copia Client ID/Secret.
3. El usuario que conecta necesita el rol de lectura de las tablas a federar (p. ej. `itil` para incidentes).
4. Sytue Platform → Integraciones → **ServiceNow**: rellena **Instancia** (`https://INSTANCIA.service-now.com`), Client ID y Secret → «Conectar».
**Verificar**: lectura de un registro de ejemplo (sys_user o incident).
**Errores**: `access_denied` = el plugin OAuth 2.0 no está activo en la instancia.
