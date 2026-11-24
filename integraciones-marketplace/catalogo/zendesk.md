<a id="zendesk--saml-sso-plan-enterprise"></a>
# Zendesk — SAML SSO (Enterprise plan)
1. Zendesk Admin Center → **Account → Security → Single sign-on → SAML**.
2. Copy the **ACS URL** (`https://TUZENDESK.zendesk.com/access/saml`) and the **Entity ID** to the Sytue Platform integration page.
3. Upload the Sytue ID certificate as “SAML certificate”; Name ID `email`.
4. “SAML only” (or per group) after testing in a private window.
**Errors**: the user does not exist in Zendesk = enable JIT provisioning by domain.
