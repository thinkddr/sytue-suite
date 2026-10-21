# Google Workspace — SAML + OAuth ( Drive/Gmail/Calendar/Contactos de solo lectura)
**A. SSO SAML** (opcional, para identidad unificada):
1. admin.google.com → **Security → Authentication → Set up SSO with a third-party IdP**; sube certificado y URLs de Sytue ID.
**B. Datos por OAuth** (obligatorio para la federación):
1. console.cloud.google.com → proyecto → habilita Drive, Gmail y Calendar API.
2. OAuth consent screen **Internal** + scopes de la semilla (todos readonly).
3. Crea una **cuenta de servicio**, dale **Domain-wide delegation** en Workspace (Security → API Controls → Domain-wide Delegation) con los mismos scopes, y anota el **admin_email** que se impersona.
4. Sytue Platform → Integraciones → **Google Workspace**: **Domain** y **admin_email** + el JSON de la cuenta de servicio → «Conectar».
**Verificar**: federación encuentra un correo y un evento de calendario reales.
**Errores**: `unauthorized_client` = el scope de delegación no coincide con el consentido.
