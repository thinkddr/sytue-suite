# Guía de alta — Microsoft OneDrive ⇄ Sytue

Tiempo: ~15 min. Permisos: Application Administrator en Entra. Resultado: las
copias de Sytue Drive se guardan en el OneDrive del usuario conectado.

## 1. Registrar la aplicación en Entra ID

1. <https://portal.azure.com> → «Microsoft Entra ID» → «App registrations» →
   «New registration».
2. Nombre «Sytue Integration Gateway».
3. Tipos de cuenta: «Accounts in any organizational directory (Multitenant)»
   — es lo que la semilla espera para funcionar con cualquier tenant.
4. Redirect URI (plataforma **Web**):
   `https://integrations.sytue.eu/api/v1/oauth/callback`.

## 2. Credenciales y permisos

1. «Certificates & secrets» → «New client secret» → copia el VALOR ahora
   (después no se ve).
2. «API permissions» → «Microsoft Graph» → «Delegated permissions»:
   - `Files.ReadWrite`
   - `offline_access`
3. «Grant admin consent» si tu tenant lo exige.

## 3. Conectar desde Sytue Platform

1. Sytue Platform → Integraciones → «Microsoft OneDrive».
2. Pega **Application (client) ID** y el secreto → «Conectar».
3. Autoriza con la cuenta de Microsoft que tiene el OneDrive destino.

## 4. Verificar y operar

- Sube una copia desde Sytue Drive y comprueba `Archivos → Sytue` en OneDrive.
- `AADSTS7000215` en la conexión: pegaste el «Secret ID» y no el «Value».

## Nota de seguridad

El gateway pide solo permisos delegados del usuario que conecta —no
`Files.ReadWrite.All` de aplicación—, así que el alcance queda limitado a lo
que ese usuario puede tocar en su propio OneDrive.
