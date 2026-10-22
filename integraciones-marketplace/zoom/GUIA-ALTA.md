# Guía de alta — Zoom ⇄ Sytue

Tiempo: ~20 min. Permisos: admin de la cuenta Zoom (vanity URL). Resultado:
inicio de sesión único SAML y acceso OAuth a reuniones desde la suite.

## A. SAML SSO (identidad)

1. Entra en <https://zoom.us> como admin → «Advanced» → «Security» →
   «Sign In Method» → activa «Pre-authenticate my domain» si procede.
2. «Advanced» → «Single Sign-On» → «Edit»/«Add Configuration».
3. Rellena con los datos de tu instancia de Sytue ID:
   - **Sign-in page URL** y **Entity ID**: los metadatos SAML de Sytue ID
     (Sytue Platform → Integraciones → Zoom → «Descargar metadatos»).
   - **Name ID format**: `email`.
   - **ACS URL** (lo que Sytue ID debe conocer como destino):
     `https://{vanity}.zoom.us/saml/SSO` — sustituye `{vanity}` por el tuyo.
4. Descarga el certificado de firma de Zoom y cárgalo en Sytue ID.
5. Prueba el SSO en una ventana privada antes de activarlo para todos.

## B. OAuth para reuniones

1. <https://marketplace.zoom.us> → «Develop» → «Build App» → **OAuth**.
2. Nombre «Sytue», tipo «User-managed» (o «Account-level» si tu plan lo
   permite).
3. «Redirect URL for OAuth» y «Allow list»:
   `https://integrations.sytue.eu/api/v1/oauth/callback`.
4. Scopes (solo lectura/escritura de reuniones, nada más):
   - `meeting:read`
   - `meeting:write`
5. Copia **Client ID** y **Client Secret**.

## C. Conectar desde Sytue Platform

1. Sytue Platform → Integraciones → «Zoom».
2. Rellena **Vanity URL** (el de `https://VANITY.zoom.us`), Client ID y
   Client Secret → «Conectar» y autoriza.

## Problemas frecuentes

- «This app is not available for your account»: el vanity del campo y el de la
  cuenta no coinciden.
- El SSO no redirige: el ACS de Sytue ID no usa exactamente
  `https://{vanity}.zoom.us/saml/SSO`.
