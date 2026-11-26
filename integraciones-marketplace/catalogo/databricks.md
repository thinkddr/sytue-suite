<a id="databricks--alta-en-sytue-platform"></a>
# Databricks — onboarding in Sytue Platform
**What it connects**: running SQL statements on a SQL warehouse in the workspace (OAuth `all-apis`).
1. In your workspace: **Settings → Identity and access → OAuth configuration → Add OAuth configuration**.
2. Name “Sytue”, redirect URI `https://integrations.sytue.eu/api/v1/oauth/callback`, scopes `all-apis`, TTL the longest allowed.
3. Copy the **Client ID** (starts with `databricks-oauth-`) and the secret.
4. Copy your **workspace_url** (the root, `https://adb-….azuredatabricks.net`) and the **warehouse_id** (SQL Warehouses → the ID starting with `a1…`).
5. Sytue Platform → Integrations → **Databricks**: workspace_url, warehouse_id, client_id, secret → “Connect” and authorize.
**Verify**: a `SELECT 1` statement from the gateway panel.
**Errors**: `INVALID_PARAMETER_VALUE` on the code exchange = redirect URI differs byte for byte.
