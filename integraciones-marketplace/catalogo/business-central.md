# Business Central — alta en Sytue Platform
**Qué conecta**: lectura/escritura financiera (Financials.ReadWrite.All) contra UNA empresa.
1. portal.azure.com → Entra → **App registrations → New** (multi-tenant) → redirect URI Web: `https://integrations.sytue.eu/api/v1/oauth/callback`.
2. **API permissions** → **Dynamics 365 Business Central** → delegated `Financials.ReadWrite.All` + `offline_access` → consent del admin.
3. Copia Client ID + secreto.
4. En BC: busca **«Microsoft Entra Applications»** → nueva → añade el Client ID con el permiso **D365 BUS FULL ACCESS** (o el conjunto mínimo que quieras).
5. Obtén la **service_url**: en la empresa, «Help → Debug» o la página 810: termina en `/api/v2.0/companies(<id>)`.
6. Sytue Platform → Integraciones → **Business Central**: tenant, client_id, secreto y service_url → «Conectar».
**Verificar**: prueba de lectura de la empresa en la ficha de la integración.
**Errores**: `401` desde BC = falta el registro en «Entra Applications»; 403 = permiso D365 insuficiente.
