<a id="azure-entra--alta-en-sytue-platform"></a>
# Azure (Entra) — onboarding in Sytue Platform
**What it connects**: Entra ID tenant management from the suite (OAuth with `user_impersonation` over ARM).
1. portal.azure.com → **Microsoft Entra ID → App registrations → New registration**; name “Sytue”, account types “Single tenant”.
2. **Redirect URI (Web)**: `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. “API permissions” → **Azure Service Management** → delegated `user_impersonation` → **Grant admin consent**.
4. Copy the **Application (client) ID**; create a **Client secret** and copy its VALUE.
5. Sytue Platform → Integraciones (Integrations) → **Azure**: fill in **Tenant** (domain or GUID), Client ID and Secret → «Conectar» (Connect).
**Verify**: the detail card becomes active; the gateway's Azure panel lists the tenant.
**Errors**: `AADSTS700016` = wrong client_id; `AADSTS650051` = the ARM permission with consent is missing.
