# Microsoft 365 — setup in Sytue Platform
**What it connects**: Graph (read-only mail, files, contacts, basic Teams) via OAuth.
**Mode A — gateway application (recommended)**: in Sytue Platform choose “Shared instance” and just grant consent. Nothing to register.
**Mode B — your own app in Entra**:
1. portal.azure.com → **App registrations → New** (multi-tenant) → Web redirect `https://integrations.sytue.eu/api/v1/oauth/callback`.
2. Graph delegated permissions: `offline_access openid email profile User.Read Files.Read.All Contacts.Read Team.ReadBasic.All Channel.ReadBasic.All ChannelMember.Read.All ChannelMessage.Read.All` → **Grant admin consent**.
3. Copy the Client ID (and the secret if you require one).
4. Sytue Platform → Integrations → **Microsoft 365**: fill in **Tenant** and **Client ID** for mode B → “Connect” → “Select account”.
**Verify**: search federation finds emails and OneDrive files.
**Errors**: `AADSTS900021`/`65001` = admin consent is missing for some scope.
