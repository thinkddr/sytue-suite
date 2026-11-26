<a id="sytue-rooms--app-movil-capacitor-8"></a>
# Sytue Rooms — Mobile app (Capacitor 8)

**Native** shell of **Sytue Rooms** for **iOS and Android**. It **wraps the Rooms
PWA** (`apps/frontend/apps/rooms`, served at `https://rooms.sytue.eu`) in a native
webview with [Capacitor](https://capacitorjs.com) (**MIT**) and adds mobile
integration: push, deep links, status bar, splash, keyboard, unread badge and the Android
back button. It does **not** reimplement the UI — it is a branded client on top of the same PWA, just
like the Rooms desktop app in Wails (retired on 2026-09-23) and like the
Mattermost/Slack desktop app.

> **Location.** `apps/mobile/` already contains (earlier commit, another session) the
> Capacitor shell of **Sytue One** (the full suite, `one.sytue.eu`, ADR-0024). This app
> is a different product — the **Rooms** client (`rooms.sytue.eu`) — so it lives
> in `apps/mobile/rooms/` in order **not** to destroy that work and to respect “separate
> module, without touching what is shared”. It was the exact mirror of the Rooms desktop
> app in Wails, now retired. If the goal were to _replace_ the Sytue
> One shell with the Rooms one, move it to the root of `apps/mobile/`.

Standalone **on purpose**: its own `package.json` with the Capacitor CLI + plugins,
**outside** the frontend's Bun workspace (`apps/frontend`). `node_modules/`, `ios/` and
`android/` are in `.gitignore` (they are generated).

<a id="integracion-nativa"></a>
## Native integration

| Feature                   | Plugin (MIT)                    | Implementation                                                                                                                                                                                                                                                                                                                    |
| ------------------------- | ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Push**                  | `@capacitor/push-notifications` | Requests permission, registers the native token (**FCM** Android / **APNs** iOS) and **POSTs** it to the backend (`POST {origen}/api/push/native`, session cookie). Tapping the notification → deep link to the channel (`data.url`). In the foreground it shows a banner (`presentationOptions`). **Backend FCM/APNs sender: PENDING** (see below). |
| **Deep links**            | `@capacitor/app` (`appUrlOpen`) | `sytue-rooms://open?url=…` and universal links `https://rooms.sytue.eu/channels/x` → navigates the webview to the route within the Rooms origin.                                                                                                                                                                                  |
| **Status bar**            | `@capacitor/status-bar`         | Graphite `#22262b` + light text (Graphite & Brass tokens), without overlapping the content.                                                                                                                                                                                                                                       |
| **Splash**                | `@capacitor/splash-screen`      | Brand on ivory/graphite; `launchAutoHide:false` → `native.js` hides it once Rooms has loaded (no white flash).                                                                                                                                                                                                                    |
| **Keyboard**              | `@capacitor/keyboard`           | `resize: 'native'` → the chat composer moves up with the keyboard.                                                                                                                                                                                                                                                                |
| **Unread badge**          | `@capawesome/capacitor-badge`   | Unread count on the app icon; cleared when returning to the foreground. iOS also receives the badge from the APNs payload.                                                                                                                                                                                                        |
| **Back (Android)**        | `@capacitor/app` (`backButton`) | Navigates the webview history; if it cannot, exits the app.                                                                                                                                                                                                                                                                       |
| **Webview↔native bridge** | `www/native.js`                 | Reads unread counts from the title (Rooms prefixes `(N)`) and exposes `window.SytueNative` so the PWA can push unread counts/focus.                                                                                                                                                                                               |

<a id="el-puente-webview--nativo-wwwnativejs"></a>
### The webview ↔ native bridge (`www/native.js`)

All the wiring runs **inside the webview**, on top of the Rooms page, from
`www/native.js`. It talks to the **global `window.Capacitor` bridge** (which Capacitor
injects **even into a remote origin**) by calling `window.Capacitor.Plugins.*`.
**Security:** it does not expose FS/exec/dangerous APIs — it only reads unread counts from the title and
**navigates within the Rooms origin** (deep links to other hosts are ignored; external
links already open in the system browser through `server.allowNavigation`).

**How the PWA detects that it is running in Capacitor:** `window.Capacitor?.isNativePlatform?.()`
is `true`; `window.Capacitor.getPlatform()` returns `'ios'` | `'android'`. In addition, once
`native.js` has loaded, the PWA can use `window.SytueNative`:

```js
if (window.SytueNative?.isNative) {
  window.SytueNative.setUnread(unreadCount); // pinta el badge nativo
  // window.SytueNative.openChannel('sytue-rooms://open?url=https%3A%2F%2Frooms.sytue.eu%2Fchannels%2Fx')
}
```

**How `native.js` reaches the remote PWA** (two routes, not mutually exclusive):

1. **Native injection (without touching the PWA — the default route).** Since Rooms is a
   remote origin, the native layer injects the script on every load (idempotent thanks to the
   `__sytueRoomsNative` guard), just as the desktop app injects its bridge.
   **These snippets are applied after `npx cap add`** (they need Xcode/Android Studio):

   - **iOS** — `ios/App/App/AppDelegate.swift`, in `application(_:didFinishLaunchingWithOptions:)`:

     ```swift
     if let url = Bundle.main.url(forResource: "native", withExtension: "js",
                                  subdirectory: "public"),
        let js = try? String(contentsOf: url, encoding: .utf8) {
       let script = WKUserScript(source: js, injectionTime: .atDocumentEnd,
                                 forMainFrameOnly: true)
       // El WKWebViewConfiguration de Capacitor: anade el userScript al userContentController.
       // (En Capacitor 8, accede al webView del bridge y a su configuration.userContentController.)
       CAPBridgeViewController.userScripts.append(script) // o via el config del webview
     }
     ```

     `www/` is copied to `public/` inside the bundle on `cap sync`, so
     `native.js` is available as a resource.

   - **Android** — `android/app/src/main/java/.../MainActivity.java`, hook
     `WebViewClient.onPageFinished` (or a `bridge.eval(...)`) and evaluate the asset:
     ```java
     String js = new String(getAssets().open("public/native.js").readAllBytes(),
                            StandardCharsets.UTF_8);
     bridge.getWebView().post(() -> bridge.getWebView().evaluateJavascript(js, null));
     ```

2. **PWA cooperation (optional, follow-up in `apps/frontend/apps/chat`).** The PWA
   detects Capacitor and loads/imports the client itself — without native injection. It is a
   change in the frontend repo (another session), **out of scope for this wave**.

> `native.js` is plain, **defensive** JS (every plugin is guarded; it never throws), so
> the same route 1 or 2 works without a bundler and without coupling the PWA to Capacitor.

<a id="configuracion-de-la-url-de-rooms"></a>
## Rooms URL configuration

Same `SYTUE_ROOMS_URL` convention that the Rooms desktop app (Wails) had:

| Source                               | Value                    |
| ------------------------------------ | ------------------------ |
| `SYTUE_ROOMS_URL` (env, takes priority) | any http(s) URL       |
| Default                              | `https://rooms.sytue.eu` |

```bash
SYTUE_ROOMS_URL=https://rooms.sytue.localhost npx cap run ios   # dev (Traefik + mkcert)
```

`server.url` in `capacitor.config.ts` is the URL of the Rooms PWA;
`server.allowNavigation` is a **closed list of two hosts**: the Rooms origin and the
**Sytue ID** origin, derived from the same apex (`rooms.sytue.eu` → `id.sytue.eu`,
`rooms.sytue.localhost` → `id.sytue.localhost`). Any other link opens in the
system browser, without carrying the session along. In dev, install the mkcert CA
(`mkcert -install`) so the webview trusts the local TLS.

**Why `id.*` has to be there.** Rooms sends a visitor without a session to `/auth/login`, and
from there to `id.*/oauth/authorize` (OIDC + PKCE). Capacitor applies `allowNavigation` to
**server redirects** too (iOS in `WebViewDelegationHandler.decidePolicyFor`,
Android in `shouldOverrideUrlLoading`), not just to clicks: with `id.*` off the list,
the webview **cancels** the navigation and opens it in the system browser. The user
authenticates there, the cookie stays there, and the app starts the same loop again on every
launch. In other words: **without that host login is impossible**, not “inconvenient”.

<a id="requisitos"></a>
## Requirements

- **Node** (LTS) or **Bun** — for the Capacitor CLI.
- **iOS:** **macOS + Xcode** (+ CocoaPods) and an Apple Developer account to sign/publish.
- **Android:** **Android Studio** + Android SDK (JDK 17), with `ANDROID_HOME` set.
- **ImageMagick** (`magick`) only if you regenerate icons/splash (`scripts/gen-icons.sh`).

<a id="comandos"></a>
## Commands

From `apps/mobile/rooms/`:

```bash
npm install                 # @capacitor/* + plugins (gitignored)
npx cap --version           # comprueba la CLI
npm run check               # node --check native.js + tsc --noEmit + self-check

# Anadir las plataformas nativas (genera ios/ y android/ — requieren Xcode/SDK):
npx cap add ios
npx cap add android

# Iconos y splash de marca (fuentes en resources/):
bash scripts/gen-icons.sh   # regenera resources/{icon,splash,splash-dark}.png
npm run assets              # @capacitor/assets -> assets por plataforma

npx cap sync                # copia web (www/ -> public/) + config a los proyectos nativos
npx cap open ios            # abre Xcode          (compilar/ejecutar/firmar alli)
npx cap open android        # abre Android Studio
npx cap run ios | android   # build + deploy a simulador/dispositivo
```

After `cap add`, apply **once**:

- The `native/` fragments to `Info.plist` / `AndroidManifest.xml`.
- The `native.js` injection snippets (§Bridge, route 1).
- The per-platform push config (§Push).

<a id="push--configuracion-y-estado-del-backend"></a>
## Push — configuration and backend status

**Client: DONE** (`www/native.js`). It registers the native token and POSTs it:

```
POST {origen-de-Rooms}/api/push/native      (cookie de sesion, credentials:include)
Body: { "platform": "ios" | "android", "token": "<FCM/APNs token>" }
```

A **new endpoint** was chosen for native tokens because the existing Web Push one
(`/api/push`, Wave 18) expects an **RFC 8291 subscription** (`endpoint` + `p256dh`/`auth`
keys), a shape incompatible with a plain FCM/APNs token.

**Backend — PENDING (`services/**` is not touched in this wave):**

- **Endpoint** `POST /me/push/native` in `services/chat` (BFF `/api/push/native`) that
  persists `{subject, platform, token}` (analogous to `domain.PushSubscription`, with
  `token` instead of `endpoint`/keys).
- **Sender** that delivers via **FCM HTTP v1** (Android) and **APNs** (iOS), including
  `data.url` (channel/message route) for the deep link when the notification is tapped.
- **Required credentials** (secrets, **outside git**):
  - **FCM:** Firebase service account (`google-services.json` in the app +
    service account JSON in the backend), or a legacy server key.
  - **APNs:** `.p8` key (Key ID + Team ID) or `.p12` certificate, and the `bundle id`
    `eu.sytue.rooms`; `aps-environment` entitlement (development/production).

Push config in the native project (after `cap add`):

- **Android:** place `google-services.json` in `android/app/` (gitignored). The
  plugin wires up FCM on `cap sync`.
- **iOS:** in Xcode, the **Push Notifications** capability (+ **Background Modes → Remote
  notifications**); upload the APNs `.p8` key to your delivery provider.

<a id="deep-links-y-universal-links"></a>
## Deep links and Universal Links

- **`sytue-rooms://` scheme** — registered in `native/ios-Info.plist.additions.xml`
  (`CFBundleURLTypes`) and `native/android-AndroidManifest.additions.xml`
  (`<data android:scheme="sytue-rooms"/>`). Works without any server configuration.
- **Universal / App Links** (`https://rooms.sytue.eu/...`) — require, **on the
  Rooms domain** (served by the BFF of `apps/frontend/apps/chat`, not by this
  repo):
  - **iOS:** `https://rooms.sytue.eu/.well-known/apple-app-site-association` (JSON, no
    extension, `Content-Type: application/json`):
    ```json
    {
      "applinks": {
        "apps": [],
        "details": [
          {
            "appID": "<TEAMID>.eu.sytue.rooms",
            "paths": ["/channels/*", "/dms/*", "*"]
          }
        ]
      }
    }
    ```
    - **Associated Domains** capability `applinks:rooms.sytue.eu` in Xcode.
  - **Android:** `https://rooms.sytue.eu/.well-known/assetlinks.json`:
    ```json
    [
      {
        "relation": ["delegate_permission/common.handle_all_urls"],
        "target": {
          "namespace": "android_app",
          "package_name": "eu.sytue.rooms",
          "sha256_cert_fingerprints": ["<SHA256 de tu clave de firma>"]
        }
      }
    ]
    ```
    - `android:autoVerify="true"` (already in the manifest fragment).

<a id="build-firma-y-publicacion"></a>
## Build, signing and publishing

Each build produces the artifact for **its own** platform; it requires the native toolchain.

- **iOS for this Capacitor project:** it has no active publishing job. The old
  `ios-apps` job in `release-mobile.yml` was retired on 2026-09-21; the interim
  Codemagic flow is retired too. The Fyne Rooms app is published through
  [`release-desktop.yml`](../../../.github/workflows/release-desktop.yml), which builds
  `apps/fyne` and not this project. Building this shell manually requires Xcode, signing and
  its own capabilities; the Capacitor instructions do not describe the suite's current
  binaries. The current flow is detailed in
  [`docs/operations.md`](../../../docs/operations.md#xcode-cloud).
- **Android (Play Store):** Android Studio → `Generate Signed Bundle (.aab)` with your
  keystore (gitignored); upload the `.aab` to Play Console. The SHA-256 fingerprint of the signing
  key (or of Play App Signing) must match `assetlinks.json`.

Certificates and store accounts → CI (ADR-0024). **No secrets in git**
(`.gitignore` covers `*.p8`/`*.p12`/`*.keystore`/`google-services.json`/…).

<a id="verificacion-en-este-entorno-macosarm64"></a>
## Verification (in this environment, macOS/arm64)

To be honest — **the signed native build is NOT built here** (it requires Xcode / the Android
SDK). What could be verified was checked:

- `package.json` is valid (JSON), `capacitor.config.ts` typechecks (`tsc --noEmit`).
- `www/native.js` is valid (`node --check`) and its **pure logic** (unread parsing and
  deep-link translation) passes the self-check (`node scripts/selfcheck.cjs`).
- `www/index.html` is valid HTML.
- See the wave report for the exact result of `npm install` / `npx cap
--version` in this environment.

<a id="que-queda-para-un-artefacto-publicable"></a>
## What remains for a publishable artifact

1. `npx cap add ios/android` + apply the `native/` fragments and the injection
   snippets (§Bridge) on a machine with Xcode/Android SDK.
2. **Push backend:** `/me/push/native` endpoint + **FCM/APNs sender** with credentials.
3. **Universal Links:** serve `apple-app-site-association` / `assetlinks.json` on the
   Rooms domain.
4. **Sign and publish** (Apple Developer / Play Console) — CI (ADR-0024).
5. (Optional) PWA cooperation with `window.SytueNative` for exact unread counts without
   relying on title parsing.

<a id="estructura"></a>
## Structure

```
apps/mobile/rooms/
├── capacitor.config.ts   # appId eu.sytue.rooms, server.url (SYTUE_ROOMS_URL), allowNavigation, plugins
├── package.json          # @capacitor/* + plugins (standalone, fuera del workspace Bun)
├── tsconfig.json         # solo para `tsc --noEmit` de la config
├── www/
│   ├── index.html        # loader/offline de respaldo (marca); carga native.js
│   └── native.js         # puente webview↔nativo (push, deep-links, statusbar, splash, badge, back)
├── native/               # fragmentos a pegar en los proyectos GENERADOS tras `cap add`
│   ├── ios-Info.plist.additions.xml
│   └── android-AndroidManifest.additions.xml
├── scripts/
│   ├── gen-icons.sh      # iconos/splash desde branding/logo/sytue-mark.svg
│   └── selfcheck.cjs     # self-check de la logica pura de native.js
├── resources/            # fuentes icon/splash de marca (para @capacitor/assets)
└── (ios/ · android/)     # proyectos nativos GENERADOS por `cap add` (gitignored)
```
