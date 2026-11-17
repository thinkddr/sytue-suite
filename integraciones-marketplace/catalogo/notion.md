# Notion — SAML SSO (Enterprise plan) + workspace
1. Notion **Settings → Identity → SAML SSO**: upload the Sytue ID metadata, Name ID `email`.
2. Write down the **workspace** (the EXACT name of the workspace) on the Sytue Platform detail card: Notion routes by it before SSO.
3. “Require SAML” per verified domain only after verifying.
**Errors**: “workspace not found” when signing in = the workspace name on the detail card does not match.
