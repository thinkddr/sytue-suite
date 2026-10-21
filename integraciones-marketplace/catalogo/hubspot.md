# HubSpot — SAML SSO (plan Enterprise)
1. HubSpot → **Settings → General → Log in → Single sign-on**.
2. Sube el certificado y pega las URLs de Sytue ID; **Name ID** `email`; identifier format «Email».
3. Copia el **SP Entity ID** (`https://app.hubspot.com/login/sso/…`) y el **ACS URL** a la ficha de Sytue Platform.
4. Activa «Require SSO» solo para los usuarios deseados.
**Verificar**: «Verify SSO» de HubSpot y entrada desde el lanzador.
