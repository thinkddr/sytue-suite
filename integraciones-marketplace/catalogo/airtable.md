<a id="airtable--saml-sso-plan-enterprise"></a>
# Airtable — SAML SSO (Enterprise plan)
1. Airtable admin → **Enterprise Hub → Settings → Identity & access → SSO**.
2. Add a SAML provider with the **Sytue ID metadata** (Platform → Integrations → Airtable → “IdP metadata”).
3. Copy the **Entity ID** and the **ACS URL** from Airtable and paste them into the Sytue Platform integration page.
4. Enable “Restrict to SSO” only after testing the login in a private window.
**Verify**: an assigned user signs in from the gateway launcher.
