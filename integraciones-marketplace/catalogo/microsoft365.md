# Microsoft 365 — alta en Sytue Platform
**Qué conecta**: Graph (correo de solo lectura, ficheros, contactos, Teams básico) por OAuth.
**Modo A — aplicación del gateway (recomendado)**: en Sytue Platform elige «Instancia compartida» y solo consiente. Nada que registrar.
**Modo B — tu propia app en Entra**:
1. portal.azure.com → **App registrations → New** (multi-tenant) → redirect Web `https://integrations.sytue.eu/api/v1/oauth/callback`.
2. Permisos delegados de Graph: `offline_access openid email profile User.Read Files.Read.All Contacts.Read Team.ReadBasic.All Channel.ReadBasic.All ChannelMember.Read.All ChannelMessage.Read.All` → **Grant admin consent**.
3. Copia Client ID (y secreto si lo exiges).
4. Sytue Platform → Integraciones → **Microsoft 365**: rellena **Tenant** y **Client ID** del modo B → «Conectar» → «Seleccionar cuenta».
**Verificar**: federación de búsqueda encuentra correos y ficheros de OneDrive.
**Errores**: `AADSTS900021`/`65001` = falta consent del admin para algún scope.
