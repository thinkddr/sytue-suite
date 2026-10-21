# Asana — SAML SSO (plan Enterprise/Enterprise+)
1. Asana Admin Console → **Settings → Authentication** → «SAML» → «Edit».
2. IdP: pega los metadatos de Sytue ID. **Name ID format** `email` (o `emailAddress`).
3. Guarda y deja «SAML + contraseña» hasta verificar; luego «SAML only».
No hay campos que copiar a Sytue Platform: Asana descubre el IdP por dominio de correo.
**Verificar**: login de un usuario del dominio desde la suite.
**Errores**: «Invalid audience» = el Entity ID del IdP no es exactamente el que Asana espera (`https://app.asana.com/`).
