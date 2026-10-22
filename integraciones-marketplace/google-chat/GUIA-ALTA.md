# Guía de alta — Google Chat ⇄ Sytue

Tiempo: ~15 min. Permisos: admin de Workspace y acceso a Google Cloud Console.
Resultado: leer espacios y mensajes de Chat desde la búsqueda y avisos de la
suite (solo lectura).

> Nota honesta: semilla del catálogo pendiente de la ola de Chat; scopes y
> callback estándar, no cambian.

## 1. Configurar la app de Chat

1. <https://console.cloud.google.com> → proyecto → «APIs & Services» →
   habilita **Google Chat API**.
2. «Chat configuration»:
   - App name «Sytue», avatar de la marca, descripción.
   - **Connection**: «HTTP endpoint» → URL del Integration Gateway
     (`https://integrations.sytue.eu/api/v1/chat` en tu despliegue).
   - «Verify» el dominio `sytue.eu` en Search Console si Google lo pide.
3. Visibility: los usuarios/grupos de tu Workspace que podrán usarla.

## 2. OAuth para lectura

1. «Credentials» → «OAuth client ID» → **Web application** con redirect URI
   `https://integrations.sytue.eu/api/v1/oauth/callback`.
2. Scopes delegados:
   - `https://www.googleapis.com/auth/chat.spaces.readonly`
   - `https://www.googleapis.com/auth/chat.messages.readonly`
3. Copia Client ID y Client secret.

## 3. Conectar y verificar

1. Sytue Platform → Integraciones → «Google Chat» → credenciales → «Conectar».
2. Los espacios del usuario aparecen en la federación de búsqueda de la suite.
