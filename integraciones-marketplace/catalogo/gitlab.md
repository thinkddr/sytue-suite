# GitLab (SaaS) — SAML SSO por GRUPO (plan Premium+)
1. Tu grupo en gitlab.com → **Settings → SAML SSO for GitLab.com Groups**.
2. Pega los metadatos de Sytue ID en «IdP URL»/certificado; **Name ID** `email`.
3. Copia el **GitLab group SAML URL** (ACS) y el **Identifier** (Entity ID) del grupo a la ficha de Sytue Platform.
4. «Test SAML» y asigna membresías por defecto.
**Errores**: `audience mismatch` = el Identifier no coincide exactamente con el Entity ID emitido.
