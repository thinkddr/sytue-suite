# Odoo — alta en Sytue Platform
**Qué conecta**: identidad (OIDC del propio Odoo) y datos de tu base por XML-RPC/JSON-RPC.
1. En Odoo: **Ajustes → Usuarios y compañías → OAuth?** — para la identidad basta un usuario técnico con permisos de lectura del modelo que federas.
2. Crea el usuario técnico (p. ej. `sytue@…`) con 2FA aparte y permisos mínimos.
3. Sytue Platform → Integraciones → **Odoo**: rellena **Base URL** (`https://odoo.tuempresa.com`), **Database** (el nombre EXACTO de la base — si dudas, `GET /web/database/list`) y **Login** del usuario técnico.
4. Pega la API key del usuario (Preferencias → «API Keys») si tu Odoo la pide en vez de contraseña.
**Verificar**: la ficha lista registros de ejemplo del modelo configurado.
**Errores**: `Access Denied` = permisos del usuario técnico; base no encontrada = el nombre no coincide (es sensible a mayúsculas).
