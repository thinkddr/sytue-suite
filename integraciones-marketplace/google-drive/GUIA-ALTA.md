# Guía de alta — Google Drive ⇄ Sytue

Tiempo: ~15 min. Permisos: acceso a Google Cloud Console de tu organización.
Resultado: los archivos nuevos de Sytue Drive se guardan en tu Google Drive.

## 1. Crear el proyecto OAuth

1. <https://console.cloud.google.com> → selector de proyectos → «New project»
   → nombre «Sytue Integration Gateway».
2. «APIs & Services» → «Library» → busca **Google Drive API** → «Enable».
3. «OAuth consent screen»: tipo **Internal** (Workspace) o **External**;
   rellena nombre de app «Sytue», correo de soporte y dominio `sytue.eu`.
   Añade el scope `.../auth/drive.file` a la lista declarada.
4. Si es External y está en «Testing», añade a los usuarios de prueba.

## 2. Crear el cliente OAuth

1. «Credentials» → «Create credentials» → «OAuth client ID» → **Web
   application**.
2. «Authorized redirect URIs» → añade EXACTAMENTE:
   `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. Crea y copia **Client ID** y **Client secret**.

## 3. Conectar desde Sytue Platform

1. Sytue Platform → Integraciones → «Google Drive».
2. Pega Client ID y Client Secret → «Conectar».
3. Google pide consentimiento con `access_type=offline` y `prompt=consent`
   (ya configurado): el gateway guarda un refresh token y la conexión queda
   activa sin re-autorizar.

## 4. Verificar y operar

- En Sytue Drive, «subir copia a Google Drive» sobre un fichero; revisa que
  aparece en la carpeta raíz del Drive del usuario conectado.
- La ficha de la integración muestra la última sincronización.
- Para revocar: Google Account → Security → Third-party access, o «Eliminar»
  en la ficha (el gateway llama al revoke_url de la semilla).

## Problemas frecuentes

- `redirect_uri_mismatch`: el URI no coincide byte a byte (sin barra final).
- `access_denied`: la app está en Testing y el usuario no es tester.
