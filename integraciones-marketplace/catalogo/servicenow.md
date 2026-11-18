<a id="servicenow--alta-en-sytue-platform"></a>
# ServiceNow — onboarding in Sytue Platform
**What it connects**: the user's OAuth identity (`useraccount`) and reading records.
1. In your instance: **System OAuth → Application Registry → New → Connect to a third party OAuth provider** (or “Create an OAuth 2.0 API endpoint for external clients”).
2. Redirect URI: `https://integrations.sytue.eu/api/v1/oauth/callback`. Copy the Client ID/Secret.
3. The connecting user needs the read role for the tables to be federated (e.g. `itil` for incidents).
4. Sytue Platform → Integrations → **ServiceNow**: fill in **Instance** (`https://INSTANCIA.service-now.com`), Client ID and Secret → “Connect”.
**Verify**: read a sample record (sys_user or incident).
**Errors**: `access_denied` = the OAuth 2.0 plugin is not active on the instance.
