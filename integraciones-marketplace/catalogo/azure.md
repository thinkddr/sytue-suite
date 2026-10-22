# Azure (Entra) — alta en Sytue Platform
**Qué conecta**: gestión de inquilino Entra ID desde la suite (OAuth con `user_impersonation` sobre ARM).
1. portal.azure.com → **Microsoft Entra ID → App registrations → New registration**; nombre «Sytue», tipos de cuenta «Single tenant».
2. **Redirect URI (Web)**: `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. «API permissions» → **Azure Service Management** → delegated `user_impersonation` → **Grant admin consent**.
4. Copia **Application (client) ID**; crea un **Client secret** y copia el VALOR.
5. Sytue Platform → Integraciones → **Azure**: rellena **Tenant** (dominio o GUID), Client ID y Secret → «Conectar».
**Verificar**: la ficha pasa a activa; el panel de Azure del gateway lista el tenant.
**Errores**: `AADSTS700016` = client_id mal; `AADSTS650051` = falta el permiso ARM con consent.
