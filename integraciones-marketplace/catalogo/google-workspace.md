<a id="google-workspace--saml--oauth--drivegmailcalendarcontactos-de-solo-lectura"></a>
# Google Workspace — SAML + OAuth (read-only Drive/Gmail/Calendar/Contacts)
**A. SAML SSO** (optional, for unified identity):
1. admin.google.com → **Security → Authentication → Set up SSO with a third-party IdP**; upload the Sytue ID certificate and URLs.
**B. Data via OAuth** (required for federation):
1. console.cloud.google.com → project → enable the Drive, Gmail and Calendar APIs.
2. OAuth consent screen **Internal** + the seed scopes (all readonly).
3. Create a **service account**, grant it **Domain-wide delegation** in Workspace (Security → API Controls → Domain-wide Delegation) with the same scopes, and note down the **admin_email** to impersonate.
4. Sytue Platform → Integrations → **Google Workspace**: **Domain** and **admin_email** + the service account JSON → “Connect”.
**Verify**: federation finds a real email and a real calendar event.
**Errors**: `unauthorized_client` = the delegation scope does not match the consented one.
