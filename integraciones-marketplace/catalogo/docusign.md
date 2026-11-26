# DocuSign — SAML SSO
1. DocuSign eSignature Admin → **Identity Providers → Add Identity Provider**.
2. Upload the Sytue ID metadata; **Name ID format** `emailAddress`.
3. Copy DocuSign's **SP Entity ID** and **ACS URL** into the Sytue Platform card.
4. In “Claim Settings” choose the email attribute; create the “Signing Certificate” if your flow requires it.
**Verify**: DocuSign's own “Test SSO” before enabling it for everyone.
