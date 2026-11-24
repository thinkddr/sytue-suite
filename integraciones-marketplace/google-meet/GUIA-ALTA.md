<a id="guía-de-alta--google-meet--sytue"></a>
# Setup guide — Google Meet ⇄ Sytue

Time: ~12 min. Permissions: your organization's Google Cloud Console. Result:
view and open Meet meetings from the suite's calendar (read-only).

> Honest note: this connector's seed arrives with the Meet integrations wave;
> this guide uses the standard read-only scopes and the gateway callback,
> which do not change.

<a id="1-proyecto-y-consentimiento"></a>
## 1. Project and consent

1. <https://console.cloud.google.com> → project (or create “Sytue Integration
   Gateway”).
2. “APIs & Services” → “Library” → enable the **Google Meet API** and the **Google
   Calendar API**.
3. “OAuth consent screen”: declare the scopes below.

<a id="2-cliente-oauth"></a>
## 2. OAuth client

1. “Credentials” → “OAuth client ID” → **Web application**.
2. Redirect URI:
   `https://integrations.sytue.eu/api/v1/oauth/callback`.
3. Delegated scopes:
   - `https://www.googleapis.com/auth/meetings.space.readonly`
   - `https://www.googleapis.com/auth/calendar.readonly`
4. Copy the Client ID and Client secret.

<a id="3-conectar-y-verificar"></a>
## 3. Connect and verify

1. Sytue Platform → Integrations → “Google Meet” → paste the credentials →
   “Connect”.
2. Open the suite's calendar: events with a Meet conference show the
   «Unirse a Meet» (Join Meet) link.
