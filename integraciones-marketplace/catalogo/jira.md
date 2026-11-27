<a id="jira-atlassian-cloud--alta-en-sytue-platform"></a>
# Jira (Atlassian Cloud) — setup in Sytue Platform
**What it connects**: read access to issues and users (read:jira-work, read:jira-user) via Atlassian OAuth.
1. <https://developer.atlassian.com/console/myapps/> → **OAuth 2.0 (3LO) apps → Create**.
2. Name “Sytue”, **Redirect URL**: `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. Under **Permissions**, add the **Jira platform REST API** with `read:jira-work` and `read:jira-user`.
4. Copy the **Client ID** and **Client secret**.
5. Sytue Platform → Integrations → **Jira**: fill in **Site** (`SITIO.atlassian.net`) and, if you use a token instead of consent, the **email** of the API account.
6. “Connect” → grant consent in Atlassian.
**Verify**: federated search returns issues from the site.
**Errors**: `Grant type not authorized` = 3LO is disabled on the app; empty sites = the site has not been added to the app under “Authorization”.
