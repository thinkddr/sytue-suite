# Guía de alta — Microsoft Teams ⇄ Sytue

Tiempo: ~15 min. Permisos: Teams Administrator (o Global Admin) del tenant de
Entra. Resultado: app «Sytue» en Teams con panel de la suite.

## 1. Preparar el paquete

El zip de la release ya trae `manifest.json`, `color.png` y `outline.png`.
Si lo generas tú: `bash scripts/empaquetar-marketplace.sh`.

## 2. Fijar el ID de la app (una sola vez)

1. Genera un GUID (`uuidgen`).
2. Sustituye en `manifest.json` el valor `"id": "00000000-…"` por tu GUID.
   Este ID es el que Teams usa como identidad de la app; no lo cambies más.

## 3. Subir a Teams

- **Centro de administración**: <https://admin.teams.com> → «Teams apps» →
  «Manage apps» → «Upload new app» → sube el zip → «Publish».
- Con directivas de apps personalizadas, añade además la app a las políticas
  de permisos de tu organización.

## 4. OAuth de la conexión (Integration Gateway)

1. Entra en <https://portal.azure.com> → «Microsoft Entra ID» → «App
   registrations» → «New registration».
2. Nombre «Sytue Integration Gateway», cuentas «Any organizational directory».
3. «Add a Redirect URI» (Web):
   `https://integrations.sytue.eu/api/v1/oauth/callback`.
4. Copia **Application (client) ID** y crea un **Client secret**.
5. Permisos delegados (Microsoft Graph): `User.Read`, `Team.ReadBasic.All`,
   `Channel.ReadBasic.All`, `ChannelMessage.Read.All` → «Grant admin consent».

## 5. Conectar desde Sytue Platform

1. Sytue Platform → Integraciones → «Microsoft Teams».
2. Rellena **Tenant** (contoso.onmicrosoft.com o el GUID), **Client ID** y
   **Client Secret** del paso 4. «Conectar» y autoriza.

## 6. Verificar

- Teams → «Apps» → «Built for your org» → «Sytue» → la pestaña abre el panel
  del gateway con tu sesión.
- En Sytue Platform la ficha pasa a «activa».

## Problemas frecuentes

- «App not allowed»: la política de permisos de tu tenant bloquea apps de
  terceros/subidas — asígnale política en «Permission policies».
- `AADSTS50011`: el redirect URI registrado no coincide byte a byte.
