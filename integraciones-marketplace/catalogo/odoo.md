<a id="odoo--alta-en-sytue-platform"></a>
# Odoo — onboarding in Sytue Platform
**What it connects**: identity (Odoo's own OIDC) and data from your database via XML-RPC/JSON-RPC.
1. In Odoo: **Settings → Users & Companies → OAuth?** — for identity, a technical user with read permissions on the model you federate is enough.
2. Create the technical user (e.g. `sytue@…`) with its own 2FA and minimal permissions.
3. Sytue Platform → Integrations → **Odoo**: fill in **Base URL** (`https://odoo.tuempresa.com`), **Database** (the EXACT database name — if in doubt, `GET /web/database/list`) and the technical user's **Login**.
4. Paste the user's API key (Preferences → “API Keys”) if your Odoo asks for it instead of a password.
**Verify**: the card lists sample records from the configured model.
**Errors**: `Access Denied` = the technical user's permissions; database not found = the name does not match (it is case-sensitive).
