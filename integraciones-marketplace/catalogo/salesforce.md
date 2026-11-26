<a id="salesforce--alta-en-sytue-platform"></a>
# Salesforce — onboarding in Sytue Platform
**What it connects**: SAML SSO + REST API (`api`, `refresh_token`) of your org through its My Domain.
1. Salesforce Setup → **Company Settings → My Domain**: copy your **subdomain** (`https://SUBDOMINIO.my.salesforce.com`).
2. **OAuth**: Setup → **App Manager → New Connected App** (“Create a Connected App”):
   - Enable OAuth: callback `https://integrations.sytue.eu/api/v1/oauth/callback`,
   - Scopes: `api`, `refresh_token`, `offline_access`,
   - “Enable OAuth Settings” + relax “Require secret for Web Server Flow” according to your policy.
3. Copy the **Consumer Key/Secret**.
4. **SAML** (if you federate identity): Setup → Single Sign-On → use the Sytue ID metadata; Salesforce's Entity ID/ACS go into the Sytue Platform integration page.
5. Sytue Platform → Integrations → **Salesforce**: subdomain + OAuth credentials → “Connect”.
**Verify**: a test SOQL query from the panel; SSO login from a private window.
**Errors**: `invalid_grant` = IP not in “Network Access” (add the gateway's egress IPs).
