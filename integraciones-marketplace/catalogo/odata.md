# OData genérico — alta en Sytue Platform
**Qué conecta**: cualquier endpoint OData v4 (ERP/CRM propios, Dynamics on-prem, SAP gateways…).
1. Necesitas la **service_url** del servicio: termina en `/odata` o `/$metadata` y responde 200 en un navegador autenticado.
2. Sytue Platform → Integraciones → **OData**: pega la service_url.
3. Autenticación según exponga el servicio: token pegado (Bearer) o credenciales del propio servicio — el formulario lo pide.
**Verificar**: el panel lista los EntitySets del `$metadata`.
**Errores**: `406` = el servicio no negocia JSON (`application/json;odata.metadata=minimal`): revisa la versión v4.
