# Snowflake — SAML SSO
1. Snowflake (role ACCOUNTADMIN): **Admin → Identiy → Single Sign-On** (or `ALTER ACCOUNT SET SAML_IDENTITY_PROVIDER`).
2. Sytue ID metadata as the IdP; **Name ID** `email`.
3. Write down your **account** (the `orgname-account_name` identifier from the URL) on the Sytue Platform detail card.
4. Test with `sso_login_page=true` before enforcing it.
**Errors**: `user not found` = the Snowflake login does not match the Name ID email.
