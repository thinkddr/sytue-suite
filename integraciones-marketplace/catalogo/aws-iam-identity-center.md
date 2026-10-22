# AWS IAM Identity Center — SAML + SCIM
1. Consola AWS → **IAM Identity Center → Settings → Add identity source → External IdP**... o más simple: deja AWS como IdP hacia Sytue.
2. Copia a la ficha de Sytue Platform: tu **region** (p. ej. `eu-south-2`), el **saml_id** (el `samlprovider/…` del proveedor creado) y el **scim_endpoint** que AWS genera.
3. Crea el **token SCIM** en AWS y pégalo en la ficha: el aprovisionamiento de usuarios/grupos va por ahí.
4. Asigna al menos un permiso set a los usuarios sincronizados.
**Verificar**: en AWS, la pestaña «Automated provisioning» queda en verde y aparecen los usuarios.
