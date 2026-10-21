# Confluence (Atlassian Cloud) — SAML SSO
1. admin.atlassian.com → **Security → SAML single sign-on** (exige Atlassian Guard).
2. IdP: metadatos/certificado de Sytue ID; **Name ID** `email`.
3. Copia de Atlassian el **Entity ID** y el **ACS URL** (`https://id.atlassian.com/login?…`) a la ficha de Sytue Platform.
4. Asigna el dominio de correo verificado a la política SAML.
**Verificar**: «Test SAML login» del panel de Atlassian y entrada desde la suite.
