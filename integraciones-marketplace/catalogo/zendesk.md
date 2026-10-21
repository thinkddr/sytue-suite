# Zendesk — SAML SSO (plan Enterprise)
1. Zendesk Admin Center → **Account → Security → Single sign-on → SAML**.
2. Copia el **ACS URL** (`https://TUZENDESK.zendesk.com/access/saml`) y el **Entity ID** a la ficha de Sytue Platform.
3. Sube el certificado de Sytue ID como «SAML certificate»; Name ID `email`.
4. «SAML only» (o por grupo) tras probar en ventana privada.
**Errores**: el usuario no existe en Zendesk = activa el aprovisionamiento JIT por dominio.
