<a id="asana--saml-sso-plan-enterpriseenterprise"></a>
# Asana — SAML SSO (Enterprise/Enterprise+ plan)
1. Asana Admin Console → **Settings → Authentication** → “SAML” → “Edit”.
2. IdP: paste the Sytue ID metadata. **Name ID format** `email` (or `emailAddress`).
3. Save and keep “SAML + password” until verified; then “SAML only”.
There are no fields to copy into Sytue Platform: Asana discovers the IdP by email domain.
**Verify**: log in as a user of the domain from the suite.
**Errors**: “Invalid audience” = the IdP's Entity ID is not exactly the one Asana expects (`https://app.asana.com/`).
