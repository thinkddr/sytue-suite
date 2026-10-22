# DocuSign — SAML SSO
1. DocuSign eSignature Admin → **Identity Providers → Add Identity Provider**.
2. Sube los metadatos de Sytue ID; **Name ID format** `emailAddress`.
3. Copia el **SP Entity ID** y el **ACS URL** de DocuSign a la ficha de Sytue Platform.
4. En «Claim Settings» elige el atributo de correo; crea el «Signing Certificate» si tu flujo lo exige.
**Verificar**: «Test SSO» del propio DocuSign antes de activarlo para todos.
