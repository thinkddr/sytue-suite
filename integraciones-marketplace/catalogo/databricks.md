# Databricks — alta en Sytue Platform
**Qué conecta**: ejecución de sentencias SQL sobre un SQL warehouse del workspace (OAuth `all-apis`).
1. En tu workspace: **Settings → Identity and access → OAuth configuration → Add OAuth configuration**.
2. Nombre «Sytue», redirect URI `https://integrations.sytue.eu/api/v1/oauth/callback`, scopes `all-apis`, TTL el mayor que permita.
3. Copia el **Client ID** (empieza por `databricks-oauth-`) y el secreto.
4. Copia tu **workspace_url** (la raíz, `https://adb-….azuredatabricks.net`) y el **warehouse_id** (SQL Warehouses → el ID que empieza por `a1…`).
5. Sytue Platform → Integraciones → **Databricks**: workspace_url, warehouse_id, client_id, secreto → «Conectar» y autoriza.
**Verificar**: una sentencia `SELECT 1` desde el panel del gateway.
**Errores**: `INVALID_PARAMETER_VALUE` en el canje = redirect URI distinto byte a byte.
