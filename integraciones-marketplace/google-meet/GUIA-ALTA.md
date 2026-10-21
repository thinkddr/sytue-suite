# Guía de alta — Google Meet ⇄ Sytue

Tiempo: ~12 min. Permisos: Google Cloud Console de tu organización. Resultado:
ver y abrir reuniones de Meet desde la agenda de la suite (solo lectura).

> Nota honesta: la semilla de este conector llega con la ola de integraciones
> de Meet; esta guía usa los scopes estándar de solo lectura y el callback del
> gateway, que no cambian.

## 1. Proyecto y consentimiento

1. <https://console.cloud.google.com> → proyecto (o crea «Sytue Integration
   Gateway»).
2. «APIs & Services» → «Library» → habilita **Google Meet API** y **Google
   Calendar API**.
3. «OAuth consent screen»: declara los scopes de abajo.

## 2. Cliente OAuth

1. «Credentials» → «OAuth client ID» → **Web application**.
2. Redirect URI:
   `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. Scopes delegados:
   - `https://www.googleapis.com/auth/meetings.space.readonly`
   - `https://www.googleapis.com/auth/calendar.readonly`
4. Copia Client ID y Client secret.

## 3. Conectar y verificar

1. Sytue Platform → Integraciones → «Google Meet» → pega las credenciales →
   «Conectar».
2. Abre la agenda de la suite: los eventos con conferencia de Meet muestran el
   enlace «Unirse a Meet».
