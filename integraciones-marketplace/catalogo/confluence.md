# Confluence (Atlassian Cloud) — SAML SSO
1. admin.atlassian.com → **Security → SAML single sign-on** (requires Atlassian Guard).
2. IdP: Sytue ID metadata/certificate; **Name ID** `email`.
3. Copy the **Entity ID** and the **ACS URL** (`https://id.atlassian.com/login?…`) from Atlassian to the Sytue Platform integration page.
4. Assign the verified email domain to the SAML policy.
**Verify**: “Test SAML login” in the Atlassian panel and sign-in from the suite.
