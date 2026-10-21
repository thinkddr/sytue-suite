# Snowflake — SAML SSO
1. Snowflake (role ACCOUNTADMIN): **Admin → Identiy → Single Sign-On** (o `ALTER ACCOUNT SET SAML_IDENTITY_PROVIDER`).
2. Metadatos de Sytue ID como IdP; **Name ID** `email`.
3. Anota tu **account** (el identificador `orgname-account_name` de la URL) en la ficha de Sytue Platform.
4. Prueba con `sso_login_page=true` antes de forzarlo.
**Errores**: `user not found` = el login de Snowflake no coincide con el correo del Name ID.
