# SAML genérico — cualquier proveedor que no tenga ficha propia
Para proveedores SAML 2.0 sin plantilla (ERP a medida, portales internos…):
1. En el proveedor: activa SAML y copia su **Entity ID** y **ACS URL**.
2. Sytue Platform → Integraciones → **SAML genérico**: pégalos.
3. En el proveedor, carga los metadatos de Sytue ID (Platform → «Metadatos IdP»): Entity/Login/Certificado.
4. Name ID `email` salvo que el proveedor exija otra cosa.
**Verificar**: siempre en ventana privada y con «SAML + contraseña» antes de «SAML only».
