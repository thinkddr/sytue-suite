<a id="apps-moviles-de-sytue-capacitor-8"></a>

# Sytue mobile apps (Capacitor 8)

> **⚠️ THIS PROJECT NO LONGER PUBLISHES ANYTHING (2026-09-21).** The suite's mobile apps are
> the **Fyne** ones (`apps/fyne/cmd/*`, packaged by
> [`apps/fyne/scripts/empaquetar.sh`](../../fyne/scripts/empaquetar.sh)), native and with no
> webview: Android in [`release-mobile.yml`](../../../.github/workflows/release-mobile.yml),
> iOS and macOS in [`release-desktop.yml`](../../../.github/workflows/release-desktop.yml).
> Codemagic is retired. Between 2026-09-21 and 2026-09-23 the
> Gio ones were, and they have been removed from the repository. Neither `release-mobile.yml` nor
> `ci-mobile.yml` calls anything from here any longer.
>
> **It is kept, deliberately not deleted**, and the reason is written in
> [`docs/operations.md` §2.1b](../../../docs/operations.md#21b-clientes-moviles-una-sola-tecnologia):
> its maintenance cost is SHARED with [`../rooms`](../rooms), another
> Capacitor project that is kept, so deleting it does not switch off any tool or any gate. That
> section also has the exact list of files that must be touched IN THE SAME COMMIT to
> retire it. **What follows describes how it used to be, not how things are published today.**

Native shells for **iOS, iPadOS and Android**. They wrap the **same web** (a single codebase)
in a native webview with [Capacitor](https://capacitorjs.com) (**MIT**): presence in the App
Store / Play Store and, in later phases, native APIs (share, files, camera). They do not
reimplement editors; they are branded clients —the same approach as the Wails desktop,
retired on 2026-09-23— on top of Sytue's Go backend.

<a id="un-proyecto-diez-apps"></a>

## ONE project, TEN apps

Each product is published **separately**: its own package identifier, name,
icon and deep link scheme. What they share is this project, because ten folders
would be ten `package.json`, ten `node_modules` and ten places to repeat every
fix. It is selected with `SYTUE_APP` (table in [`products.ts`](products.ts)):

| `SYTUE_APP` | App            | Package             | Web                | Scheme              |
| ----------- | -------------- | ------------------- | ------------------ | ------------------- |
| _(empty)_   | Sytue One      | `eu.sytue.one`      | `one.*`            | `sytue://`          |
| `mail`      | Sytue Mail     | `eu.sytue.mail`     | `mail.*`           | `sytue-mail://`     |
| `drive`     | Sytue Drive    | `eu.sytue.drive`    | `drive.*`          | `sytue-drive://`    |
| `docs`      | Sytue Docs     | `eu.sytue.docs`     | `docus.*/docus/`   | `sytue-docs://`     |
| `sheets`    | Sytue Sheets   | `eu.sytue.sheets`   | `calcus.*/calcus/` | `sytue-sheets://`   |
| `slides`    | Sytue Slides   | `eu.sytue.slides`   | `diapos.*/diapos/` | `sytue-slides://`   |
| `calendar`  | Sytue Calendar | `eu.sytue.calendar` | `calendar.*`       | `sytue-calendar://` |
| `contacts`  | Sytue Contacts | `eu.sytue.contacts` | `contacts.*`       | `sytue-contacts://` |
| `cloud`     | Sytue Cloud    | `eu.sytue.cloud`    | `cloud.*`          | `sytue-cloud://`    |
| `admin`     | Sytue Admin    | `eu.sytue.admin`    | `admin.*`          | `sytue-admin://`    |

Contacts shares CardDAV with Mail's suggestions, but it is published as its own app.
**Rooms is not here either**: it lives in [`apps/mobile/rooms`](../rooms), with native push,
an unread badge and its own channel, which the rest do not need.

```bash
SYTUE_APP=mail npx cap sync android    # produce Sytue Mail
npx cap sync android                   # sin variable: la suite
```

An unknown key **fails**: silently building the wrong app and uploading it to the
store would be much worse than an error.

The **installable PWA** (manifest + service worker in [`apps/frontend/apps/portal`](../../frontend/apps/portal))
is the foundation: it provides installation, an icon and an offline shell without stores. Capacitor adds
store presence on top of that same web.

Decision and phases: [`docs/decisions.md#0024`](../../../docs/decisions.md#0024).

<a id="estado"></a>

## Status

- **Done:** a shell that loads the suite (`server.url`), navigation restricted to Sytue
  domains, icon/splash and an offline experience shared per account. After a first
  connected launch, the service worker keeps the shell, screens and reads; IndexedDB
  keeps documents and drafts. **The first launch and sign-in require a
  network**: no tokens are persisted in client storage.
- **Pending (follow-up, not implemented):**
  - **Native APIs**: share, files, camera, push notifications
    (`@capacitor/share`, `@capacitor/filesystem`, `@capacitor/push-notifications`…).
  - **Secure native session** and CSP hardening for the embedded webview (ADR-0024).
  - **iOS/iPadOS: the build path EXISTS but has never been run.** The
    `ios-apps` job in `.sygit/workflows/release-mobile.yml` archives and exports the eleven `.ipa`
    (ten products + Rooms), but **it is skipped entirely as long as there are no Apple
    credentials** — which do not exist today. What exactly is needed:
    [§Publish on iOS](#publicar-en-ios-app-store). Android publishes eleven APKs and
    desktop 33 artifacts (11 apps × 3 platforms).

<a id="ipados-que-hereda-y-que-no"></a>

## iPadOS: what it inherits and what it does not

iPadOS **is not a separate platform in Capacitor**: it is built from the same
`ios/` project. What follows is **verified against the real template** that `cap add ios` generates
(Capacitor 8.5.0, `node_modules/@capacitor/cli/assets/ios-pods-template.tar.gz`), not
assumed.

**Already works without touching anything** (template defaults, not our configuration):

| Capability                  | Key                                                    | Template value                                         |
| --------------------------- | ------------------------------------------------------ | ------------------------------------------------------ |
| Installs on iPad            | `TARGETED_DEVICE_FAMILY` (`project.pbxproj`)           | `"1,2"` = iPhone **+ iPad**                            |
| All four orientations       | `UISupportedInterfaceOrientations~ipad` (`Info.plist`) | portrait, upside-down, landscape L and R               |
| **Split View** / Slide Over | `UIRequiresFullScreen`                                 | **absent** ⇒ the app takes part                        |
| Pinch to zoom               | `ios.zoomEnabled` (`capacitor.config.ts`)              | Capacitor ships it as `false`; **we set it to `true`** |

That last one was **not** an acceptable default: with `zoomEnabled:false` Capacitor cancels the
gesture (`WebViewDelegationHandler.scrollViewWillBeginZooming`) and, since WKWebView does not
honor Dynamic Type for web content either, the user is left **with no way at all to
enlarge**. It violates WCAG 1.4.4 —which the house requires (ADR-0004)— and it shows exactly where it
matters most: a PDF, a plan or a spreadsheet on an iPad.

**What it does NOT inherit, and why:**

- **Multi-window (two Sytue Docs side by side) and full Stage Manager.** The template
  ships `UIApplicationSupportsMultipleScenes: false`, and **it is not a flag that can be
  flipped**: Capacitor 8's `SceneDelegateProxy` is a **singleton**
  (`SceneDelegateProxy.shared`, with a shared `lastURL`), so several scenes
  would share one bridge. Split View with **another** app does work; two windows **of the
  same** app do not. It requires upstream multi-scene support in Capacitor.
- **Apple Pencil.** There is nothing to configure here, and it is better not to pretend otherwise:
  in a webview the pencil arrives as **Pointer Events with `pointerType: "pen"`** (pressure
  in `pressure`, tilt in `tiltX`/`tiltY`), and the one that has to handle them is the
  **web** (`apps/frontend`), with `touch-action: none` on the canvas. What a webview
  **cannot** receive is the native `UIPencilInteraction` API (double tap and
  “squeeze” on Pencil 2/Pro) or PencilKit: that requires a native plugin that does not exist today.
- **Content mode.** `ios.preferredContentMode` is not set, so it is
  `'recommended'`: on a large iPad, WKWebView requests the site in **desktop** mode. That is
  what we want for editors, but it is worth knowing — the Docs/Sheets ribbon measures
  ~1,242 px and an iPad Mini is 768 (measured, `docs/design.md`), so ribbon clipping
  shows up **earlier** on an iPad than on a laptop.
- **Downloads.** The public page offers an iPad visitor the **macOS DMG**:
  `detectPlatform` (`apps/frontend/apps/www/src/lib/downloads.ts`) breaks the tie by user
  agent and iPadOS 13+ announces itself as `Macintosh`. The comment itself says that
  touch support resolves it, but the code does not check it. Outside this directory.

<a id="requisitos"></a>

## Requirements

- **Node** (LTS) or **Bun** — for the Capacitor CLI (`npm install` here).
- **iOS / iPadOS:** **macOS + Xcode** (+ CocoaPods) and an Apple Developer account to
  sign/publish. **A single `ios/` project covers both**; the iPad is tested with an
  iPad simulator (`npx cap run ios --target <udid de iPad>`) or with the device.
- **Android:** **Android Studio** + Android SDK (JDK 17). `ANDROID_HOME` configured.
- **ImageMagick** (`magick`) only if you regenerate icons/splash (`scripts/gen-icons.sh`).

This package is **deliberately outside** the frontend Bun workspace
(`apps/frontend`): it only needs the Capacitor CLI, not the monorepo deps.
`node_modules/`, `ios/` and `android/` are in `.gitignore` (they are generated).

<a id="comandos"></a>

## Commands

From `apps/mobile/suite/`:

```bash
npm install                 # trae @capacitor/core|cli|ios|android (gitignored)
npx cap --version           # comprueba la CLI

# Anadir las plataformas nativas (genera ios/ y android/ — requieren Xcode/SDK):
npx cap add ios
npx cap add android

# Iconos y splash nativos desde la marca (fuentes en resources/):
bash scripts/gen-icons.sh   # regenera resources/{icon,splash,splash-dark}.png
npm run assets              # @capacitor/assets -> assets por plataforma

npx cap sync                # copia web + config a los proyectos nativos
npx cap open ios            # abre Xcode        (compilar/ejecutar/firmar alli)
npx cap open android        # abre Android Studio
npx cap run ios             # build + deploy a simulador/dispositivo
npx cap run android
```

> Building and signing iOS/Android requires the native toolchain (Xcode / Android SDK) and
> store accounts; it will be documented in `docs/operations.md#2-cicd-y-cadena-de-suministro`
> (ADR-0024). The local shell shows the startup; the remote authenticated web keeps
> a single codebase and reopens from its cache after it has been used online.

<a id="publicar-en-ios-app-store"></a>

## Publish on iOS (App Store)

**RETIRED on 2026-09-21.** The `ios-apps` job described here no longer exists: it requested
`runs-on: sytue-macos`, a label that no git.gay runner served, so its twelve
jobs were queued forever. Its interim replacement on Codemagic is also retired.
The current flow is [`release-desktop.yml`](../../../.github/workflows/release-desktop.yml),
which builds **Fyne** from `apps/fyne`, without compiling this Capacitor project.
What follows is valid only as the history of the old path, including the references to accounts
and credentials that were missing at the time.

> **It has never been run.** There is no Apple Developer Program account, no certificate and no
> App Store Connect: **the secrets are empty and the job is skipped entirely**, just as
> Android is skipped without a keystore. It is written so that the path exists the day the
> credentials exist — not because it is tested. Neither the `.ipa` nor the archive has ever
> been produced anywhere.

The `.ipa` is attached to the release but is **not** copied to the public downloads bucket: iOS
does not allow installing outside the store, so offering it on the web would be a useless link.
Its destination is App Store Connect.

<a id="lo-que-tiene-que-aportar-el-propietario"></a>

### What the owner has to provide

Without this, everything else is paper. **None of this can be solved by the repository.**

1. **Apple Developer Program**, ~99 USD/year. As an **organization** (not personal) it requires
   the legal entity's **D-U-N-S** number and verification by Apple: that takes **days or weeks**,
   not minutes. It is the real bottleneck.
2. **Distribution certificate** (_Apple Distribution_) exported as a **`.p12`** with its
   password. It is generated once from the account; the CI imports it into an ephemeral keychain.
3. **App Store Connect API key** (`.p8`) with the **App Manager** role or higher, plus its
   **Key ID** and **Issuer ID**. This is what enables **automatic** signing: Xcode registers the
   App IDs and creates/renews the provisioning profiles by itself. Without it,
   **eleven** `.mobileprovision` files would be needed, each one expiring on its own every twelve months.
4. **Team ID** (10 characters, on the account's membership page).
5. **Eleven app records in App Store Connect**, one per package identifier:
   `eu.sytue.one`, `.mail`, `.drive`, `.docs`, `.sheets`, `.slides`, `.calendar`,
   `.contacts`, `.cloud`, `.admin` and `eu.sytue.rooms`. The archive can create the App ID,
   but the **app record** (name, category, store listing) is done by hand once.

<a id="secretos-del-repositorio"></a>

### Repository secrets

All in Secret Manager, like `ANDROID_KEYSTORE_*`. **If any of the first three
is missing, the job is skipped and the release goes out anyway** (without iOS):

| Secret                         | What it is                                                |
| ------------------------------ | --------------------------------------------------------- |
| `APPLE_CERTIFICATE_P12_BASE64` | the distribution `.p12`, in base64 (`base64 -i cert.p12`) |
| `APPLE_CERTIFICATE_PASSWORD`   | password with which that `.p12` was exported              |
| `APPLE_TEAM_ID`                | 10-character Team ID                                      |
| `APPLE_API_KEY_P8_BASE64`      | the App Store Connect `.p8` key, in base64                |
| `APPLE_API_KEY_ID`             | Key ID of that key                                        |
| `APPLE_API_ISSUER_ID`          | App Store Connect Issuer ID (UUID, one per team)          |

<a id="lo-que-este-job-no-hace-a-proposito"></a>

### What this job does NOT do, on purpose

- **It does not upload anything to App Store Connect.** `destination: export` leaves the `.ipa` on disk.
  Publishing (TestFlight, review, release date) is a product decision, not a
  side effect of cutting a tag.
- **It does not configure capabilities.** Rooms needs **Push Notifications** (`aps-environment`)
  and **Associated Domains** for universal links; the Capacitor template does not include those
  entitlements and the archive will come out **without push**. Fixing it requires adding an
  `App.entitlements` to the generated project — same pattern as `scripts/setup-deeplinks.sh`,
  not yet written. See [`../rooms/README.md`](../rooms/README.md).
- **It does not cover store listing requirements**: privacy manifest, export compliance,
  screenshots per screen size. They are resolved in App Store Connect, not in YAML.
- **There is no iPad as a separate platform**: the same `.ipa` covers iPhone and iPad
  (`TARGETED_DEVICE_FAMILY "1,2"`). What it inherits and what it does not:
  [§iPadOS](#ipados-que-hereda-y-que-no).

## Deep links

“Open in the app” on the web emits `sytue-<producto>://open?url=<https>` —the same
gesture as Office Online's `ms-word:ofe|u|…`— and the system hands it to the installed app.

**Each app registers ONLY its own scheme.** If they all declared `sytue://`, the operating system
would pick one arbitrarily and the link would open the wrong one.

Since `ios/` and `android/` are **generated** by `cap add` (they are in `.gitignore`), their manifest cannot
be versioned: the registration is applied by an **idempotent** script with the scheme of the active
product.

```bash
SYTUE_APP=mail npx cap add android
SYTUE_APP=mail npm run deeplinks    # intent-filter (Android) + CFBundleURLTypes (iOS)
SYTUE_APP=mail npx cap sync
```

The incoming URL arrives through `appUrlOpen` (`@capacitor/app`) and is handled by the **Sytue Shell**
(`listenForDeepLinks` in `@sytue/shell`), which navigates the webview to the document. **The destination
is always validated** against the suite's domain before navigating: anyone can emit a `sytue://`
and the operating system does not check who, so without that fence a single
link would be enough to take the webview —with the session inside— to a foreign site.

<a id="configuracion-de-la-url"></a>

## URL configuration

Each app builds its URL against the suite's **apex**, derived from `SYTUE_URL` (the same
convention the Wails desktop had). That way the same project works in production and in dev without
branching:

| `SYTUE_URL`                   | Apex              | `SYTUE_APP=drive` loads          |
| ----------------------------- | ----------------- | -------------------------------- |
| _(unset)_                     | `sytue.eu`        | `https://drive.sytue.eu/`        |
| `https://one.sytue.localhost` | `sytue.localhost` | `https://drive.sytue.localhost/` |

```bash
SYTUE_URL=https://one.sytue.localhost SYTUE_APP=drive npx cap run ios   # dev
```

`server.allowNavigation` lists the hosts that stay inside the webview; any other
link opens in the system browser, without carrying the session along.

In dev, install the mkcert CA (`mkcert -install`) so that the webview trusts
the local TLS of `*.sytue.localhost`.

<a id="iconos-y-splash"></a>

## Icons and splash

They are generated from the `@sytue/ui` product SVGs —the same ones used by the app
switcher and the desktop apps; no logos are invented—. `scripts/gen-icons.sh` produces the
**sources** of the active product in `resources/`:

- `resources/icon.png` (1024) — mark centered on full-bleed graphite (maskable-safe).
- `resources/splash.png` / `splash-dark.png` (2732) — mark on ivory / graphite.

```bash
SYTUE_APP=mail bash scripts/gen-icons.sh   # icono y splash de Sytue Mail
SYTUE_APP=mail npm run assets              # assets nativos por plataforma
```

`resources/` holds those of the **last product generated**: it is a build artifact, not a
per-app source. Regenerate it when you change `SYTUE_APP`.

<a id="estructura"></a>

## Structure

```
apps/mobile/suite/
├── capacitor.config.ts   # appId eu.sytue.mobile, server.url (SYTUE_URL), allowNavigation
├── package.json          # deps @capacitor/* (fuera del workspace Bun)
├── www/index.html        # webDir local (splash de arranque; runtime usa server.url)
├── resources/            # fuentes icon/splash de marca (para @capacitor/assets)
├── scripts/gen-icons.sh  # regeneracion de iconos/splash desde la marca
└── (ios/ · android/)     # proyectos nativos GENERADOS por `cap add` (gitignored)
```
