<a id="saml-genérico--cualquier-proveedor-que-no-tenga-ficha-propia"></a>
# Generic SAML — any provider without its own integration page
For SAML 2.0 providers without a template (custom ERP, internal portals…):
1. In the provider: enable SAML and copy its **Entity ID** and **ACS URL**.
2. Sytue Platform → Integrations → **Generic SAML**: paste them.
3. In the provider, load the Sytue ID metadata (Platform → “IdP metadata”): Entity/Login/Certificate.
4. Name ID `email` unless the provider requires something else.
**Verify**: always in a private window and with “SAML + password” before “SAML only”.
