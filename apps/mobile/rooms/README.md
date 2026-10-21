# Sytue Rooms — App movil (Capacitor 8)

Cascaron **nativo** de **Sytue Rooms** para **iOS y Android**. **Envuelve el PWA de
Rooms** (`apps/frontend/apps/rooms`, servido en `https://rooms.sytue.eu`) en un webview
nativo con [Capacitor](https://capacitorjs.com) (**MIT**) y le anade integracion
movil: push, deep-links, status bar, splash, teclado, badge de no-leidos y back de
Android. **No** reimplementa la UI — es un cliente de marca sobre el mismo PWA, igual
que la app de escritorio de Rooms en Wails (retirada el 2026-09-23) y que la
app de escritorio de Mattermost/Slack.

> **Ubicacion.** `apps/mobile/` ya contiene (commit previo, otra sesion) el cascaron
> Capacitor de **Sytue One** (la suite completa, `one.sytue.eu`, ADR-0024). Esta app
> es un producto distinto — el cliente **de Rooms** (`rooms.sytue.eu`) — asi que vive
> en `apps/mobile/rooms/` para **no** destruir ese trabajo y respetar «modulo
> separado, sin tocar lo compartido». Era el espejo exacto de la app de Rooms de
> escritorio en Wails, ya retirada. Si el objetivo fuera _reemplazar_ el shell Sytue
> One por el de Rooms, muevela a la raiz de `apps/mobile/`.

Standalone **a proposito**: `package.json` propio con la CLI de Capacitor + plugins,
**fuera** del workspace Bun del frontend (`apps/frontend`). `node_modules/`, `ios/` y
`android/` estan en `.gitignore` (se generan).

## Integracion nativa

| Funcion                   | Plugin (MIT)                    | Implementacion                                                                                                                                                                                                                                                                                                                    |
| ------------------------- | ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Push**                  | `@capacitor/push-notifications` | Pide permiso, registra el token nativo (**FCM** Android / **APNs** iOS) y lo **POSTea** al backend (`POST {origen}/api/push/native`, cookie de sesion). Tap en la notificacion → deep-link al canal (`data.url`). En primer plano muestra banner (`presentationOptions`). **Sender FCM/APNs del backend: PENDIENTE** (ver abajo). |
| **Deep-links**            | `@capacitor/app` (`appUrlOpen`) | `sytue-rooms://open?url=…` y universal links `https://rooms.sytue.eu/channels/x` → navega el webview a la ruta dentro del origen de Rooms.                                                                                                                                                                                        |
| **Status bar**            | `@capacitor/status-bar`         | Grafito `#22262b` + texto claro (tokens Graphite & Brass), sin superponerse al contenido.                                                                                                                                                                                                                                         |
| **Splash**                | `@capacitor/splash-screen`      | Marca sobre marfil/grafito; `launchAutoHide:false` → `native.js` la oculta cuando Rooms ha cargado (sin flash blanco).                                                                                                                                                                                                            |
| **Teclado**               | `@capacitor/keyboard`           | `resize: 'native'` → el composer de chat sube con el teclado.                                                                                                                                                                                                                                                                     |
| **Badge de no-leidos**    | `@capawesome/capacitor-badge`   | Cuenta de no-leidos en el icono de la app; se limpia al volver a primer plano. iOS ademas recibe el badge del payload APNs.                                                                                                                                                                                                       |
| **Back (Android)**        | `@capacitor/app` (`backButton`) | Navega el historial del webview; si no puede, sale de la app.                                                                                                                                                                                                                                                                     |
| **Puente webview↔nativo** | `www/native.js`                 | Lee no-leidos del titulo (Rooms prefija `(N)`) y expone `window.SytueNative` para que el PWA empuje no-leidos/foco.                                                                                                                                                                                                               |

### El puente webview ↔ nativo (`www/native.js`)

Toda la wiring corre **dentro del webview**, sobre la pagina de Rooms, desde
`www/native.js`. Habla con el **puente global `window.Capacitor`** (que Capacitor
inyecta **incluso en un origen remoto**) llamando a `window.Capacitor.Plugins.*`.
**Seguridad:** no expone FS/exec/APIs peligrosas — solo lee no-leidos del titulo y
**navega dentro del origen de Rooms** (deep-links a otros hosts se ignoran; los
enlaces externos ya abren en el navegador del sistema por `server.allowNavigation`).

**Como el PWA detecta que corre en Capacitor:** `window.Capacitor?.isNativePlatform?.()`
es `true`; `window.Capacitor.getPlatform()` da `'ios'` | `'android'`. Ademas, una vez
cargado `native.js`, el PWA puede usar `window.SytueNative`:

```js
if (window.SytueNative?.isNative) {
  window.SytueNative.setUnread(unreadCount); // pinta el badge nativo
  // window.SytueNative.openChannel('sytue-rooms://open?url=https%3A%2F%2Frooms.sytue.eu%2Fchannels%2Fx')
}
```

**Como `native.js` llega al PWA remoto** (dos vias, no excluyentes):

1. **Inyeccion nativa (sin tocar el PWA — via por defecto).** Como Rooms es un
   origen remoto, la capa nativa inyecta el script en cada carga (idempotente por el
   guard `__sytueRoomsNative`), igual que la app de escritorio inyecta su puente.
   **Estos snippets se aplican tras `npx cap add`** (necesitan Xcode/Android Studio):

   - **iOS** — `ios/App/App/AppDelegate.swift`, en `application(_:didFinishLaunchingWithOptions:)`:

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

     `www/` se copia a `public/` dentro del bundle en `cap sync`, asi que
     `native.js` esta disponible como recurso.

   - **Android** — `android/app/src/main/java/.../MainActivity.java`, engancha el
     `WebViewClient.onPageFinished` (o un `bridge.eval(...)`) y evalua el asset:
     ```java
     String js = new String(getAssets().open("public/native.js").readAllBytes(),
                            StandardCharsets.UTF_8);
     bridge.getWebView().post(() -> bridge.getWebView().evaluateJavascript(js, null));
     ```

2. **Cooperacion del PWA (opcional, follow-up en `apps/frontend/apps/chat`).** El PWA
   detecta Capacitor y carga/importa el cliente el mismo — sin inyeccion nativa. Es un
   cambio en el repo del frontend (otra sesion), **fuera del alcance de esta ola**.

> `native.js` es JS plano y **defensivo** (cada plugin va guardado; nunca lanza), por
> lo que la misma via 1 o 2 funciona sin bundler y sin acoplar el PWA a Capacitor.

## Configuracion de la URL de Rooms

Mismo convenio `SYTUE_ROOMS_URL` que tenia la app de Rooms de escritorio (Wails):

| Origen                               | Valor                    |
| ------------------------------------ | ------------------------ |
| `SYTUE_ROOMS_URL` (env, prioritaria) | cualquier URL http(s)    |
| Default                              | `https://rooms.sytue.eu` |

```bash
SYTUE_ROOMS_URL=https://rooms.sytue.localhost npx cap run ios   # dev (Traefik + mkcert)
```

`server.url` en `capacitor.config.ts` es la URL del PWA de Rooms;
`server.allowNavigation` es una **lista cerrada de dos hosts**: el origen de Rooms y el de
**Sytue ID**, derivado del mismo apex (`rooms.sytue.eu` → `id.sytue.eu`,
`rooms.sytue.localhost` → `id.sytue.localhost`). Cualquier otro enlace abre en el
navegador del sistema, sin arrastrar la sesion. En dev, instala la CA de mkcert
(`mkcert -install`) para que el webview confie en el TLS local.

**Por que `id.*` tiene que estar.** Rooms manda al visitante sin sesion a `/auth/login`, y
de ahi a `id.*/oauth/authorize` (OIDC + PKCE). Capacitor aplica `allowNavigation` tambien a
las **redirecciones del servidor** —iOS en `WebViewDelegationHandler.decidePolicyFor`,
Android en `shouldOverrideUrlLoading`—, no solo a los clics: con `id.*` fuera de la lista,
el webview **cancela** la navegacion y la abre en el navegador del sistema. El usuario se
autentica alli, la cookie se queda alli, y la app vuelve a empezar el mismo bucle en cada
arranque. Es decir: **sin ese host el login es imposible**, no «incomodo».

## Requisitos

- **Node** (LTS) o **Bun** — para la CLI de Capacitor.
- **iOS:** **macOS + Xcode** (+ CocoaPods) y cuenta de Apple Developer para firmar/publicar.
- **Android:** **Android Studio** + Android SDK (JDK 17), `ANDROID_HOME` configurado.
- **ImageMagick** (`magick`) solo si regeneras iconos/splash (`scripts/gen-icons.sh`).

## Comandos

Desde `apps/mobile/rooms/`:

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

Tras `cap add`, aplica **una vez**:

- Los fragmentos de `native/` a `Info.plist` / `AndroidManifest.xml`.
- Los snippets de inyeccion de `native.js` (§Puente, via 1).
- La config de push por plataforma (§Push).

## Push — configuracion y estado del backend

**Cliente: HECHO** (`www/native.js`). Registra el token nativo y lo POStea:

```
POST {origen-de-Rooms}/api/push/native      (cookie de sesion, credentials:include)
Body: { "platform": "ios" | "android", "token": "<FCM/APNs token>" }
```

Se eligio un **endpoint nuevo** para tokens nativos porque el de Web Push existente
(`/api/push`, Ola 18) espera una **suscripcion RFC 8291** (`endpoint` + claves
`p256dh`/`auth`), forma incompatible con un token FCM/APNs plano.

**Backend — PENDIENTE (no se toca `services/**` en esta ola):**

- **Endpoint** `POST /me/push/native` en `services/chat` (BFF `/api/push/native`) que
  persista `{subject, platform, token}` (analogo a `domain.PushSubscription`, con
  `token` en vez de `endpoint`/claves).
- **Sender** que entregue via **FCM HTTP v1** (Android) y **APNs** (iOS), incluyendo
  `data.url` (ruta del canal/mensaje) para el deep-link al tocar la notificacion.
- **Credenciales necesarias** (secretos, **fuera de git**):
  - **FCM:** cuenta de servicio de Firebase (`google-services.json` en la app +
    JSON de service account en el backend), o server key legacy.
  - **APNs:** clave `.p8` (Key ID + Team ID) o certificado `.p12`, y el `bundle id`
    `eu.sytue.rooms`; entitlement `aps-environment` (development/production).

Config de push en el proyecto nativo (tras `cap add`):

- **Android:** coloca `google-services.json` en `android/app/` (gitignored). El
  plugin cablea FCM al `cap sync`.
- **iOS:** en Xcode, capability **Push Notifications** (+ **Background Modes → Remote
  notifications**); sube la clave APNs `.p8` a tu proveedor de envio.

## Deep-links y Universal Links

- **Esquema `sytue-rooms://`** — registrado en `native/ios-Info.plist.additions.xml`
  (`CFBundleURLTypes`) y `native/android-AndroidManifest.additions.xml`
  (`<data android:scheme="sytue-rooms"/>`). Funciona sin configuracion de servidor.
- **Universal / App Links** (`https://rooms.sytue.eu/...`) — requieren, **en el
  dominio de Rooms** (servido por el BFF de `apps/frontend/apps/chat`, no por este
  repo):
  - **iOS:** `https://rooms.sytue.eu/.well-known/apple-app-site-association` (JSON, sin
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
    - capability **Associated Domains** `applinks:rooms.sytue.eu` en Xcode.
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
    - `android:autoVerify="true"` (ya en el fragmento del manifest).

## Build, firma y publicacion

Cada build produce el artefacto de **su** plataforma; exige el toolchain nativo.

- **iOS de este proyecto Capacitor:** no tiene un job de publicacion activo. El antiguo
  `ios-apps` de `release-mobile.yml` se retiro el 2026-09-21; el flujo intermedio de
  Codemagic tambien esta retirado. La app Rooms de Fyne se publica mediante
  [`release-desktop.yml`](../../../.github/workflows/release-desktop.yml), que construye
  `apps/fyne` y no este proyecto. Compilar manualmente este shell exige Xcode, firma y
  capabilities propias; las instrucciones de Capacitor no describen los binarios actuales
  de la suite. El flujo vigente se detalla en
  [`docs/operations.md`](../../../docs/operations.md#xcode-cloud).
- **Android (Play Store):** Android Studio → `Generate Signed Bundle (.aab)` con tu
  keystore (gitignored); sube el `.aab` a Play Console. La huella SHA-256 de la clave
  de firma (o de Play App Signing) debe coincidir con `assetlinks.json`.

Certificados y cuentas de tienda → CI (ADR-0024). **Nada de secretos en git**
(`.gitignore` cubre `*.p8`/`*.p12`/`*.keystore`/`google-services.json`/…).

## Verificacion (en este entorno, macOS/arm64)

Honesto — **el build nativo firmado NO se construye aqui** (exige Xcode / Android
SDK). Lo verificable si se comprobo:

- `package.json` valido (JSON), `capacitor.config.ts` typechea (`tsc --noEmit`).
- `www/native.js` valido (`node --check`) y su **logica pura** (parseo de no-leidos y
  traduccion de deep-links) pasa el self-check (`node scripts/selfcheck.cjs`).
- `www/index.html` es HTML valido.
- Ver el reporte de la ola para el resultado exacto de `npm install` / `npx cap
--version` en este entorno.

## Que queda para un artefacto publicable

1. `npx cap add ios/android` + aplicar los fragmentos de `native/` y los snippets de
   inyeccion (§Puente) en una maquina con Xcode/Android SDK.
2. **Backend push:** endpoint `/me/push/native` + **sender FCM/APNs** con credenciales.
3. **Universal Links:** servir `apple-app-site-association` / `assetlinks.json` en el
   dominio de Rooms.
4. **Firmar y publicar** (Apple Developer / Play Console) — CI (ADR-0024).
5. (Opcional) Cooperacion del PWA con `window.SytueNative` para no-leidos exactos sin
   depender del parseo del titulo.

## Estructura

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
