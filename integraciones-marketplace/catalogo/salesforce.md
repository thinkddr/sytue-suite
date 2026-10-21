# Salesforce — alta en Sytue Platform
**Qué conecta**: SAML SSO + API REST (`api`, `refresh_token`) de tu org por su My Domain.
1. Salesforce Setup → **Company Settings → My Domain**: copia tu **subdominio** (`https://SUBDOMINIO.my.salesforce.com`).
2. **OAuth**: Setup → **App Manager → New Connected App** («Create a Connected App»):
   - Enable OAuth: callback `https://integrations.sytue.eu/api/v1/oauth/callback`,
   - Scopes: `api`, `refresh_token`, `offline_access`,
   - «Enable OAuth Settings» + relaja «Require secret for Web Server Flow» según tu política.
3. Copia **Consumer Key/Secret**.
4. **SAML** (si federas identidad): Setup → Single Sign-On → usa los metadatos de Sytue ID; Entity ID/ACS de Salesforce van en la ficha de Sytue Platform.
5. Sytue Platform → Integraciones → **Salesforce**: subdominio + credenciales OAuth → «Conectar».
**Verificar**: SOQL de prueba desde el panel; login SSO desde una ventana privada.
**Errores**: `invalid_grant` = IP no en «Network Access» (añade las IPs de salida del gateway).
