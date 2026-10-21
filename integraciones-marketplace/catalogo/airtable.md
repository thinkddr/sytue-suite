# Airtable — SAML SSO (plan Enterprise)
1. Airtable admin → **Enterprise Hub → Settings → Identity & access → SSO**.
2. Añade proveedor SAML con los **metadatos de Sytue ID** (Platform → Integraciones → Airtable → «Metadatos IdP»).
3. Copia de Airtable el **Entity ID** y el **ACS URL** y péganos en la ficha de Sytue Platform.
4. Activa «Restrict to SSO» solo tras probar el login en ventana privada.
**Verificar**: un usuario asignado entra desde el lanzador del gateway.
