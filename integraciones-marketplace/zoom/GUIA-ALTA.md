<a id="guía-de-alta--zoom--sytue"></a>
# Setup guide — Zoom ⇄ Sytue

Time: ~20 min. Permissions: Zoom account admin (vanity URL). Result:
SAML single sign-on and OAuth access to meetings from the suite.

<a id="a-saml-sso-identidad"></a>
## A. SAML SSO (identity)

1. Go to <https://zoom.us> as an admin → “Advanced” → “Security” →
   “Sign In Method” → enable “Pre-authenticate my domain” if applicable.
2. “Advanced” → “Single Sign-On” → “Edit”/“Add Configuration”.
3. Fill in the data from your Sytue ID instance:
   - **Sign-in page URL** and **Entity ID**: the Sytue ID SAML metadata
     (Sytue Platform → Integrations → Zoom → “Download metadata”).
   - **Name ID format**: `email`.
   - **ACS URL** (what Sytue ID must know as the destination):
     `https://{vanity}.zoom.us/saml/SSO` — replace `{vanity}` with yours.
4. Download Zoom's signing certificate and load it into Sytue ID.
5. Test SSO in a private window before enabling it for everyone.

<a id="b-oauth-para-reuniones"></a>
## B. OAuth for meetings

1. <https://marketplace.zoom.us> → “Develop” → “Build App” → **OAuth**.
2. Name “Sytue”, type “User-managed” (or “Account-level” if your plan
   allows it).
3. “Redirect URL for OAuth” and “Allow list”:
   `https://integrations.sytue.eu/api/v1/oauth/callback`.
4. Scopes (meeting read/write only, nothing else):
   - `meeting:read`
   - `meeting:write`
5. Copy the **Client ID** and **Client Secret**.

<a id="c-conectar-desde-sytue-platform"></a>
## C. Connect from Sytue Platform

1. Sytue Platform → Integrations → “Zoom”.
2. Fill in the **Vanity URL** (the one in `https://VANITY.zoom.us`), Client ID and
   Client Secret → “Connect” and authorize.

<a id="problemas-frecuentes"></a>
## Common problems

- “This app is not available for your account”: the vanity in the field and the one of the
  account do not match.
- SSO does not redirect: the Sytue ID ACS does not use exactly
  `https://{vanity}.zoom.us/saml/SSO`.
