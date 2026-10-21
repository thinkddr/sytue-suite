# Guía de alta — Slack ⇄ Sytue

Tiempo: ~10 min. Permisos: admin del workspace. Resultado: Slack conectado con
OAuth (canales, avisos entrantes) y SAML/SCIM si tu plan los soporta.

## 1. Crear la app de Slack

1. Entra en <https://api.slack.com/apps> con tu cuenta de Slack.
2. «Create New App» → «From an app manifest» → elige tu workspace.
3. Pega el contenido de `app-manifest.yml` (este mismo zip) → «Next» → «Create».

El manifiesto ya declara scopes, la página de instalación y el redirect URI del
Integration Gateway (`https://integrations.sytue.eu/api/v1/oauth/callback`).

## 2. Copiar las credenciales

1. «Basic Information» → «App Credentials».
2. Copia **Client ID** y **Client Secret**.
3. En «OAuth & Permissions», verifica que «Redirect URLs» contiene exactamente
   el URI del paso 1 (el manifiesto lo rellena; si Slack lo marca pendiente,
   «Add» y «Save URLs»).

## 3. Conectar desde Sytue Platform

1. Entra en Sytue Platform → Integraciones → «Slack».
2. Rellena el campo **Workspace** (el de `WORKSPACE.slack.com`) y los dos
   valores del paso 2.
3. «Conectar» te lleva a Slack: revisa los permisos y autoriza.

## 4. (Opcional) SAML SSO y SCIM

- **SAML**: en Slack, «Settings and Permissions» → «Sign in with Slack» /
  SAML: carga los metadatos de Sytue ID y usa ACS
  `https://WORKSPACE.slack.com/sso/saml`, NameID `email`.
- **SCIM**: genera un token de API de SCIM en Slack y pégalo en la ficha de la
  integración en Sytue Platform; el aprovisionamiento de usuarios y grupos va
  por `https://api.slack.com/scim/v2`.

## 5. Verificar

1. «OAuth & Permissions» → «Install to Workspace» si Slack lo pide.
2. En Sytue Platform, la ficha de Slack pasa a «activa» con la última sync.
3. Envía un aviso de prueba desde la suite; debe aparecer en el canal elegido.

## Problemas frecuentes

- `redirect_uri_mismatch`: el Redirect URL de Slack y el de la ficha no son
  byte a byte iguales (sin barra final).
- La app no aparece en «From an app manifest»: el workspace elegido no es el
  correcto; vuelve al paso 1.
