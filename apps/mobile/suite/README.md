# Apps moviles de Sytue (Capacitor 8)

> **⚠️ ESTE PROYECTO YA NO PUBLICA NADA (2026-09-21).** Las apps moviles de la suite son
> las de **Fyne** (`apps/fyne/cmd/*`, empaquetadas por
> [`apps/fyne/scripts/empaquetar.sh`](../../fyne/scripts/empaquetar.sh)), nativas y sin
> webview: Android en [`release-mobile.yml`](../../../.github/workflows/release-mobile.yml),
> iOS y macOS en [`release-desktop.yml`](../../../.github/workflows/release-desktop.yml).
> Codemagic esta retirado. Entre el 2026-09-21 y el 2026-09-23 lo
> fueron las de Gio, retiradas del repositorio. Ni `release-mobile.yml` ni
> `ci-mobile.yml` llaman ya a nada de aqui.
>
> **Se conserva sin borrar a proposito**, y el motivo esta escrito en
> [`docs/operations.md` §2.1b](../../../docs/operations.md#21b-clientes-moviles-una-sola-tecnologia):
> su coste de mantenimiento es COMPARTIDO con [`../rooms`](../rooms), otro proyecto
> Capacitor conservado, asi que borrarlo no apaga ninguna herramienta ni ningun gate. Alli esta
> tambien la lista exacta de ficheros que hay que tocar EN EL MISMO COMMIT para darlo de
> baja. **Lo de abajo describe como era, no como se publica hoy.**

Shells nativos para **iOS, iPadOS y Android**. Envuelven la **misma web** (codigo unico)
en un webview nativo con [Capacitor](https://capacitorjs.com) (**MIT**): presencia en App
Store / Play Store y, en fases siguientes, APIs nativas (compartir, ficheros, camara). No
reimplementan editores; son clientes de marca —mismo enfoque que el escritorio de Wails,
retirado el 2026-09-23— sobre el backend Go de Sytue.

## UN proyecto, DIEZ apps

Cada producto se publica **por separado**: su identificador de paquete, su nombre, su
icono y su esquema de deep link. Lo que comparten es este proyecto, porque diez carpetas
serian diez `package.json`, diez `node_modules` y diez sitios donde repetir cada
arreglo. Se elige con `SYTUE_APP` (tabla en [`products.ts`](products.ts)):

| `SYTUE_APP` | App            | Paquete             | Web                | Esquema             |
| ----------- | -------------- | ------------------- | ------------------ | ------------------- |
| _(vacio)_   | Sytue One      | `eu.sytue.one`      | `one.*`            | `sytue://`          |
| `mail`      | Sytue Mail     | `eu.sytue.mail`     | `mail.*`           | `sytue-mail://`     |
| `drive`     | Sytue Drive    | `eu.sytue.drive`    | `drive.*`          | `sytue-drive://`    |
| `docs`      | Sytue Docs     | `eu.sytue.docs`     | `docus.*/docus/`   | `sytue-docs://`     |
| `sheets`    | Sytue Sheets   | `eu.sytue.sheets`   | `calcus.*/calcus/` | `sytue-sheets://`   |
| `slides`    | Sytue Slides   | `eu.sytue.slides`   | `diapos.*/diapos/` | `sytue-slides://`   |
| `calendar`  | Sytue Calendar | `eu.sytue.calendar` | `calendar.*`       | `sytue-calendar://` |
| `contacts`  | Sytue Contacts | `eu.sytue.contacts` | `contacts.*`       | `sytue-contacts://` |
| `cloud`     | Sytue Cloud    | `eu.sytue.cloud`    | `cloud.*`          | `sytue-cloud://`    |
| `admin`     | Sytue Admin    | `eu.sytue.admin`    | `admin.*`          | `sytue-admin://`    |

Contacts comparte CardDAV con las sugerencias de Mail, pero se publica como app propia.
**Rooms tampoco esta aqui**: vive en [`apps/mobile/rooms`](../rooms), con push nativo,
badge de no leidos y su propio canal, que el resto no necesita.

```bash
SYTUE_APP=mail npx cap sync android    # produce Sytue Mail
npx cap sync android                   # sin variable: la suite
```

Una clave desconocida **falla**: construir en silencio la app equivocada y subirla a la
tienda seria mucho peor que un error.

La **PWA instalable** (manifest + service worker en [`apps/frontend/apps/portal`](../../frontend/apps/portal))
es la base: da instalacion, icono y shell offline sin tiendas. Capacitor anade la
presencia en tiendas sobre esa misma web.

Decision y fases: [`docs/decisions.md#0024`](../../../docs/decisions.md#0024).

## Estado

- **Hecho:** shell que carga la suite (`server.url`), navegacion restringida a dominios
  Sytue, icono/splash y experiencia offline compartida por cuenta. Tras una primera
  apertura conectada, el service worker conserva shell, pantallas y lecturas; IndexedDB
  conserva documentos y borradores. **El primer arranque y el inicio de sesion requieren
  red**: no se persisten tokens en almacenamiento cliente.
- **Pendiente (follow-up, no implementado):**
  - **APIs nativas**: compartir, ficheros, camara, notificaciones push
    (`@capacitor/share`, `@capacitor/filesystem`, `@capacitor/push-notifications`…).
  - **Sesion nativa segura** y endurecido de CSP para webview embebido (ADR-0024).
  - **iOS/iPadOS: la ruta de build EXISTE pero no se ha ejecutado nunca.** El job
    `ios-apps` de `.sygit/workflows/release-mobile.yml` archiva y exporta los once `.ipa`
    (diez productos + Rooms), pero **se salta entero mientras no haya credenciales de
    Apple** — que hoy no existen. Que hace falta exactamente:
    [§Publicar en iOS](#publicar-en-ios-app-store). Android publica once APK y
    escritorio 33 artefactos (11 apps × 3 plataformas).

## iPadOS: que hereda y que no

iPadOS **no es una plataforma aparte en Capacitor**: se construye desde el mismo proyecto
`ios/`. Lo que sigue esta **verificado sobre la plantilla real** que genera `cap add ios`
(Capacitor 8.5.0, `node_modules/@capacitor/cli/assets/ios-pods-template.tar.gz`), no
supuesto.

**Ya funciona sin tocar nada** (defaults de la plantilla, no configuracion nuestra):

| Capacidad                   | Clave                                                  | Valor de la plantilla                                  |
| --------------------------- | ------------------------------------------------------ | ------------------------------------------------------ |
| Se instala en iPad          | `TARGETED_DEVICE_FAMILY` (`project.pbxproj`)           | `"1,2"` = iPhone **+ iPad**                            |
| Las cuatro orientaciones    | `UISupportedInterfaceOrientations~ipad` (`Info.plist`) | portrait, upside-down, landscape L y R                 |
| **Split View** / Slide Over | `UIRequiresFullScreen`                                 | **ausente** ⇒ la app participa                         |
| Pellizco para ampliar       | `ios.zoomEnabled` (`capacitor.config.ts`)              | Capacitor lo trae en `false`; **lo ponemos en `true`** |

Ese ultimo **no** era un default aceptable: con `zoomEnabled:false` Capacitor cancela el
gesto (`WebViewDelegationHandler.scrollViewWillBeginZooming`) y, como WKWebView tampoco
atiende Dynamic Type para contenido web, el usuario se queda **sin ninguna forma de
agrandar**. Incumple WCAG 1.4.4 —que la casa exige (ADR-0004)— y se nota justo donde mas
importa: un PDF, un plano o una hoja de calculo en iPad.

**Lo que NO hereda, y por que:**

- **Multi-ventana (dos Sytue Docs en paralelo) y Stage Manager completo.** La plantilla
  trae `UIApplicationSupportsMultipleScenes: false`, y **no es un flag que se pueda
  girar**: el `SceneDelegateProxy` de Capacitor 8 es un **singleton**
  (`SceneDelegateProxy.shared`, con `lastURL` compartido), asi que varias escenas
  compartirian un puente. Split View con **otra** app si funciona; dos ventanas **de la
  misma** app, no. Requiere soporte multi-escena aguas arriba en Capacitor.
- **Apple Pencil.** No hay nada que configurar aqui, y conviene no fingir lo contrario:
  en un webview el lapiz llega como **Pointer Events con `pointerType: "pen"`** (presion
  en `pressure`, inclinacion en `tiltX`/`tiltY`), y quien tiene que atenderlos es la
  **web** (`apps/frontend`), con `touch-action: none` en el lienzo. Lo que un webview
  **no** puede recibir es la API nativa de `UIPencilInteraction` (doble toque y
  «squeeze» del Pencil 2/Pro) ni PencilKit: eso exige un plugin nativo que hoy no existe.
- **Modo de contenido.** `ios.preferredContentMode` no se fija, asi que vale
  `'recommended'`: en iPad grande WKWebView pide el sitio en modo **escritorio**. Es lo
  que queremos para editores, pero conviene saberlo — la cinta de Docs/Sheets mide
  ~1.242 px y un iPad Mini tiene 768 (medido, `docs/design.md`), asi que el recorte de
  cinta se ve **antes** en iPad que en un portatil.
- **Descargas.** La pagina publica ofrece a un visitante de iPad el **DMG de macOS**:
  `detectPlatform` (`apps/frontend/apps/www/src/lib/downloads.ts`) desempata por user
  agent y iPadOS 13+ se anuncia como `Macintosh`. El propio comentario dice que lo
  resuelve el soporte tactil, pero el codigo no lo comprueba. Fuera de este directorio.

## Requisitos

- **Node** (LTS) o **Bun** — para la CLI de Capacitor (`npm install` aqui).
- **iOS / iPadOS:** **macOS + Xcode** (+ CocoaPods) y una cuenta de Apple Developer para
  firmar/publicar. **Un solo proyecto `ios/` cubre las dos**; el iPad se prueba con un
  simulador de iPad (`npx cap run ios --target <udid de iPad>`) o con el dispositivo.
- **Android:** **Android Studio** + Android SDK (JDK 17). `ANDROID_HOME` configurado.
- **ImageMagick** (`magick`) solo si regeneras iconos/splash (`scripts/gen-icons.sh`).

Este paquete esta **a proposito fuera** del workspace Bun del frontend
(`apps/frontend`): solo necesita la CLI de Capacitor, no las deps del monorepo.
`node_modules/`, `ios/` y `android/` estan en `.gitignore` (se generan).

## Comandos

Desde `apps/mobile/suite/`:

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

> Compilar y firmar iOS/Android exige el toolchain nativo (Xcode / Android SDK) y
> cuentas de las tiendas; se documentara en `docs/operations.md#2-cicd-y-cadena-de-suministro`
> (ADR-0024). El shell local muestra el arranque; la web autenticada remota mantiene
> una sola base de codigo y se vuelve a abrir desde su caché tras haberla usado con red.

## Publicar en iOS (App Store)

**RETIRADO el 2026-09-21.** El job `ios-apps` que se describia aqui ya no existe: pedia
`runs-on: sytue-macos`, una etiqueta que ningun runner de git.gay atendia, asi que sus doce
trabajos se encolaban para siempre. Su relevo intermedio en Codemagic tambien esta retirado.
El flujo vigente es [`release-desktop.yml`](../../../.github/workflows/release-desktop.yml),
que construye **Fyne** desde `apps/fyne`, sin compilar este proyecto Capacitor.
Lo que sigue vale solo como historial de la ruta vieja, incluidas las referencias a cuentas
y credenciales que faltaban entonces.

> **Nunca se ha ejecutado.** No hay cuenta de Apple Developer Program, ni certificado, ni
> App Store Connect: **los secretos estan vacios y el job se salta entero**, igual que
> Android se salta sin keystore. Esta escrito para que la ruta exista el dia que existan
> las credenciales — no porque este probado. Ni el `.ipa` ni el archive se han producido
> jamas en ningun sitio.

El `.ipa` se adjunta a la release pero **no** se copia al bucket publico de descargas: iOS
no permite instalar fuera de la tienda, asi que ofrecerlo en la web seria un enlace inutil.
Su destino es App Store Connect.

### Lo que tiene que aportar el propietario

Sin esto, todo lo demas es papel. **Nada de esto lo puede resolver el repositorio.**

1. **Apple Developer Program**, ~99 USD/ano. Como **organizacion** (no personal) exige
   numero **D-U-N-S** de la entidad legal y verificacion por Apple: son **dias o semanas**,
   no minutos. Es el cuello de botella real.
2. **Certificado de distribucion** (_Apple Distribution_) exportado como **`.p12`** con su
   contrasena. Se genera una vez desde la cuenta; el CI lo importa en un llavero efimero.
3. **Clave de API de App Store Connect** (`.p8`) con rol **App Manager** o superior, mas su
   **Key ID** e **Issuer ID**. Es lo que permite firma **automatica**: Xcode registra los
   App ID y crea/renueva los perfiles de aprovisionamiento solo. Sin ella harian falta
   **once** `.mobileprovision`, cada uno caducando por su cuenta cada doce meses.
4. **Team ID** (10 caracteres, en la pagina de pertenencia de la cuenta).
5. **Once registros de app en App Store Connect**, uno por identificador de paquete:
   `eu.sytue.one`, `.mail`, `.drive`, `.docs`, `.sheets`, `.slides`, `.calendar`,
   `.contacts`, `.cloud`, `.admin` y `eu.sytue.rooms`. El archive puede crear el App ID,
   pero el **registro de la app** (nombre, categoria, ficha) se hace a mano una vez.

### Secretos del repositorio

Todos en Secret Manager, como `ANDROID_KEYSTORE_*`. **Si falta cualquiera de los tres
primeros, el job se salta y la release sale igual** (sin iOS):

| Secreto                        | Que es                                                      |
| ------------------------------ | ----------------------------------------------------------- |
| `APPLE_CERTIFICATE_P12_BASE64` | el `.p12` de distribucion, en base64 (`base64 -i cert.p12`) |
| `APPLE_CERTIFICATE_PASSWORD`   | contrasena con la que se exporto ese `.p12`                 |
| `APPLE_TEAM_ID`                | Team ID de 10 caracteres                                    |
| `APPLE_API_KEY_P8_BASE64`      | la clave `.p8` de App Store Connect, en base64              |
| `APPLE_API_KEY_ID`             | Key ID de esa clave                                         |
| `APPLE_API_ISSUER_ID`          | Issuer ID de App Store Connect (UUID, uno por equipo)       |

### Lo que este job NO hace, a proposito

- **No sube nada a App Store Connect.** `destination: export` deja el `.ipa` en disco.
  Publicar (TestFlight, revision, fecha de salida) es una decision de producto, no un
  efecto secundario de cortar un tag.
- **No configura capabilities.** Rooms necesita **Push Notifications** (`aps-environment`)
  y **Associated Domains** para universal links; la plantilla de Capacitor no trae esos
  entitlements y el archive saldra **sin push**. Arreglarlo pide anadir un
  `App.entitlements` al proyecto generado — mismo patron que `scripts/setup-deeplinks.sh`,
  todavia sin escribir. Ver [`../rooms/README.md`](../rooms/README.md).
- **No cubre requisitos de ficha**: manifiesto de privacidad, cumplimiento de exportacion,
  capturas por tamano de pantalla. Se resuelven en App Store Connect, no en YAML.
- **No hay iPad como plataforma aparte**: el mismo `.ipa` cubre iPhone y iPad
  (`TARGETED_DEVICE_FAMILY "1,2"`). Que hereda y que no:
  [§iPadOS](#ipados-que-hereda-y-que-no).

## Deep links

«Abrir en la aplicacion» en la web emite `sytue-<producto>://open?url=<https>` —el mismo
gesto que `ms-word:ofe|u|…` de Office Online— y el sistema lo entrega a la app instalada.

**Cada app registra SOLO su esquema.** Si todas declararan `sytue://`, el sistema operativo
elegiria una arbitrariamente y el enlace abriria la que no toca.

Como `ios/` y `android/` los **genera** `cap add` (estan en `.gitignore`), su manifest no se
puede versionar: el registro lo aplica un script **idempotente** con el esquema del producto
activo.

```bash
SYTUE_APP=mail npx cap add android
SYTUE_APP=mail npm run deeplinks    # intent-filter (Android) + CFBundleURLTypes (iOS)
SYTUE_APP=mail npx cap sync
```

La URL entrante llega por `appUrlOpen` (`@capacitor/app`) y la atiende la **Sytue Shell**
(`listenForDeepLinks` en `@sytue/shell`), que navega el webview al documento. **El destino
se valida siempre** contra el dominio de la suite antes de navegar: un `sytue://` lo puede
emitir cualquiera y el sistema operativo no comprueba quien, asi que sin esa verja bastaria
un enlace para llevarse el webview —con la sesion dentro— a un sitio ajeno.

## Configuracion de la URL

Cada app compone su URL contra el **apex** de la suite, derivado de `SYTUE_URL` (mismo
convenio que tenia el escritorio de Wails). Asi el mismo proyecto sirve en produccion y en dev sin
ramificar:

| `SYTUE_URL`                   | Apex              | `SYTUE_APP=drive` carga          |
| ----------------------------- | ----------------- | -------------------------------- |
| _(sin definir)_               | `sytue.eu`        | `https://drive.sytue.eu/`        |
| `https://one.sytue.localhost` | `sytue.localhost` | `https://drive.sytue.localhost/` |

```bash
SYTUE_URL=https://one.sytue.localhost SYTUE_APP=drive npx cap run ios   # dev
```

`server.allowNavigation` lista los hosts que permanecen dentro del webview; cualquier otro
enlace abre en el navegador del sistema, sin arrastrar la sesion.

En dev, instala la CA de mkcert (`mkcert -install`) para que el webview confie en
el TLS local de `*.sytue.localhost`.

## Iconos y splash

Se generan desde los SVG de producto de `@sytue/ui` —los mismos que usan el conmutador de
apps y las apps de escritorio; no se inventan logos—. `scripts/gen-icons.sh` produce las
**fuentes** del producto activo en `resources/`:

- `resources/icon.png` (1024) — marca centrada sobre grafito a sangre (maskable-safe).
- `resources/splash.png` / `splash-dark.png` (2732) — marca sobre marfil / grafito.

```bash
SYTUE_APP=mail bash scripts/gen-icons.sh   # icono y splash de Sytue Mail
SYTUE_APP=mail npm run assets              # assets nativos por plataforma
```

`resources/` guarda los del **ultimo producto generado**: es un artefacto de build, no una
fuente por app. Regeneralo al cambiar de `SYTUE_APP`.

## Estructura

```
apps/mobile/suite/
├── capacitor.config.ts   # appId eu.sytue.mobile, server.url (SYTUE_URL), allowNavigation
├── package.json          # deps @capacitor/* (fuera del workspace Bun)
├── www/index.html        # webDir local (splash de arranque; runtime usa server.url)
├── resources/            # fuentes icon/splash de marca (para @capacitor/assets)
├── scripts/gen-icons.sh  # regeneracion de iconos/splash desde la marca
└── (ios/ · android/)     # proyectos nativos GENERADOS por `cap add` (gitignored)
```
