<a id="mi-cuenta"></a>

# My account

The user's own profile, security, devices and access. Astro 7 + Preact,
SSR for the BFF, on top of the Sytue ID API.

<a id="por-qué-es-una-app-y-no-una-página-de-sytue-id"></a>

## Why it is an app and not a Sytue ID page

This screen used to be served as HTML from Go, without a bundler, so it could not mount
the Sytue Shell: you landed on it and there was no app switcher and no way out
to the rest of the suite.

Replicating the switcher in Go meant duplicating the product catalog
(names, icons, colors of fourteen products) in a second language. That duplicate
drifts with the first new application, so the page was moved here.

The Sytue ID flows that are **not** the account (login, OIDC, SAML) remain in Go.
They are screens without navigation by design and gain nothing from this.

<a id="la-api-ya-existía"></a>

## The API already existed

Sytue ID has published `/api/v1/me*` for a long time, with its integration tests, and
until now it **had no consumer in `apps/`**: the browser used the HTML routes
and the JSON was left for native clients. Its own code says so.

So this app does not invent a contract: it consumes the one that already exists.

| Section       | API                                                                         |
| ------------- | --------------------------------------------------------------------------- |
| Profile       | `GET /me`, `PATCH /me` (the seven fields, partial patching)                 |
| Password      | `POST /me/password` (closes ALL sessions)                                   |
| Second factor | `POST /me/mfa/totp/{enroll,confirm,recovery-codes}`, `DELETE /me/mfa/totp`  |
| Sessions      | `GET /me/sessions`, `DELETE /me/sessions/{id}`, `DELETE /me/sessions`       |
| Devices       | `GET /me/devices`, `POST /me/devices/{id}/trust`, `DELETE /me/devices/{id}` |

| Passkeys | `GET /webauthn/credentials`, `DELETE /webauthn/credentials/{id}` |
| App passwords | `GET`, `POST /me/app-passwords`, `DELETE /me/app-passwords/{id}` |

Passkey **registration** is not just another call: `navigator.credentials.create()`
opens a browser dialog that talks to the authenticator (Touch ID, a USB
key, the phone), so that part lives in `src/lib/passkeys.ts` and the BFF only
sets the challenge beforehand and verifies the attestation afterwards.

The Apple configuration profile (`.mobileconfig`) is a **link** to Sytue ID
and not a proxy: Sytue ID serves it requiring its session COOKIE, not a bearer, and whoever is
on this screen has an SSO session there by definition. On a top-level
navigation the `SameSite=Lax` cookie is sent, so the link downloads. Proxying it
would have meant duplicating the plist generator.

<a id="direcciones"></a>

## Addresses

The canonical one is the organization's, `https://<slug>.<dominio>/account`; its own
subdomain serves the same prefix. OIDC client `sytue-account` (migration 0060).

**`id.<dominio>/account` redirects here**, to the organization host, which is the
suite's canonical address: same cookies, same language and same app
switcher. It only falls back to the `account.<dominio>` subdomain when the session does not
have an organization yet.

It is switched on by the chart's `global.accountAppPath`, and **leaving it empty switches it off**: an
installation that does not deploy this surface and received the 302 would leave its
people without an account screen (and without a way to change their password). That is why it is
a value and not a constant, and why the Go HTML is still standing: it is what
answers where the app is not deployed, and the only path for anyone who has that tab
open. Removing it is a separate change, once the app is already being served.

<a id="estado"></a>

## Status

- [x] Scaffold with the Shell and the app switcher
- [x] Full profile: name, given name, surnames, job title, department,
      location and language (email read-only)
- [x] Sessions (close one, or all except the current one)
- [x] Devices (forget any, mark the current one as trusted)
- [x] Registered in the chart and in the pilot deployment
- [x] Password and second factor (TOTP, recovery codes)
- [x] Passkeys: list and remove (registration, with the WebAuthn dialog, is separate)
- [x] App passwords (create, list and revoke)
- [x] Apple configuration profile (link to the IdP)
- [x] Passkey registration (WebAuthn dialog in the browser)
- [x] Redirect from `id.<dominio>/account` (`global.accountAppPath`)
- [ ] Removal of the Go HTML, once the app has been serving for a while
