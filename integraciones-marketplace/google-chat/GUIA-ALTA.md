<a id="guía-de-alta--google-chat--sytue"></a>
# Onboarding guide — Google Chat ⇄ Sytue

Time: ~15 min. Permissions: Workspace admin and access to Google Cloud Console.
Result: read Chat spaces and messages from the suite's search and
notifications (read-only).

> Honest note: catalog seed pending the Chat wave; scopes and
> callback are standard and do not change.

<a id="1-configurar-la-app-de-chat"></a>
## 1. Configure the Chat app

1. <https://console.cloud.google.com> → project → “APIs & Services” →
   enable **Google Chat API**.
2. “Chat configuration”:
   - App name “Sytue”, brand avatar, description.
   - **Connection**: “HTTP endpoint” → Integration Gateway URL
     (`https://integrations.sytue.eu/api/v1/chat` in your deployment).
   - “Verify” the `sytue.eu` domain in Search Console if Google asks for it.
3. Visibility: the users/groups in your Workspace that will be able to use it.

<a id="2-oauth-para-lectura"></a>
## 2. OAuth for reading

1. “Credentials” → “OAuth client ID” → **Web application** with redirect URI
   `https://integrations.sytue.eu/api/v1/oauth/callback`.
2. Delegated scopes:
   - `https://www.googleapis.com/auth/chat.spaces.readonly`
   - `https://www.googleapis.com/auth/chat.messages.readonly`
3. Copy the Client ID and Client secret.

<a id="3-conectar-y-verificar"></a>
## 3. Connect and verify

1. Sytue Platform → Integrations → “Google Chat” → credentials → “Connect”.
2. The user's spaces appear in the suite's search federation.
