# Notion — SAML SSO (plan Enterprise) + workspace
1. Notion **Settings → Identity → SAML SSO**: sube metadatos de Sytue ID, Name ID `email`.
2. Anota el **workspace** (el nombre EXACTO del espacio) en la ficha de Sytue Platform: Notion enruta por él antes del SSO.
3. «Require SAML» por dominio verificado solo tras verificar.
**Errores**: «workspace not found» al entrar = el nombre del workspace en la ficha no coincide.
