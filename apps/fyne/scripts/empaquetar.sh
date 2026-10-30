#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# EMPAQUETA apps/fyne PARA LOS CINCO DESTINOS: macOS, Linux, Windows, iOS y Android
# (ADR 0048). Una sola herramienta —`fyne package`— y una sola tabla: la de
# directorios que hay bajo `apps/fyne/cmd/`.
#
#   bash apps/fyne/scripts/empaquetar.sh linux            # todas las apps
#   bash apps/fyne/scripts/empaquetar.sh android demo     # una
#   bash apps/fyne/scripts/empaquetar.sh iossimulator rooms  # .app para el Simulador, sin firma
#   bash apps/fyne/scripts/empaquetar.sh macappstore calcus  # .pkg para la Mac App Store
#
# En ESCRITORIO lo que sale son BUNDLES, un instalador con varias apps (ver «LOS
# BUNDLES DE ESCRITORIO» más abajo): `dist/sytue-<bundle>-linux-amd64.tar.xz` (con su
# `instalar.sh`), `…-windows-amd64-installer.exe` (NSIS) y `…-macos-universal.dmg`. En
# el MÓVIL, un paquete por app: `sytue-<app>-android-sin-firmar.aab` (Google Play; lo
# firma `release-mobile.yml` con la clave de subida, ver «Android: Play App Signing» en
# docs/operations.md) y el de iOS.
#
# # QUÉ APPS EXISTEN NO LO DICE ESTE GUION
#
# Lo dice el árbol: se empaqueta todo `apps/fyne/cmd/*/` que tenga su
# `FyneApp.toml` al lado. Ese fichero es el formato propio de Fyne —nombre,
# identificador de paquete, icono— y lo lee `fyne package` él mismo, así que aquí
# NO hay una segunda tabla de productos que mantener sincronizada. El escritorio y
# el móvil de Gio ya divergieron por tener dos; no se repite.
#
# Cuando aterricen las apps de verdad, cada una trae su `cmd/<app>/FyneApp.toml` y
# esta cadena las recoge sin tocar una línea.
#
# # POR QUÉ `--executable` EN ESCRITORIO Y NO EN MÓVIL
#
# En Linux, Windows y macOS el binario lo compila ESTE guion con su `go build`, y
# `fyne package` solo lo envuelve (tar.xz con su .desktop, .exe con su icono,
# bundle .app con su Info.plist). Es deliberado: los runners propios son ARM64 y lo
# que se publica es amd64, así que el compilador cruzado, el `PKG_CONFIG_LIBDIR` y
# las etiquetas de build tienen que estar en nuestras manos — cuatro versiones se
# publicaron con la arquitectura del runner sin que nada saliera en rojo.
#
# En Android e iOS no se puede: el empaquetador compila él una biblioteca por ABI y
# la mete en el paquete. Ahí se le deja compilar y se verifica lo que salga —con
# `--target android/arm64`, o mete las cuatro ABI y el APK se multiplica por cuatro.
#
# # LO QUE ESTE GUION NO HACE
#
# Firmar. En Android firma `release-mobile.yml` con jarsigner y la clave de SUBIDA
# (Play App Signing: la de la app la guarda Google). En macOS firma los .app si le dan
# `SYTUE_MACOS_IDENTIDAD` (Developer ID, runtime endurecido), pero el .dmg lo firma, lo
# notariza y lo verifica `apple-firma.sh`, que es quien trae el certificado en Codemagic.
# Cada destino lo dice con su nombre y su precio cuando le toca, en vez de fallar
# con un «command not found» dentro de la herramienta.
#
# # LA MAC APP STORE ES OTRA VARIANTE DE macOS, NO OTRO DESTINO
#
# `macappstore` compila lo mismo que `macos` —universal, mismo Info.plist— con la etiqueta
# `tienda` (sin actualizador propio, sesión en el llavero de protección de datos) y, en vez
# de un .dmg por bundle firmado con Developer ID, saca UN `.pkg` POR APP, que es lo que
# pide la tienda: el .app con App Sandbox y sus derechos mínimos (`derechos_tienda`), el
# perfil de Mac App Store embebido, firmado con el certificado de distribución
# (`SYTUE_MAS_IDENTIDAD`), y el .pkg con el de instalador (`SYTUE_MAS_INSTALADOR`). Los dos
# los deja en el llavero `apple-firma.sh preparar macappstore`. Ver «Mac App Store» en
# docs/operations.md. `fyne release --target darwin` no sirve aquí: firma con una plantilla
# de derechos fija, sin perfil embebido, y antes de que se pueda escribir el esquema.
set -euo pipefail

AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FYNE="$(cd "$AQUI/.." && pwd)"
RAIZ="$(cd "$FYNE/../.." && pwd)"
SALIDA="$FYNE/dist"

plataforma="${1:-}"
case "$plataforma" in
linux | windows | macos | macappstore | android | ios) shift ;;
# `iossimulator`: un .app para el Simulador de iOS SIN cuenta de Apple, para barridos y
# pruebas de deep link. No se publica: firma ad hoc y sin perfil.
iossimulator) shift ;;
# `empaquetar.sh plist-simulador <app>` imprime el Info.plist base de ese .app: el test
# lo valida en cualquier sistema sin compilar nada.
plist-simulador) shift ;;
# Expone la copia de metadatos canónicos para probar el plist efectivo sin un Mac.
cifrado-apple) shift ;;
# `empaquetar.sh verificar <artefacto> <goarch>` expone SOLO el guardián de
# arquitectura. Lo usa `empaquetar-test.sh`, que así prueba ESTE guion y no una
# copia suya; y sirve para auditar a mano un artefacto ya descargado.
verificar) shift ;;
verificar-permisos-apple) shift ;;
# `empaquetar.sh verificar-pie <ejecutable|.tar.xz>`: el guardián de PIE de Linux (FYP-06).
verificar-pie) shift ;;
# `empaquetar.sh tipos <app>` imprime los tipos MIME que esa app declara al sistema
# para «Abrir con». Por lo mismo: el test prueba la tabla de aquí, no una copia.
tipos) shift ;;
# `empaquetar.sh documentos-mac <app>` imprime las órdenes de PlistBuddy con las que el
# .app declara sus tipos de documento, y `registro-windows <app>`, las líneas de NSIS de
# «Abrir con» y «Aplicaciones predeterminadas»: el test mira las de aquí sin un Mac ni NSIS.
documentos-mac | registro-windows | mesa-windows) shift ;;
# `empaquetar.sh paquetes` imprime los bundles de escritorio, uno por línea, con su
# clave y sus apps; `empaquetar.sh publicables <plataforma>`, los instaladores que se
# publican de ella. Los usan el test, los workflows y `traer-instaladores.sh`, para
# que nadie repita la lista.
paquetes | publicables) shift ;;
# `empaquetar.sh derechos-tienda <app> <equipo>` imprime los derechos (entitlements) con
# los que esa app se firma para la Mac App Store: el test los mira sin un Mac.
derechos-tienda | derechos-sesion) shift ;;
# `empaquetar.sh widgets-apple` imprime «si» o «no»: el interruptor de WidgetKit y del App
# Group de la suite (SYTUE_WIDGETS_APPLE de apps/fyne/xcode/Sytue.xcconfig). Lo pregunta
# release-desktop.yml antes de traer perfiles de widgets y Developer ID.
widgets-apple) shift ;;
# `empaquetar.sh derechos-archivos <app> <equipo> <ios|macos|app-ios>` imprime los de la
# extensión File Provider de esa app (`archivos = "si"`) y, con app-ios, los de la app en
# iOS. Los escribe generar.py en el proyecto de Xcode.
derechos-archivos) shift ;;
# `empaquetar.sh version-code <vX.Y.Z>` imprime el versionCode de Android de ese tag.
version-code) shift ;;
# `empaquetar.sh instalar-sh "<bundle>"` imprime el `instalar.sh` del bundle de Linux:
# el test lo ejecuta de verdad contra un bundle de mentira.
instalar-sh) shift ;;
# `empaquetar.sh xcode` es la fase de compilación de cada target de
# apps/fyne/xcode/Sytue.xcodeproj (Xcode Cloud): lee lo que Xcode exporta y no lleva
# argumentos. Ver «LA FASE DE XCODE» más abajo.
xcode) shift ;;
*)
  echo "uso: $0 {linux|windows|macos|macappstore|android|ios|iossimulator} [app...]" >&2
  echo "     $0 xcode   (solo desde una fase de Sytue.xcodeproj)" >&2
  echo "     $0 derechos-tienda <app|directorio> <equipo de Apple>" >&2
  echo "     $0 derechos-sesion <app|directorio> <equipo de Apple> <ios|developerid>" >&2
  echo "     $0 derechos-archivos <app|directorio> <equipo de Apple> <ios|macos|app-ios>" >&2
  echo "     $0 widgets-apple   (si|no: SYTUE_WIDGETS_APPLE de apps/fyne/xcode/Sytue.xcconfig)" >&2
  echo "     $0 plist-simulador <app>" >&2
  echo "     $0 cifrado-apple <Info.plist canónico> <Info.plist del artefacto>" >&2
  echo "     $0 verificar <artefacto> <goarch>" >&2
  echo "     $0 verificar-pie <ejecutable|.tar.xz>" >&2
  echo "     $0 tipos <app|directorio>" >&2
  echo "     $0 documentos-mac <app|directorio>" >&2
  echo "     $0 registro-windows <app|directorio>" >&2
  echo "     $0 mesa-windows <carpeta>" >&2
  echo "     $0 paquetes" >&2
  echo "     $0 publicables {linux|windows|macos|macappstore|android|android-sin-firmar|ios} [directorio]" >&2
  echo "     $0 version-code <vX.Y.Z>" >&2
  echo "     $0 instalar-sh <nombre del bundle>" >&2
  exit 2
  ;;
esac
# La Mac App Store es macOS con otra firma y otra caja: todo lo que dice `macos` vale.
tienda=""
if [ "$plataforma" = macappstore ]; then
  plataforma=macos
  tienda=si
fi

# --- Metadatos de cada app: UNA sola declaración, en su FyneApp.toml ----------
# `fyne package` lee ese fichero él solo; aquí se leen las cosas que hacen falta
# FUERA de la herramienta: el identificador de paquete, el nombre, el ESQUEMA de URL
# con el que las demás apps llaman a esta y el bundle de escritorio que la lleva.
metadato() { # <dir> <clave> [sección]
  if [ -n "${3:-}" ]; then
    local contenido
    contenido="$(awk -v seccion="$3" '/^[[:space:]]*\[/ { activa = ($0 ~ "^[[:space:]]*\\[" seccion "\\][[:space:]]*$") } activa' "$1/FyneApp.toml")"
    grep -qE "^[[:space:]]*$2[[:space:]]*=[[:space:]]*\"" <<<"$contenido" || return 1
    sed -n "s/^[[:space:]]*$2[[:space:]]*=[[:space:]]*\"\([^\"]*\)\".*/\1/p" <<<"$contenido" | head -1
  else
    sed -n "s/^[[:space:]]*$2[[:space:]]*=[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$1/FyneApp.toml" | head -1
  fi
}

# --- LOS BUNDLES DE ESCRITORIO --------------------------------------------------
# En macOS, Windows y Linux no se publica una app por instalador sino un instalador
# por BUNDLE, con varias apps dentro: «Sytue One» (el Hub y las de trabajo) y «Sytue
# Drive». Cada app sigue siendo su propio programa, con su icono, su esquema y sus
# tipos de fichero; el bundle es la caja. En el móvil no hay bundles: una app por
# producto, como piden las tiendas.
#
# Qué app va en qué bundle lo dice su FyneApp.toml (`bundle`), no una tabla de aquí.
# Una app sin él va sola, en un bundle con su nombre: así `empaquetar.sh linux demo`
# sigue sirviendo para probar, y el test exige que las de la suite lo declaren.
bundle_de() { # <dir de la app> -> clave del bundle
  local b
  b="$(metadato "$1" bundle)"
  printf '%s' "${b:-$(basename "$1")}"
}

# El nombre sale de la clave («one» → «Sytue One»): es el de la carpeta de Windows,
# el volumen del DMG y la entrada de «Aplicaciones instaladas».
nombre_bundle() { # <clave>
  printf 'Sytue %s%s' "$(printf '%s' "${1:0:1}" | tr '[:lower:]' '[:upper:]')" "${1:1}"
}

# «<clave> <app> <app>…», un bundle por línea, en orden de clave y de app.
paquetes() {
  local t d
  for t in "$FYNE"/cmd/*/FyneApp.toml; do
    [ -f "$t" ] || continue
    d="$(dirname "$t")"
    printf '%s %s\n' "$(bundle_de "$d")" "$(basename "$d")"
  done | sort -s -k1,1 | awk '$1 != b { if (b != "") print l; b = $1; l = $1 } { l = l " " $2 } END { if (b != "") print l }'
}

sufijo() { # <plataforma> -> lo que va detrás de `sytue-<bundle>-`
  case "$1" in
  linux) printf 'linux-amd64.tar.xz' ;;
  windows) printf 'windows-amd64-installer.exe' ;;
  macos) printf 'macos-universal.dmg' ;;
  # En Android no hay bundles: `<bundle>` es la app. Lo que sale de aquí va SIN firmar
  # y release-mobile.yml lo publica firmado con la clave de subida, con el otro nombre.
  android) printf 'android.aab' ;;
  android-sin-firmar) printf 'android-sin-firmar.aab' ;;
  # iOS, como Android: uno por app. Va a App Store Connect, no a la release.
  ios) printf 'ios.ipa' ;;
  # La Mac App Store, igual: un .pkg por app, a App Store Connect.
  macappstore) printf 'macos-tienda.pkg' ;;
  esac
}

# --- LOS DERECHOS DE SANDBOX DE LA MAC APP STORE --------------------------------
# La tienda exige App Sandbox, y dentro de él una app solo puede lo que su firma declara.
# Aquí está TODO lo que se declara y por qué; lo que no está, no se pide (la revisión de
# Apple pregunta por cada derecho, y `empaquetar-test.sh` es rojo si aparece uno de más):
#
#   - app-sandbox: lo exige la tienda.
#   - application-identifier y team-identifier: tienen que casar con el perfil embebido.
#   - network.client: toda la suite habla con sus servicios por HTTPS.
#   - network.server: el login vuelve del navegador a un puerto del propio equipo
#     (127.0.0.1:8921, RFC 8252 §7.3, apps/nucleo/auth/oidc.go) y escuchar, aunque sea en
#     loopback, es de este derecho. En Rooms y Cast lo usa además WebRTC.
#   - files.downloads.read-write: las quince guardan lo que se baja en Descargas
#     (`ficheros.Carpeta`), que dentro del sandbox es un enlace a la de verdad.
#   - application-groups y keychain-access-groups, las dos con `<equipo>.eu.sytue.suite`:
#     el tema, el idioma y la marca de sesión (apps/nucleo/ajustes) y la sesión misma
#     (apps/nucleo/auth) son de la SUITE, no de cada app. Sin grupo, quince contenedores.
#   - files.user-selected.read-write: solo las que abren ficheros que la persona elige
#     (`ficheros = "si"` en su FyneApp.toml).
#   - device.audio-input y device.camera: solo las que tienen llamadas (`llamadas`).
#   - un SEGUNDO keychain-access-group, `<equipo>.<App ID>.shared`: solo las que llevan
#     la extensión File Provider (`archivos = "si"`, Drive). Ahí deja la app la contraseña
#     de aplicación con la que la extensión habla WebDAV, y es un grupo aparte para que la
#     extensión NO lea la sesión de la suite (apps/nucleo/sistema/archivos.go).
#
# Compartir la pantalla (Rooms, Cast) no lleva derecho: lo concede la persona en Ajustes
# del Sistema. Imprimir tampoco: se abre el documento en su visor (`ficheros.Imprimir`).
# EL INTERRUPTOR DE LOS WIDGETS DE APPLE: SYTUE_WIDGETS_APPLE en apps/fyne/xcode/Sytue.xcconfig
# (YES/NO), el mismo que lee generar.py. Apagado, ningún host pide `group.eu.sytue.suite`
# ni lleva WidgetKit dentro, y Developer ID se firma sin perfil: es la firma de antes de
# los widgets, que no depende de nada registrado a mano en Developer Portal. La variable
# de entorno del mismo nombre (1/0) solo existe para los tests y para ensayar en un Mac;
# release-desktop.yml no la define. Un tag anterior al interruptor sale apagado.
widgets_apple_activos() {
  local valor="${SYTUE_WIDGETS_APPLE:-}"
  [ -n "$valor" ] || valor="$(sed -n 's/^SYTUE_WIDGETS_APPLE = \([A-Z]*\)$/\1/p' "$FYNE/xcode/Sytue.xcconfig" 2>/dev/null)"
  case "$valor" in
  1 | YES) return 0 ;;
  0 | NO | '') return 1 ;;
  *)
    echo "::error::SYTUE_WIDGETS_APPLE vale «$valor»: solo YES/NO en Sytue.xcconfig (o 1/0 en el entorno)" >&2
    exit 1
    ;;
  esac
}
if [ "$plataforma" = widgets-apple ]; then
  if widgets_apple_activos; then echo si; else echo no; fi
  exit 0
fi

# El código y las firmas leen los mismos grupos. Un equipo distinto no debe
# generar un paquete aparentemente válido que luego no puede abrir la sesión.
grupos_sesion_apple() { # [equipo esperado]
  local fuente="$RAIZ/apps/nucleo/ajustes/grupo_apple.go"
  GRUPO_LLAVERO_SUITE="$(sed -n 's/^const GrupoSuite = "\([A-Za-z0-9.]*\)"$/\1/p' "$fuente")"
  GRUPO_CONTENEDOR_SUITE="$(sed -n 's/^const GrupoContenedorSuite = "\(group\.[a-z0-9.]*\)"$/\1/p' "$fuente")"
  local equipo="${1:-${GRUPO_LLAVERO_SUITE%%.*}}"
  if ! [[ "$GRUPO_LLAVERO_SUITE" =~ ^[A-Z0-9]{10}\.[a-z0-9.]+$ && "$GRUPO_CONTENEDOR_SUITE" =~ ^group\.[a-z0-9.]+$ ]] || [ "${GRUPO_LLAVERO_SUITE%%.*}" != "$equipo" ]; then
    echo '::error::los grupos o el equipo de Apple no coinciden con ajustes/grupo_apple.go' >&2
    return 1
  fi
}

derechos_tienda() { # <dir de la app> <equipo de Apple (Team ID)>
  local appid equipo="${2:-}" clave
  appid="$(metadato "$1" ID)"
  [ -n "$appid" ] && [[ "$equipo" =~ ^[A-Z0-9]{10}$ ]] || {
    echo "::error::derechos de tienda: $1 no declara ID o «$equipo» no es un Team ID de Apple" >&2
    return 1
  }
  grupos_sesion_apple "$equipo" || return 1
  printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0"><dict>\n'
  printf '\t<key>com.apple.application-identifier</key><string>%s.%s</string>\n' "$equipo" "$appid"
  printf '\t<key>com.apple.developer.team-identifier</key><string>%s</string>\n' "$equipo"
  for clave in app-sandbox network.client network.server files.downloads.read-write; do
    printf '\t<key>com.apple.security.%s</key><true/>\n' "$clave"
  done
  # Preferencias y estado de acceso de Drive son de la suite, también sin widgets.
  # Se conserva el grupo legado únicamente para migrar las preferencias de MAS.
  printf '\t<key>com.apple.security.application-groups</key><array><string>%s</string><string>%s</string>' "$GRUPO_CONTENEDOR_SUITE" "$GRUPO_LLAVERO_SUITE"
  [ "$(metadato "$1" archivos)" != si ] || printf '<string>group.%s</string>' "$appid"
  printf '</array>\n'
  if [ "$(metadato "$1" archivos)" = si ]; then
    printf '\t<key>keychain-access-groups</key><array><string>%s</string><string>%s.%s.shared</string></array>\n' "$GRUPO_LLAVERO_SUITE" "$equipo" "$appid"
  else
    printf '\t<key>keychain-access-groups</key><array><string>%s</string></array>\n' "$GRUPO_LLAVERO_SUITE"
  fi
  [ "$(metadato "$1" ficheros)" != si ] ||
    printf '\t<key>com.apple.security.files.user-selected.read-write</key><true/>\n'
  [ "$(metadato "$1" llamadas)" != si ] ||
    printf '\t<key>com.apple.security.device.audio-input</key><true/>\n\t<key>com.apple.security.device.camera</key><true/>\n'
  printf '</dict></plist>\n'
}
if [ "$plataforma" = derechos-tienda ]; then
  d="${1:?uso: $0 derechos-tienda <app|directorio> <equipo de Apple>}"
  [ -d "$d" ] || d="$FYNE/cmd/$1"
  derechos_tienda "$d" "${2:-}"
  exit $?
fi

# Los hosts móviles y Developer ID comparten la sesión. Las extensiones de
# archivos y widgets conservan sus propios derechos, sin este grupo de Keychain.
derechos_sesion() { # <dir> <equipo> <ios|developerid>
  local appid equipo="${2:-}" modo="${3:-}"
  appid="$(metadato "$1" ID)"
  [[ "$appid" =~ ^eu\.sytue\.[a-z]+$ ]] && [[ "$equipo" =~ ^[A-Z0-9]{10}$ ]] || return 1
  case "$modo" in ios | developerid) ;; *) return 1 ;; esac
  grupos_sesion_apple "$equipo" || return 1
  printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0"><dict>\n'
  if [ "$modo" = developerid ]; then
    printf '\t<key>com.apple.application-identifier</key><string>%s.%s</string>\n' "$equipo" "$appid"
    printf '\t<key>com.apple.developer.team-identifier</key><string>%s</string>\n' "$equipo"
    [ "$(metadato "$1" llamadas)" != si ] || printf '\t<key>com.apple.security.device.audio-input</key><true/>\n\t<key>com.apple.security.device.camera</key><true/>\n'
  fi
  # La marca compartida de acceso al proveedor existe independientemente de WidgetKit.
  printf '\t<key>com.apple.security.application-groups</key><array><string>%s</string>' "$GRUPO_CONTENEDOR_SUITE"
  [ "$(metadato "$1" archivos)" != si ] || printf '<string>group.%s</string>' "$appid"
  printf '</array>\n'
  printf '\t<key>keychain-access-groups</key><array><string>%s.%s</string><string>%s</string>' "$equipo" "$appid" "$GRUPO_LLAVERO_SUITE"
  [ "$(metadato "$1" archivos)" != si ] || printf '<string>%s.%s.shared</string>' "$equipo" "$appid"
  printf '</array>\n</dict></plist>\n'
}
if [ "$plataforma" = derechos-sesion ]; then
  d="${1:?uso: $0 derechos-sesion <app|directorio> <equipo de Apple> <ios|developerid>}"
  [ -d "$d" ] || d="$FYNE/cmd/$1"
  derechos_sesion "$d" "${2:-}" "${3:-}"
  exit $?
fi

# --- LOS DERECHOS DE LA EXTENSIÓN FILE PROVIDER (apps/fyne/xcode/proveedor-archivos) ---
# Sytue Drive en el Finder y en Archivos. La extensión solo necesita la red y el grupo de
# llavero `<equipo>.<App ID>.shared`, donde la app le deja la credencial WebDAV; el grupo
# de apps registrado (`group.<App ID>`) es el de NSExtensionFileProviderDocumentGroup
# en ambos sistemas; el perfil de la extensión debe autorizarlo.
#   - macos: la extensión del Mac, con App Sandbox (la tienda lo exige) y network.client.
#   - ios: la extensión de iOS.
#   - app-ios: la APP en iOS. El primer grupo es su propio App ID, que es donde iOS guarda
#     sin grupo explícito: así la sesión sigue donde estaba (apps/nucleo/auth/store_apple.c).
derechos_archivos() { # <dir de la app> <equipo> <ios|macos|app-ios>
  local appid equipo="${2:-}"
  appid="$(metadato "$1" ID)"
  [ -n "$appid" ] && [[ "$equipo" =~ ^[A-Z0-9]{10}$ ]] && [ "$(metadato "$1" archivos)" = si ] || {
    echo "::error::derechos de archivos: $1 no declara ID ni archivos = \"si\", o «$equipo» no es un Team ID" >&2
    return 1
  }
  printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0"><dict>\n'
  case "$3" in
  macos)
    printf '\t<key>com.apple.application-identifier</key><string>%s.%s.archivos</string>\n\t<key>com.apple.developer.team-identifier</key><string>%s</string>\n' "$equipo" "$appid" "$equipo"
    printf '\t<key>com.apple.security.app-sandbox</key><true/>\n\t<key>com.apple.security.network.client</key><true/>\n'
    grupos_sesion_apple "$equipo" || return 1
    printf '\t<key>com.apple.security.application-groups</key><array><string>group.%s</string><string>%s</string></array>\n' "$appid" "$GRUPO_CONTENEDOR_SUITE"
    printf '\t<key>keychain-access-groups</key><array><string>%s.%s.shared</string></array>\n' "$equipo" "$appid"
    ;;
  ios)
    printf '\t<key>application-identifier</key><string>%s.%s.archivos</string>\n\t<key>com.apple.developer.team-identifier</key><string>%s</string>\n' "$equipo" "$appid" "$equipo"
    printf '\t<key>com.apple.developer.default-data-protection</key><string>NSFileProtectionComplete</string>\n'
    grupos_sesion_apple "$equipo" || return 1
    printf '\t<key>com.apple.security.application-groups</key><array><string>group.%s</string><string>%s</string></array>\n' "$appid" "$GRUPO_CONTENEDOR_SUITE"
    printf '\t<key>keychain-access-groups</key><array><string>%s.%s.shared</string></array>\n' "$equipo" "$appid"
    ;;
  app-ios)
    printf '\t<key>keychain-access-groups</key><array><string>%s.%s</string><string>%s.%s.shared</string></array>\n' "$equipo" "$appid" "$equipo" "$appid"
    ;;
  *)
    echo "::error::derechos de archivos: plataforma «$3» (ios, macos o app-ios)" >&2
    return 1
    ;;
  esac
  printf '</dict></plist>\n'
}
if [ "$plataforma" = derechos-archivos ]; then
  d="${1:?uso: $0 derechos-archivos <app|directorio> <equipo de Apple> <ios|macos|app-ios>}"
  [ -d "$d" ] || d="$FYNE/cmd/$1"
  derechos_archivos "$d" "${2:-}" "${3:-}"
  exit $?
fi

# Las apps móviles publicables, una por línea: todo `cmd/<app>/FyneApp.toml` salvo
# la demo. La lista la da el árbol, como en el resto de este guion.
apps_moviles() {
  local t
  for t in "$FYNE"/cmd/*/FyneApp.toml; do
    [ -f "$t" ] || continue
    t="$(basename "$(dirname "$t")")"
    [ "$t" = demo ] || printf '%s\n' "$t"
  done
}

# EL versionCode DE ANDROID SALE DEL TAG, no del número de ejecución del CI:
#
#   vX.Y.Z  ->  X·1 000 000 + Y·1 000 + Z        (v3.6.23 -> 3006023)
#
# Play exige que crezca en cada subida y no deja reutilizar ninguno. El número de
# ejecución crece, pero es de CADA workflow y se reinicia si el workflow cambia de
# nombre; el tag es el mismo en Codemagic, en git.gay y en Play Console, y se lee a
# simple vista. Sin colisiones mientras Y y Z quepan en tres cifras —se exige, y es
# rojo si no— y X no pase de 2099 (el tope de Play es 2 100 000 000). Un parche de
# una rama vieja (v3.5.9 después de v3.6.0) sale MENOR y Play lo rechaza: es lo
# correcto, a Play solo sube la línea más nueva.
version_code() { # <vX.Y.Z | X.Y.Z>
  local v="${1#v}" x y z
  [[ "$v" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || {
    echo "::error::«$1» no es vX.Y.Z: no hay versionCode de Android sin un tag de release" >&2
    return 1
  }
  x=$((10#${BASH_REMATCH[1]})) y=$((10#${BASH_REMATCH[2]})) z=$((10#${BASH_REMATCH[3]}))
  [ "$x" -ge 1 ] && [ "$x" -le 2099 ] && [ "$y" -le 999 ] && [ "$z" -le 999 ] || {
    echo "::error::$1 no cabe en el versionCode (X de 1 a 2099, Y y Z hasta 999): cambia la fórmula en empaquetar.sh y en docs/operations.md" >&2
    return 1
  }
  printf '%d\n' $((x * 1000000 + y * 1000 + z))
}

# El instalador del bundle de Linux. POSIX sh y sin `make`, que no viene en muchos
# escritorios: copia cada programa, escribe su entrada del menú con rutas completas
# —`~/.local/bin` no está en el PATH de todos— y avisa al escritorio para que se
# entere de los esquemas y los tipos sin cerrar sesión.
instalar_sh() { # <nombre del bundle>
  printf '#!/bin/sh\n# %s para Linux: instala sus programas, cada uno con su entrada en el menú\n' "$1"
  cat <<'SH'
# de aplicaciones, su icono, su esquema de URL —con el que las apps se abren entre sí—
# y los tipos de fichero que abre.
#
#   ./instalar.sh            para ti, en ~/.local (sin contraseña)
#   sudo ./instalar.sh       para todo el equipo, en /usr/local
#   ./instalar.sh --quitar   lo desinstala (con sudo si se instaló con sudo)
#
# PREFIJO=/otra/carpeta lo pone en otra parte. El Hub pone al día los programas donde
# estén, si la carpeta es de quien lo usa.
set -eu
aqui=$(cd "$(dirname "$0")" && pwd)
if [ "$(id -u)" = 0 ]; then p=${PREFIJO:-/usr/local}; else p=${PREFIJO:-$HOME/.local}; fi
case "${1:-}" in
"") quitar= ;;
--quitar) quitar=si ;;
*) echo "uso: $0 [--quitar]" >&2; exit 2 ;;
esac
case "$p" in *[[:space:]]*)
  echo "instalar.sh: «$p» tiene espacios y el menú de aplicaciones no sabría abrir los programas" >&2
  exit 1 ;;
esac
# LAS APPS POR DEFECTO para sus documentos (decisión del dueño, 25/09/2026): Calcus para
# .ods/.xlsx, Docus para .odt/.docx, Diapos para .odp/.pptx y CertiPDF para .pdf. Lo que
# había antes se apunta y `--quitar` lo devuelve, pero solo si sigue siendo nuestro: si
# el usuario ya lo cambió, no se le pisa. Una segunda instalación (una puesta al día) no lo
# vuelve a imponer. Con sudo no: `xdg-mime` escribe en el mimeapps.list de quien lo
# ejecuta, que sería root y no quien instala.
#
# Tres fases, por el orden en que las necesita el sistema: lo de antes se APUNTA antes de
# instalar los .desktop (con el nuestro ya puesto, `xdg-mime query` contesta con él aunque
# nadie lo haya elegido), se IMPONE después, y al quitar se DEVUELVE antes de borrarlos.
# «Sigue siendo nuestro» se lee en el mimeapps.list y no con `xdg-mime query`, que durante
# el desinstalado contestaba vacío (medido en el frontal con xdg-utils 1.1.3).
antes="$p/share/sytue/predeterminadas"
lista="${XDG_CONFIG_HOME:-$HOME/.config}/mimeapps.list"
predeterminar=si
if [ -n "${SUDO_USER:-}" ] || ! command -v xdg-mime >/dev/null 2>&1; then
  predeterminar=
  [ -n "$quitar" ] || echo "Sin xdg-mime o con sudo: las apps salen en «Abrir con», pero no quedan por defecto."
fi
es_nuestro() { # <tipo> <.desktop>: la línea de [Default Applications] es exactamente la nuestra
  [ -f "$lista" ] && awk -v a="$1=$2" -v b="$1=$2;" '
    /^\[/ { dentro = ($0 == "[Default Applications]"); next }
    dentro && ($0 == a || $0 == b) { hay = 1 }
    END { exit !hay }' "$lista"
}
predeterminadas() { # apuntar | imponer | devolver
  [ -n "$predeterminar" ] || return 0
  for d in "$aqui"/share/applications/*.desktop; do
    n=$(basename "$d")
    previos="$antes/$n"
    case "$1" in
    apuntar)
      [ ! -f "$previos" ] || continue
      mkdir -p "$antes"
      : >"$previos.nuevo"
      for mime in $(sed -n 's/^MimeType=//p' "$d" | tr ';' ' '); do
        # Los de Apple quedan en «Abrir con» pero no se imponen (decisión del dueño, 15/10/2026).
        case "$mime" in x-scheme-handler/* | application/vnd.apple.*) continue ;; esac
        printf '%s %s\n' "$mime" "$(xdg-mime query default "$mime" </dev/null 2>/dev/null || true)" >>"$previos.nuevo"
      done
      ;;
    imponer)
      [ -f "$previos.nuevo" ] || continue
      while read -r mime _; do xdg-mime default "$n" "$mime" </dev/null || true; done <"$previos.nuevo"
      mv "$previos.nuevo" "$previos"
      ;;
    devolver)
      [ -f "$previos" ] || continue
      while read -r mime previo; do
        es_nuestro "$mime" "$n" || continue
        if [ -n "$previo" ]; then
          xdg-mime default "$previo" "$mime" </dev/null || true
        else
          awk -v a="$mime=$n" -v b="$mime=$n;" '$0 != a && $0 != b' "$lista" >"$lista.sytue" && mv "$lista.sytue" "$lista"
        fi
      done <"$previos"
      rm -f "${previos:?}"
      rmdir "$antes" "$p/share/sytue" 2>/dev/null || true
      ;;
    esac
  done
}
if [ -n "$quitar" ]; then predeterminadas devolver; else predeterminadas apuntar; fi
for exe in "$aqui"/bin/*; do
  n=$(basename "$exe")
  if [ -n "$quitar" ]; then rm -f "$p/bin/$n"; else install -D -m 0755 "$exe" "$p/bin/$n"; fi
done
for d in "$aqui"/share/applications/*.desktop; do
  n=$(basename "$d" .desktop)
  if [ -n "$quitar" ]; then
    rm -f "$p/share/applications/$n.desktop" "$p/share/pixmaps/$n.png"
  else
    install -D -m 0644 "$aqui/share/pixmaps/$n.png" "$p/share/pixmaps/$n.png"
    mkdir -p "$p/share/applications"
    sed -e "s|^Exec=|Exec=$p/bin/|" -e "s|^Icon=.*|Icon=$p/share/pixmaps/$n.png|" "$d" >"$p/share/applications/$n.desktop"
  fi
done
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "$p/share/applications" || true
fi
[ -n "$quitar" ] || predeterminadas imponer
if [ -n "$quitar" ]; then echo "Quitado de $p."; else echo "Instalado en $p."; fi
# DEPENDENCIA OPCIONAL: con wl-clipboard, en Wayland se copian imágenes (`wl-copy`) y se lee
# el portapapeles sin foco (`wl-paste`); sin él las apps funcionan y eso se queda fuera.
if [ -z "$quitar" ] && ! command -v wl-copy >/dev/null 2>&1; then
  echo "Opcional: instala wl-clipboard (apt/dnf/pacman install wl-clipboard) para copiar imágenes en Wayland."
fi
SH
}

# EL .app DEL SIMULADOR SE ARMA A MANO porque `fyne package --target iossimulator` pasa
# por xcodebuild y exige un Team ID (DetectIOSTeamID en build_iosapp.go de fyne.io/tools)
# aunque el Simulador no compruebe ninguna firma. Lo que Fyne pondría y aquí no hace falta:
# el catálogo de iconos y el perfil. El esquema, el micrófono y Face ID los añade después
# el mismo paso que en iOS.
plist_simulador() { # <dir de la app>
  local appid nombre
  appid="$(metadato "$1" ID)"
  nombre="$(metadato "$1" Name)"
  [ -n "$appid" ] && [ -n "$nombre" ] || {
    echo "::error::$1: FyneApp.toml sin ID o sin Name" >&2
    return 1
  }
  cat <<P
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>$appid</string>
<key>CFBundleExecutable</key><string>sytue-$(basename "$1")</string>
<key>CFBundleName</key><string>$nombre</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleVersion</key><string>1</string>
<key>CFBundleShortVersionString</key><string>1.0</string>
<key>CFBundleIconFile</key><string>Icon.png</string>
<key>MinimumOSVersion</key><string>15.0</string>
<key>UIDeviceFamily</key><array><integer>1</integer><integer>2</integer></array>
<key>LSRequiresIPhoneOS</key><true/>
<key>LSSupportsOpeningDocumentsInPlace</key><true/>
<key>UIFileSharingEnabled</key><true/>
<key>UIRequiresFullScreen</key><true/>
<key>UISupportedInterfaceOrientations</key><array><string>UIInterfaceOrientationPortrait</string><string>UIInterfaceOrientationLandscapeLeft</string><string>UIInterfaceOrientationLandscapeRight</string></array>
</dict></plist>
P
}
if [ "$plataforma" = plist-simulador ]; then
  plist_simulador "$FYNE/cmd/${1:?uso: $0 plist-simulador <app>}"
  exit $?
fi
if [ "$plataforma" = instalar-sh ]; then
  instalar_sh "${1:?uso: $0 instalar-sh <nombre del bundle>}"
  exit 0
fi
if [ "$plataforma" = paquetes ]; then
  paquetes
  exit 0
fi
if [ "$plataforma" = version-code ]; then
  version_code "${1:?uso: $0 version-code <vX.Y.Z>}"
  exit $?
fi
if [ "$plataforma" = publicables ]; then
  case "${1:-}" in linux | windows | macos | macappstore | android | android-sin-firmar | ios) ;; *)
    echo "uso: $0 publicables {linux|windows|macos|macappstore|android|android-sin-firmar|ios} [directorio]" >&2
    exit 2
    ;;
  esac
  # Sin la demo, que no es un producto. Con directorio, las rutas, y ROJO si falta
  # alguno: publicar una release a la que le falta un bundle es lo que no se ve
  # hasta que alguien va a bajarlo.
  faltan=0
  # La lista en una variable y no un `case` dentro de `<(…)`: el bash 3.2 del Mac de
  # Codemagic toma el `)` del patrón por el cierre y el guion entero no se lee
  # («syntax error near unexpected token `apps_moviles'», v3.6.24, build
  # 6ab6cbfb40438575afb17ba6). Lo vigila empaquetar-test.sh.
  lista=paquetes
  case "$1" in android* | ios | macappstore) lista=apps_moviles ;; esac
  while read -r b _; do
    [ "$b" = demo ] && continue
    f="sytue-$b-$(sufijo "$1")"
    if [ -n "${2:-}" ]; then
      [ -f "$2/$f" ] || {
        echo "::error::falta $2/$f: $b no se ha empaquetado" >&2
        faltan=1
        continue
      }
      f="$2/$f"
    fi
    printf '%s\n' "$f"
  done < <("$lista")
  exit "$faltan"
fi

# --- «ABRIR CON»: de extensión a tipo MIME -------------------------------------
# Cada app declara en su FyneApp.toml las EXTENSIONES que abre (`extensiones`, que
# `TestCadaAppDeclaraLasExtensionesDeSuProducto` ata a lo que abre la web). Linux y
# Android las quieren como tipo MIME y Windows como extensión, así que la tabla que
# falta es esta, y es un hecho del mundo, no una decisión nuestra.
#
# Una extensión que no está aquí FALLA el empaquetado, no se salta: declarar un tipo
# a medias deja la app fuera de «Abrir con» sin que nadie se entere.
tipo_mime() { # <extensión> -> tipo MIME
  case "$1" in
  docx) printf 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' ;;
  dotx) printf 'application/vnd.openxmlformats-officedocument.wordprocessingml.template' ;;
  xlsx) printf 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' ;;
  xltx) printf 'application/vnd.openxmlformats-officedocument.spreadsheetml.template' ;;
  pptx) printf 'application/vnd.openxmlformats-officedocument.presentationml.presentation' ;;
  potx) printf 'application/vnd.openxmlformats-officedocument.presentationml.template' ;;
  odt) printf 'application/vnd.oasis.opendocument.text' ;;
  ott) printf 'application/vnd.oasis.opendocument.text-template' ;;
  ods) printf 'application/vnd.oasis.opendocument.spreadsheet' ;;
  ots) printf 'application/vnd.oasis.opendocument.spreadsheet-template' ;;
  odp) printf 'application/vnd.oasis.opendocument.presentation' ;;
  otp) printf 'application/vnd.oasis.opendocument.presentation-template' ;;
  pages) printf 'application/vnd.apple.pages' ;;
  numbers) printf 'application/vnd.apple.numbers' ;;
  key) printf 'application/vnd.apple.keynote' ;;
  pdf) printf 'application/pdf' ;;
  *)
    echo "::error::no sé el tipo MIME de .$1: añádelo a tipo_mime en empaquetar.sh" >&2
    return 1
    ;;
  esac
}

# Lo mismo para macOS, que no habla de tipos MIME sino de UTI. Los de OOXML y ODF se
# declaran además como importados (`plist_documentos`), por si el Mac no trae ninguna app
# que los defina; el del PDF es del sistema.
uti_de() { # <extensión> -> UTI
  case "$1" in
  docx) printf 'org.openxmlformats.wordprocessingml.document' ;;
  dotx) printf 'org.openxmlformats.wordprocessingml.template' ;;
  xlsx) printf 'org.openxmlformats.spreadsheetml.sheet' ;;
  xltx) printf 'org.openxmlformats.spreadsheetml.template' ;;
  pptx) printf 'org.openxmlformats.presentationml.presentation' ;;
  potx) printf 'org.openxmlformats.presentationml.template' ;;
  odt) printf 'org.oasis-open.opendocument.text' ;;
  ott) printf 'org.oasis-open.opendocument.text-template' ;;
  ods) printf 'org.oasis-open.opendocument.spreadsheet' ;;
  ots) printf 'org.oasis-open.opendocument.spreadsheet-template' ;;
  odp) printf 'org.oasis-open.opendocument.presentation' ;;
  otp) printf 'org.oasis-open.opendocument.presentation-template' ;;
  pages) printf 'com.apple.iwork.pages.sffpages' ;;
  numbers) printf 'com.apple.iwork.numbers.sffnumbers' ;;
  key) printf 'com.apple.iwork.keynote.sffkey' ;;
  pdf) printf 'com.adobe.pdf' ;;
  *)
    echo "::error::no sé el UTI de .$1: añádelo a uti_de en empaquetar.sh" >&2
    return 1
    ;;
  esac
}

extensiones_de() { # <dir de la app> -> extensiones separadas por espacios
  sed -n 's/^[[:space:]]*extensiones[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$1/FyneApp.toml" | head -1 | tr ',' ' '
}

tipos_de() { # <dir de la app> -> un tipo MIME por línea
  local ext
  for ext in $(extensiones_de "$1"); do
    tipo_mime "$ext" || return 1
    printf '\n'
  done
}

# EN macOS, LOS TIPOS DE DOCUMENTO del Info.plist: sin `CFBundleDocumentTypes` el Finder
# no ofrece la app en «Abrir con» ni deja ponerla por defecto. `LSHandlerRank Owner` es
# la que se dice dueña del formato; la primera vez que arranca, la app se pone además por
# defecto (`escritorio.Predeterminada`). El documento llega por el Apple Event
# `kAEOpenDocuments`, que atiende el fork de Fyne. Una orden de PlistBuddy por línea.
plist_documentos() { # <dir de la app>
  local ext uti mime rol rango i=0 j=0
  for ext in $(extensiones_de "$1"); do
    uti="$(uti_de "$ext")" || return 1
    mime="$(tipo_mime "$ext")" || return 1
    # Los de iWork se abren convertidos y nunca se guardan en su formato: la app los LEE
    # y no se queda con ellos en un Mac que tenga Pages, Numbers o Keynote.
    case "$ext" in
    pages | numbers | key) rol=Viewer rango=Alternate ;;
    *) rol=Editor rango=Owner ;;
    esac
    [ "$i" -gt 0 ] || printf 'Add :CFBundleDocumentTypes array\n'
    printf 'Add :CFBundleDocumentTypes:%d dict\n' "$i"
    printf 'Add :CFBundleDocumentTypes:%d:CFBundleTypeName string %s\n' "$i" "$(printf '%s' "$ext" | tr '[:lower:]' '[:upper:]')"
    printf 'Add :CFBundleDocumentTypes:%d:CFBundleTypeRole string %s\n' "$i" "$rol"
    printf 'Add :CFBundleDocumentTypes:%d:LSHandlerRank string %s\n' "$i" "$rango"
    printf 'Add :CFBundleDocumentTypes:%d:LSItemContentTypes array\n' "$i"
    printf 'Add :CFBundleDocumentTypes:%d:LSItemContentTypes:0 string %s\n' "$i" "$uti"
    i=$((i + 1))
    [ "$uti" != com.adobe.pdf ] || continue
    [ "$j" -gt 0 ] || printf 'Add :UTImportedTypeDeclarations array\n'
    printf 'Add :UTImportedTypeDeclarations:%d dict\n' "$j"
    printf 'Add :UTImportedTypeDeclarations:%d:UTTypeIdentifier string %s\n' "$j" "$uti"
    printf 'Add :UTImportedTypeDeclarations:%d:UTTypeConformsTo array\n' "$j"
    printf 'Add :UTImportedTypeDeclarations:%d:UTTypeConformsTo:0 string public.data\n' "$j"
    printf 'Add :UTImportedTypeDeclarations:%d:UTTypeConformsTo:1 string public.composite-content\n' "$j"
    printf 'Add :UTImportedTypeDeclarations:%d:UTTypeTagSpecification dict\n' "$j"
    printf 'Add :UTImportedTypeDeclarations:%d:UTTypeTagSpecification:public.filename-extension array\n' "$j"
    printf 'Add :UTImportedTypeDeclarations:%d:UTTypeTagSpecification:public.filename-extension:0 string %s\n' "$j" "$ext"
    printf 'Add :UTImportedTypeDeclarations:%d:UTTypeTagSpecification:public.mime-type string %s\n' "$j" "$mime"
    j=$((j + 1))
  done
}

# En Windows «Abrir con» es el registro: un ProgID de la app y su nombre en
# `OpenWithProgids` de cada extensión. Y para poder ser la PREDETERMINADA, sus
# `Capabilities\FileAssociations` y su fila en `RegisteredApplications`: es lo que la
# pone en Configuración › Aplicaciones › Aplicaciones predeterminadas. Windows 10 y 11 no
# dejan que un programa se ponga por defecto solo (el hash de UserChoice lo impide a
# propósito), así que la primera vez que arranca la app abre SU página de esa pantalla
# (`escritorio.Predeterminada`) y el usuario pulsa «Establecer predeterminada».
nsis_abrir_con() { # <extensiones> <appid> <nombre> <ejecutable>
  [ -n "$1" ] || return 0
  local progid="$2.documento" ext orden="'\"\$INSTDIR\\$4.exe\" \"%1\"'"
  printf '  ; «ABRIR CON»: la app se ofrece para %s.\n' "$1"
  printf '  WriteRegStr HKCU "Software\\Classes\\%s" "" "%s"\n' "$progid" "$3"
  printf '  WriteRegStr HKCU "Software\\Classes\\%s\\DefaultIcon" "" "$INSTDIR\\%s.exe,0"\n' "$progid" "$4"
  printf '  WriteRegStr HKCU "Software\\Classes\\%s\\shell\\open\\command" "" %s\n' "$progid" "$orden"
  local cap="Software\\Sytue\\$2\\Capabilities"
  printf '  WriteRegStr HKCU "%s" "ApplicationName" "%s"\n' "$cap" "$3"
  printf '  WriteRegStr HKCU "%s" "ApplicationDescription" "%s"\n' "$cap" "$3"
  for ext in $1; do
    printf '  WriteRegStr HKCU "Software\\Classes\\.%s\\OpenWithProgids" "%s" ""\n' "$ext" "$progid"
    printf '  WriteRegStr HKCU "%s\\FileAssociations" ".%s" "%s"\n' "$cap" "$ext" "$progid"
  done
  printf '  WriteRegStr HKCU "Software\\RegisteredApplications" "%s" "%s"\n' "$3" "$cap"
  # SHCNE_ASSOCCHANGED: sin él, el Explorador no se entera hasta reiniciar sesión.
  printf "  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'\n"
}

nsis_quitar_abrir_con() { # <extensiones> <appid> <nombre>
  [ -n "$1" ] || return 0
  local ext
  printf '  DeleteRegKey HKCU "Software\\Classes\\%s.documento"\n' "$2"
  printf '  DeleteRegValue HKCU "Software\\RegisteredApplications" "%s"\n' "$3"
  printf '  DeleteRegKey HKCU "Software\\Sytue\\%s"\n' "$2"
  printf '  DeleteRegKey /ifempty HKCU "Software\\Sytue"\n'
  for ext in $1; do
    printf '  DeleteRegValue HKCU "Software\\Classes\\.%s\\OpenWithProgids" "%s.documento"\n' "$ext" "$2"
  done
}

# OPENGL DE RESERVA EN WINDOWS (Mesa llvmpipe, que pinta con la CPU). Fyne necesita
# OpenGL 2.1 y un Windows sin controlador de la tarjeta —máquina virtual, escritorio
# remoto— solo tiene el 1.1 de «GDI Generic»: sin esto la app no abre (medido en Windows
# Server 2022). Va en `$INSTDIR\mesa` y NUNCA al lado de los .exe: ahí Windows lo
# cargaría antes que el del sistema y el controlador de la tarjeta no se usaría jamás.
# Lo carga solo la app que ve que el del sistema no llega (escritorio.RelanzarConReserva).
# Mismo apartar-y-poner que los .exe: una DLL en uso no se sobrescribe, pero se renombra.
MESA_FICHEROS="opengl32.dll libgallium_wgl.dll LICENCIAS.txt"
nsis_mesa() { # <carpeta con MESA_FICHEROS>
  local f
  printf '  ; OpenGL de reserva (Mesa llvmpipe): en su carpeta, nunca junto a los .exe.\n'
  printf '  SetOutPath "$INSTDIR\\mesa"\n'
  for f in $MESA_FICHEROS; do
    printf '  Delete "$INSTDIR\\mesa\\%s.viejo"\n' "$f"
    printf '  Rename "$INSTDIR\\mesa\\%s" "$INSTDIR\\mesa\\%s.viejo"\n' "$f" "$f"
    printf '  File "%s/%s"\n' "$1" "$f"
    printf '  Delete /REBOOTOK "$INSTDIR\\mesa\\%s.viejo"\n' "$f"
  done
  printf '  SetOutPath "$INSTDIR"\n'
}

nsis_quitar_mesa() {
  local f
  for f in $MESA_FICHEROS; do
    printf '  Delete "$INSTDIR\\mesa\\%s"\n' "$f"
    printf '  Delete "$INSTDIR\\mesa\\%s.viejo"\n' "$f"
  done
  printf '  RMDir "$INSTDIR\\mesa"\n'
}

# preparar_mesa deja en $ESCENARIO/mesa lo que instala nsis_mesa, bajado de la versión
# fijada en versions.env con su sha256 comprobado, y en $ESCENARIO/gl-diferido una
# libopengl32.a de CARGA DIFERIDA. go-gl y GLFW enlazan `-lopengl32`, y con la normal
# Windows carga el opengl32.dll del sistema al arrancar el proceso, antes de que la app
# pueda elegir: con la diferida se carga en la primera llamada, y el proceso relanzado
# con la reserva ya ha dicho de qué carpeta.
preparar_mesa() {
  local v sha f siete lib
  v="$(sed -n 's/^MESA_VERSION=//p' "$RAIZ/deploy/sytue-git/versions.env")"
  sha="$(sed -n 's/^MESA_MINGW_SHA256=//p' "$RAIZ/deploy/sytue-git/versions.env")"
  if [ -z "$v" ] || [ -z "$sha" ]; then
    echo "::error::versions.env no fija MESA_VERSION y MESA_MINGW_SHA256" >&2
    exit 1
  fi
  siete="$(command -v 7z || command -v 7za || command -v 7zz || true)"
  [ -n "$siete" ] || {
    echo "::error::falta 7z (paquete 7zip) para sacar el Mesa de reserva de Windows." >&2
    exit 1
  }
  f="mesa3d-$v-release-mingw.7z"
  bash "$RAIZ/deploy/sytue-git/bin/bajar-herramienta.sh" mesa3d "$v" "$f" "$sha" \
    "https://github.com/pal1000/mesa-dist-win/releases/download/$v/$f" "$ESCENARIO/$f" >&2
  "$siete" e -y -o"$ESCENARIO/mesa" "$ESCENARIO/$f" x64/opengl32.dll x64/libgallium_wgl.dll >/dev/null
  cat "$AQUI/mesa-licencias.txt" /usr/share/common-licenses/Apache-2.0 >"$ESCENARIO/mesa/LICENCIAS.txt"
  [ -z "$FIRMAR_WIN" ] || bash "$FIRMAR_WIN" firmar "$ESCENARIO"/mesa/*.dll
  lib="$("$CC_DESTINO" -print-file-name=libopengl32.a)"
  mkdir -p "$ESCENARIO/gl-diferido"
  {
    echo "LIBRARY opengl32.dll"
    echo EXPORTS
    x86_64-w64-mingw32-nm -g --defined-only "$lib" | awk '$2 == "I" { s = $3; sub(/^__imp_/, "", s); print s }' | sort -u
  } >"$ESCENARIO/gl-diferido/opengl32.def"
  x86_64-w64-mingw32-dlltool --input-def "$ESCENARIO/gl-diferido/opengl32.def" --dllname opengl32.dll \
    --output-delaylib "$ESCENARIO/gl-diferido/libopengl32.a"
  echo "· Mesa $v de reserva: $(du -ch "$ESCENARIO"/mesa/*.dll | tail -1 | cut -f1) sin comprimir" >&2
}

# opengl32_diferida falla si <exe> importa opengl32.dll al arrancar: con esa importación
# el Mesa de reserva no se puede cargar nunca, y la app sin controlador no abriría.
opengl32_diferida() { # <exe>
  local dll_importadas
  dll_importadas="$(x86_64-w64-mingw32-objdump -p "$1" | awk '/^The Import Tables/ { t = 1 } /^The Delay Import/ { t = 0 } t && /DLL Name:/ { print tolower($3) }')"
  if grep -qx opengl32.dll <<<"$dll_importadas"; then
    echo "::error::$1 carga opengl32.dll al arrancar: el Mesa de reserva no podría usarse (¿falta -L gl-diferido?)." >&2
    return 1
  fi
}

# «Abrir con» en Android: un VIEW por tipo MIME sobre `content://`, que es como
# llegan los ficheros desde Android 7 (un `file://` entre apps lanza
# FileUriExposedException). El Intent trae el permiso de lectura de ESE fichero,
# así que no hace falta pedir ninguno de almacenamiento, y el guardián de permisos
# de más abajo lo sigue exigiendo.
# firmar_ipa vuelve a firmar el .app de un IPA ya editado y lo empaqueta en su sitio.
# Los derechos son los que ya traía (los de su perfil de aprovisionamiento) más, en las
# apps dueñas de un host (`enlaces`), `com.apple.developer.associated-domains` con
# `applinks:<host>`: sin él iOS no pide el apple-app-site-association y el enlace de una
# reunión abre Safari. El perfil tiene que traer esa capacidad (Apple Developer >
# Identifiers > Associated Domains): si no, iOS rechaza instalar la app, así que se
# comprueba AQUÍ y no en el teléfono. La identidad es SYTUE_IOS_IDENTIDAD, la misma
# que firmó (`Apple Distribution: …`).
firmar_ipa() { # <dir del ipa abierto> <.app> <ipa de salida> <app id> <host o vacío> <fuentes>
  local dir="$1" app="$2" ipa host="$5" ent perfil sesion equipo
  ipa="$(cd "$(dirname "$3")" && pwd)/$(basename "$3")"
  ent="$dir/derechos.plist"
  codesign -d --entitlements :- "$app" >"$ent" 2>/dev/null || {
    echo "::error::$ipa no trae derechos que leer: ¿salió sin firmar?" >&2
    exit 1
  }
  # La sesión compartida (el Keychain de la suite) desde que existe grupo_apple.go; un tag
  # anterior se firma con lo que trae. El App Group y su comprobación contra el perfil,
  # solo con el interruptor de los widgets encendido (widgets_apple_activos).
  if [ -f "$RAIZ/apps/nucleo/ajustes/grupo_apple.go" ]; then
    grupos_sesion_apple || return 1
    equipo="${GRUPO_LLAVERO_SUITE%%.*}"
    sesion="$dir/sesion.plist"
    derechos_sesion "$6" "$equipo" ios >"$sesion" || return 1
    if [ "$(basename "$6")" != demo ]; then
      local grupo_perfil="$GRUPO_CONTENEDOR_SUITE"
      bash "$AQUI/apple-firma.sh" verificar-perfil-grupo "$app/embedded.mobileprovision" "$4" ios "$grupo_perfil" "" "$GRUPO_LLAVERO_SUITE" "$sesion" || return 1
    fi
    python3 - "$ent" "$sesion" <<'PY' || return 1
import pathlib, plistlib, sys
destino, adicionales = map(pathlib.Path, sys.argv[1:])
derechos = plistlib.loads(destino.read_bytes())
derechos.update(plistlib.loads(adicionales.read_bytes()))
destino.write_bytes(plistlib.dumps(derechos))
PY
    rm -f "$sesion"
  fi
  if [ -n "$host" ]; then
    perfil="$dir/perfil.plist"
    python3 "$AQUI/apple_perfil.py" decodificar "$app/embedded.mobileprovision" ios >"$perfil" 2>/dev/null || {
      echo "::error::$ipa no trae un embedded.mobileprovision legible." >&2
      exit 1
    }
    /usr/libexec/PlistBuddy -c "Print :Entitlements:com.apple.developer.associated-domains" "$perfil" >/dev/null 2>&1 || {
      echo "::error::el perfil de $4 no tiene la capacidad Associated Domains: actívala en Apple Developer y regenera el perfil." >&2
      exit 1
    }
    /usr/libexec/PlistBuddy -c "Delete :com.apple.developer.associated-domains" "$ent" >/dev/null 2>&1 || true
    /usr/libexec/PlistBuddy -c "Add :com.apple.developer.associated-domains array" \
      -c "Add :com.apple.developer.associated-domains:0 string applinks:$host" "$ent"
  fi
  # Los derechos se leen ANTES de reemplazar el ejecutable firmado de la CLI.
  # Fyne tools 1.7.2 ignora --executable en iOS y enlaza con mínimo 7.0; la
  # xcconfig solo arregla el envoltorio Xcode, no ese Mach-O ya compilado.
  compilar_ios_paquete "$6" "$app" "$version" "$build" || return 1
  incorporar_widgets_apple "$app" "$6" ios "${SYTUE_IOS_IDENTIDAD:?}" || return 1
  codesign -f -s "${SYTUE_IOS_IDENTIDAD:?para volver a firmar el IPA hace falta SYTUE_IOS_IDENTIDAD}" \
    --entitlements "$ent" "$app"
  rm -f "$ent" "${perfil:-}"
  # Lo que App Store Connect rechaza al subir, mirado aquí: la firma tiene que cuadrar
  # con lo que se acaba de editar y el perfil tiene que viajar dentro.
  codesign --verify --strict "$app" || {
    echo "::error::la firma de $ipa no verifica tras volver a firmar." >&2
    exit 1
  }
  [ -f "$app/embedded.mobileprovision" ] || {
    echo "::error::$ipa no lleva embedded.mobileprovision: App Store Connect lo rechazaría." >&2
    exit 1
  }
  # SwiftSupport contiene las bibliotecas sin firmar que Apple necesita para
  # validar las extensiones Swift; Payload lleva sus copias firmadas.
  (
    cd "$dir"
    rm -f "$ipa"
    if [ -d SwiftSupport ]; then zip -qry "$ipa" Payload SwiftSupport; else zip -qry "$ipa" Payload; fi
  )
  rm -rf "$dir"
}

# firmar_tienda deja un .app listo para la Mac App Store: el perfil de Mac App Store de
# SU App ID embebido (`Contents/embedded.provisionprofile`), los derechos de sandbox de
# `derechos_tienda` con el equipo que dice el propio perfil, y la firma del certificado de
# distribución (`SYTUE_MAS_IDENTIDAD`). Y lo comprueba aquí: una app sin sandbox o con el
# perfil de otra la rechaza App Store Connect al subirla, cuarenta minutos después.
firmar_tienda() { # <.app> <dir de la app> <app id>
  local app="$1" dir="$2" appid="$3" perfil claro ent equipo firmados
  perfil="$(bash "$AQUI/apple-firma.sh" perfil-mac "$appid")" || exit 1
  echo "   perfil: $perfil"
  claro="$(mktemp)"
  ent="$(mktemp)"
  python3 "$AQUI/apple_perfil.py" decodificar "$perfil" macappstore >"$claro" 2>/dev/null || {
    echo "::error::el perfil de $appid ($perfil) no se deja leer." >&2
    exit 1
  }
  equipo="$(/usr/libexec/PlistBuddy -c 'Print :TeamIdentifier:0' "$claro")"
  [ "$(/usr/libexec/PlistBuddy -c 'Print :Entitlements:com.apple.application-identifier' "$claro")" = "$equipo.$appid" ] || {
    echo "::error::el perfil $perfil no es el de $appid: firmaría con el App ID de otra app." >&2
    exit 1
  }
  derechos_tienda "$dir" "$equipo" >"$ent" || exit 1
  if [ "$(basename "$dir")" != demo ]; then
    grupos_sesion_apple "$equipo" || return 1
    local grupo_perfil="$GRUPO_CONTENEDOR_SUITE"
    bash "$AQUI/apple-firma.sh" verificar-perfil-grupo "$perfil" "$appid" macappstore "$grupo_perfil" "" "$GRUPO_LLAVERO_SUITE" "$ent" || return 1
  fi
  # El perfil del almacén privado es 0600. Dentro del instalador pertenece a
  # root y debe poder leerlo quien ejecuta la app (Apple rechaza 90255 si no).
  install -m 0644 "$perfil" "$app/Contents/embedded.provisionprofile"
  incorporar_widgets_apple "$app" "$dir" macappstore "$SYTUE_MAS_IDENTIDAD" || return 1
  # Atributos extendidos fuera (un com.apple.FinderInfo hace fallar a codesign).
  xattr -cr "$app"
  codesign -f -s "$SYTUE_MAS_IDENTIDAD" --options runtime --timestamp --entitlements "$ent" "$app"
  rm -f "$claro" "$ent"
  codesign --verify --strict "$app" || {
    echo "::error::la firma de $app no verifica." >&2
    exit 1
  }
  verificar_permisos_apple "$app" || exit 1
  # Sin tubería hacia `grep -q` (ci-sin-azar-test.sh).
  firmados="$(codesign -d --entitlements - "$app" 2>/dev/null)"
  grep -q 'com.apple.security.app-sandbox' <<<"$firmados" || {
    echo "::error::$app firmado SIN App Sandbox: la Mac App Store lo rechaza." >&2
    exit 1
  }
}

verificar_permisos_apple() { # <.app>: cada usuario debe poder verificar su firma
  python3 - "$1" <<'PY'
import os, stat, sys
from pathlib import Path
app = Path(sys.argv[1])
if not app.is_dir() or app.is_symlink():
    sys.exit('::error::no existe un bundle Apple que comprobar')
for root, _, files in os.walk(app, followlinks=False):
    for path in [Path(root)] + [Path(root) / name for name in files]:
        if path.is_symlink():
            continue
        mode = path.stat().st_mode
        required = 0o555 if stat.S_ISDIR(mode) else 0o444
        if stat.S_IMODE(mode) & required != required:
            sys.exit('::error::Apple necesita lectura pública: ' + str(path.relative_to(app)))
PY
}

# El servicio en primer plano del Hub, o nada. SIN `android:foregroundServiceType`: el
# codificador de manifiestos de la CLI de Fyne (binres) lleva una tabla de atributos
# anterior a la API 29 y no lo conoce, como `autoVerify`. Lo añade `apk_a_aab` al
# binario (android-binario.py servicio-en-primer-plano), y `verificar_aab` lo exige: con
# target 34 o más, `startForeground` sin él lanza MissingForegroundServiceTypeException.
# La propiedad es la justificación que Android pide a specialUse (y Play lee).
servicio_de_avisos() { # <si|""> -> <service> o nada
  [ -n "$1" ] || return 0
  printf '\n\t<service android:name="eu.sytue.nativo.Avisos"\n\t\tandroid:exported="false">\n'
  printf '\t\t<property android:name="android.app.PROPERTY_SPECIAL_USE_FGS_SUBTYPE"\n'
  printf '\t\t\tandroid:value="Keeps the Sytue Hub notification stream open so suite notifications arrive while the apps are closed, without Firebase" />\n'
  printf '\t</service>'
}

# Drive en «Archivos» y en el selector de cualquier app (clase Documentos, que habla
# WebDAV con la credencial de `documentos.Asegurar`), o nada. `MANAGE_DOCUMENTS` no es un permiso que Drive PIDA: es el
# que Android exige a quien llame al proveedor, y solo lo tiene el propio sistema.
proveedor_documentos() { # <appid> <si|""> -> <provider> o nada
  [ -n "$2" ] || return 0
  printf '\n\t<provider android:name="eu.sytue.nativo.Documentos"\n\t\tandroid:authorities="%s.sytue.documentos"\n' "$1"
  printf '\t\tandroid:exported="true"\n\t\tandroid:grantUriPermissions="true"\n\t\tandroid:permission="android.permission.MANAGE_DOCUMENTS">\n'
  printf '\t\t<intent-filter>\n\t\t\t<action android:name="android.content.action.DOCUMENTS_PROVIDER" />\n\t\t</intent-filter>\n'
  printf '\t</provider>'
}

# Los permisos que el manifiesto PIDE: los `<uses-permission>` del volcado de aapt2, y no
# cualquier `android.permission.*` que aparezca —el proveedor de Drive EXIGE uno a quien
# lo llama (MANAGE_DOCUMENTS), y eso no es pedirlo—.
permisos_pedidos() { # volcado de `aapt2 dump xmltree` por la entrada -> «p1 p2 … »
  awk '/E: uses-permission/ {u=1; next} /E: / {u=0} u' | grep -o 'android\.permission\.[A-Z_]*' | sort -u | tr '\n' ' '
}

filtro_enlaces() { # <host> -> intent-filter de App Links o nada
  [ -n "$1" ] || return 0
  # Solo la raíz y la raíz con prefijo: es donde vive el enlace de una reunión. Todo
  # rooms.<dominio> se llevaría a la app páginas que no sabe abrir.
  # SIN `android:autoVerify`: el codificador de manifiestos de la CLI de Fyne (binres)
  # lleva una tabla de atributos anterior a la API 23 y muere con «failed to find table
  # ref by "attr/autoVerify"» (v3.6.25, build 6ab6e5eacd5b284e6a804327: las trece apps
  # salían y Rooms tumbaba el AAB de todas). Hoy no cambia nada: sin `assetlinks.json`
  # publicado no hay verificación que hacer (docs/operations.md, «App Links»). El día
  # que se publique, el atributo se añade después, en el binario (android-binario.py).
  printf '\n\t\t<intent-filter>\n\t\t\t<action android:name="android.intent.action.VIEW" />\n'
  printf '\t\t\t<category android:name="android.intent.category.DEFAULT" />\n\t\t\t<category android:name="android.intent.category.BROWSABLE" />\n'
  printf '\t\t\t<data android:scheme="https" android:host="%s" android:path="/" />\n' "$1"
  printf '\t\t\t<data android:scheme="https" android:host="%s" android:path="/rooms/" />\n' "$1"
  printf '\t\t</intent-filter>'
}

filtro_documentos() { # <tipos, uno por línea> -> intent-filter o nada
  [ -n "$1" ] || return 0
  printf '\n\t\t<intent-filter>\n\t\t\t<action android:name="android.intent.action.VIEW" />\n'
  printf '\t\t\t<category android:name="android.intent.category.DEFAULT" />\n'
  printf '\t\t\t<data android:scheme="content" />\n'
  printf '%s\n' "$1" | while read -r t; do printf '\t\t\t<data android:mimeType="%s" />\n' "$t"; done
  printf '\t\t</intent-filter>'
}

copiar_cifrado_apple() { # <Info.plist canónico> <Info.plist del artefacto>
  python3 - "$1" "$2" <<'PY'
import pathlib, plistlib, sys
fuente, destino = map(pathlib.Path, sys.argv[1:])
canonico = plistlib.loads(fuente.read_bytes())
uso, codigo = "ITSAppUsesNonExemptEncryption", "ITSEncryptionExportComplianceCode"
valor = canonico.get(codigo)
codigo_valido = (isinstance(valor, str) and bool(valor.strip()) and valor == valor.strip()
                 and valor != "[]" and not any(ord(c) < 32 for c in valor))
exento = canonico.get(uso) is False and codigo not in canonico
documentado = canonico.get(uso) is True and codigo_valido
if not (exento or documentado):
    sys.exit("error: el plist canónico debe declarar FALSE sin código o TRUE con código aprobado")
original = destino.read_bytes()
info = plistlib.loads(original)
for clave in (uso, codigo):
    info.pop(clave, None)
    if clave in canonico:
        info[clave] = canonico[clave]
formato = plistlib.FMT_BINARY if original.startswith(b"bplist00") else plistlib.FMT_XML
destino.write_bytes(plistlib.dumps(info, fmt=formato, sort_keys=False))
PY
}

if [ "$plataforma" = cifrado-apple ]; then
  copiar_cifrado_apple "${1:?falta el Info.plist canónico}" "${2:?falta el Info.plist del artefacto}"
  exit $?
fi

if [ "$plataforma" = tipos ]; then
  # Una app por nombre o un directorio con su FyneApp.toml: lo segundo es para que el
  # test pueda ver el caso malo sin inventarse una app en `cmd/`.
  [ "$#" -eq 1 ] || {
    echo "uso: $0 tipos <app|directorio>" >&2
    exit 2
  }
  d="$1"
  [ -d "$d" ] || d="$FYNE/cmd/$1"
  [ -f "$d/FyneApp.toml" ] || {
    echo "::error::$1 no tiene FyneApp.toml" >&2
    exit 2
  }
  tipos_de "$d"
  exit $?
fi
if [ "$plataforma" = mesa-windows ]; then
  nsis_mesa "${1:?uso: $0 mesa-windows <carpeta>}"
  nsis_quitar_mesa
  exit 0
fi
if [ "$plataforma" = documentos-mac ] || [ "$plataforma" = registro-windows ]; then
  d="${1:?uso: $0 $plataforma <app|directorio>}"
  [ -d "$d" ] || d="$FYNE/cmd/$1"
  if [ "$plataforma" = documentos-mac ]; then
    plist_documentos "$d"
    exit $?
  fi
  e="$(extensiones_de "$d")"
  a="$(metadato "$d" ID)"
  nsis_abrir_con "$e" "$a" "$(metadato "$d" Name)" "sytue-$(basename "$d")"
  nsis_quitar_abrir_con "$e" "$a" "$(metadato "$d" Name)"
  exit 0
fi

command -v go >/dev/null 2>&1 || {
  echo "::error::falta Go" >&2
  exit 1
}

bash "$AQUI/preparar-livekit.sh"

# --- El módulo privado, comprobado ANTES de compilar --------------------------
# `apps/fyne/go.mod` reemplaza `go-text/typesetting` por un fork PRIVADO en
# github.com/thinkddr (ADR 0049): el parche que quita el `ceil` del conformador. Es privado a
# propósito —publicarlo mandaría la ruta al proxy y al registro de sumas
# públicos, que es la fuga que `compliance/supply-chain/README.md` §A da por
# cerrada—, así que sin `GOPRIVATE` el `go build` de más abajo se va al proxy
# público y muere con un 404 que no dice por qué.
if grep -q '^replace .*=> github\.com/thinkddr/' "$FYNE/go.mod" 2>/dev/null; then
  case "${GOPRIVATE:-}" in
  *github.com/thinkddr*) ;;
  *)
    echo "::error::apps/fyne depende de un módulo privado de github.com/thinkddr y GOPRIVATE no lo cubre." >&2
    echo "  export GOPRIVATE='github.com/thinkddr/*'" >&2
    echo "  y un token que lea los forks (en el CI, SYTUE_GIT_FORKS_TOKEN con forks.sh;" >&2
    echo "  en el piloto, su credential.helper). Sin esto se intenta proxy.golang.org." >&2
    exit 1
    ;;
  esac
fi

# --- EL GUARDIÁN: la arquitectura de lo que sale es la declarada ---------------
# Los runners propios son ARM64 y el servidor es x86_64. Ya hubo cuatro versiones
# publicadas con la arquitectura equivocada sin que nada se pusiera en rojo, porque
# nadie miraba el binario: compilaba, empaquetaba y salía verde.
#
# Se lee del PROPIO artefacto y no del entorno que lo construyó, que es justo lo
# que fallaba. `go version -m` devuelve el GOARCH grabado en el binario; en un APK
# no hay binario Go sino una .so por ABI, y el nombre del directorio ES la
# declaración.
#
# Se ve en rojo con `apps/fyne/scripts/empaquetar-test.sh`, que le da un binario de
# la arquitectura CONTRARIA y exige que falle.
binario_de() { # <fichero o bundle> -> el ejecutable de dentro, o vacío
  local f="$1" tmp
  case "$f" in
  *.tar.xz)
    # Sin `--wildcards`, que el bsdtar de macOS no tiene: se extrae entero —son
    # unos pocos megas— y se busca después el único fichero ejecutable.
    tmp="$(mktemp -d)"
    tar -xf "$f" -C "$tmp" 2>/dev/null || true
    find "$tmp" -type f -perm -u+x ! -name '*.png' ! -name '*.desktop' ! -name '*.sh' | head -1
    ;;
  *.app) find "$f/Contents/MacOS" -type f -perm -u+x 2>/dev/null | head -1 ;;
  *) printf '%s' "$f" ;;
  esac
}

arquitectura_de() { # <fichero o bundle> -> goarch, o vacío si no se sabe
  local f="$1" bin abis
  case "$f" in
  *.apk)
    abis="$(unzip -Z1 "$f" 2>/dev/null | sed -n 's|^lib/\([^/]*\)/.*|\1|p' | sort -u)"
    case "$abis" in
    arm64-v8a) printf 'arm64' ;;
    armeabi-v7a) printf 'arm' ;;
    x86_64) printf 'amd64' ;;
    "") printf '' ;;
    *) printf 'varias:%s' "$(tr '\n' '+' <<<"$abis")" ;;
    esac
    return 0
    ;;
  *) bin="$(binario_de "$f")" ;;
  esac
  [ -n "${bin:-}" ] && [ -f "$bin" ] || {
    printf ''
    return 0
  }
  go version -m "$bin" 2>/dev/null | sed -n 's/^.*[[:space:]]GOARCH=\([a-z0-9]*\).*$/\1/p' | head -1
}

verificar_arquitectura() { # <artefacto> <goarch esperado>
  local vista
  vista="$(arquitectura_de "$1")"
  [ -n "$vista" ] || {
    echo "::error::no se pudo leer la arquitectura de $1: no se publica lo que no se puede verificar." >&2
    return 1
  }
  [ "$vista" = "$2" ] || {
    echo "::error::$1 es $vista y se declaró $2." >&2
    echo "  Es el fallo de las cuatro versiones sin desplegar: el runner es ARM64 y el servidor x86_64." >&2
    return 1
  }
  echo "  arquitectura verificada: $vista"
}

# verificar_pie exige que un ejecutable de Linux sea PIE (FYP-06): sin `-buildmode=pie`
# sale `Type: EXEC` y el sistema no puede cargarlo en una dirección aleatoria (ASLR).
# Windows y macOS ya lo son por defecto.
verificar_pie() { # <ejecutable o .tar.xz>
  # Sin tubería hacia `grep -q`: con pipefail, el acierto corta la escritura de
  # `go version` (SIGPIPE) y la verificación saldría en rojo aunque fuera PIE.
  grep -qE '^[[:space:]]*build[[:space:]]+-buildmode=pie$' <<<"$(go version -m "$(binario_de "$1")" 2>/dev/null)" || {
    echo "::error::$1 no se ha compilado como PIE (-buildmode=pie)." >&2
    return 1
  }
  echo "  PIE verificado"
}

if [ "$plataforma" = verificar-pie ]; then
  verificar_pie "${1:?uso: $0 verificar-pie <ejecutable>}"
  exit $?
fi

if [ "$plataforma" = verificar ]; then
  [ "$#" -eq 2 ] || {
    echo "uso: $0 verificar <artefacto> <goarch>" >&2
    exit 2
  }
  verificar_arquitectura "$1" "$2"
  exit 0
fi
if [ "$plataforma" = verificar-permisos-apple ]; then
  [ $# -eq 1 ] || { echo 'uso: empaquetar.sh verificar-permisos-apple <app>' >&2; exit 2; }
  verificar_permisos_apple "$1"
  exit $?
fi

# --- ANDROID: del APK al AAB de Google Play ------------------------------------
# Play solo acepta AAB. `fyne release` lo arma, pero fija target 35 (Play pide 36) y
# su `-ldflags -w` pisa el `-checklinkname=0` sin el que Rooms no enlaza; así que se
# compila con `fyne package`, que es la cadena ya verificada, y aquí se hacen los
# MISMOS pasos que `convertAPKToAAB` de fyne.io/tools —aapt2 a formato proto,
# módulo `base` con manifest/ y dex/, bundletool build-bundle— más los niveles SDK y
# la comprobación de las páginas de 16 KiB. El AAB sale SIN firmar: lo firma
# release-mobile.yml con la clave de subida, que no pasa por aquí.
ANDROID_MIN_SDK=24
ANDROID_TARGET_SDK=36

apk_a_aab() { # <apk> <aab> <lib> <servicios> <imagen-en-imagen> <manifiesto-fuente> <appdir>
  local apk="$1" aab="$2" lib="$3" servicios="${4:-}" imagen="${5:-}" manifiesto="${6:?}" appdir="${7:?}" t s
  t="$(mktemp -d)"
  mkdir -p "$t/apk" "$t/base/manifest" "$t/base/dex" "$t/base/root"
  unzip -q "$apk" -d "$t/apk" -x 'META-INF/*'
  python3 "$AQUI/android-widgets.py" --appdir "$appdir" --manifest "$manifiesto" \
    --apk-root "$t/apk" --aapt2 "$AAPT2" --android-jar "$ANDROID_JAR" \
    --min-sdk "$ANDROID_MIN_SDK" --target-sdk "$ANDROID_TARGET_SDK"
  cp "$DEX_PROPIO" "$t/apk/classes.dex"
  python3 "$AQUI/android-binario.py" alineacion "$t/apk/lib/arm64-v8a/lib$lib.so" 16384
  python3 "$AQUI/android-binario.py" min-sdk "$t/apk/AndroidManifest.xml" "$ANDROID_MIN_SDK"
  python3 "$AQUI/android-binario.py" target-sdk "$t/apk/AndroidManifest.xml" "$ANDROID_TARGET_SDK"
  # El tipo de cada servicio en primer plano, que la CLI no sabe escribir (ver
  # `servicio_de_avisos`).
  for s in $servicios; do
    python3 "$AQUI/android-binario.py" servicio-en-primer-plano "$t/apk/AndroidManifest.xml" "${s%%:*}" "${s#*:}"
  done
  # `supportsPictureInPicture` (API 24), que la tabla de atributos de la CLI no conoce.
  [ "$imagen" != si ] || python3 "$AQUI/android-binario.py" imagen-en-imagen "$t/apk/AndroidManifest.xml"
  # De vuelta a un APK (sin la firma de depuración de fyne) para que aapt2 lo pase a proto.
  (cd "$t/apk" && zip -q -r -X "$t/binario.apk" .)
  # Y comprobado con aapt2, que imprime el IDENTIFICADOR del atributo: con uno equivocado
  # (0x010104b7 en vez de 0x010104f7, pasó) aapt2 enseña el nombre bien y Android lo ignora.
  # (Sin `| grep -q`: con pipefail, grep cierra el pipe al acertar y aapt2 muere por SIGPIPE.)
  if [ "$imagen" = si ] && ! grep -q 'supportsPictureInPicture(0x010104f7)=true' \
    <<<"$("$AAPT2" dump xmltree "$t/binario.apk" --file AndroidManifest.xml)"; then
    echo "::error::$apk: supportsPictureInPicture no lleva el identificador de Android (0x010104f7)." >&2
    return 1
  fi
  "$AAPT2" convert --output-format proto -o "$t/proto.apk" "$t/binario.apk"
  unzip -q "$t/proto.apk" -d "$t/proto"
  # El módulo de bundletool: cada cosa en su carpeta, y lo que no tenga una, a root/.
  mv "$t/proto/AndroidManifest.xml" "$t/base/manifest/"
  mv "$t/proto"/*.dex "$t/base/dex/"
  for f in "$t/proto"/* "$t/proto"/.[!.]*; do
    [ -e "$f" ] || continue
    case "$(basename "$f")" in
    res | lib | assets | resources.pb) mv "$f" "$t/base/" ;;
    *) mv "$f" "$t/base/root/" ;;
    esac
  done
  rmdir "$t/base/root" 2>/dev/null || true
  (cd "$t/base" && zip -q -r -X "$t/base.zip" .)
  rm -f "$aab"
  bundletool build-bundle --modules "$t/base.zip" --output "$aab"
  rm -rf "$t"
}

# Lo que Play mira del AAB, leído DEL AAB con bundletool y no de lo que se pidió:
# el paquete (que ya no se puede cambiar una vez subido), el versionCode del tag, el
# mínimo, target y el esquema con que las demás apps la abren.
verificar_aab() { # <aab> <paquete> <esquema> [«clase:tipo …» de los servicios] [si: proveedor de documentos] [si: imagen dentro de imagen]
  local m dex
  m="$(bundletool dump manifest --bundle "$1")"
  grep -q "package=\"$2\"" <<<"$m" || {
    echo "::error::$1 no es el paquete $2." >&2
    return 1
  }
  grep -q "android:versionCode=\"$build\"" <<<"$m" || {
    echo "::error::$1 no lleva el versionCode $build." >&2
    return 1
  }
  grep -q "android:targetSdkVersion=\"$ANDROID_TARGET_SDK\"" <<<"$m" || {
    echo "::error::$1 no apunta a la API $ANDROID_TARGET_SDK: Play lo rechazaría." >&2
    return 1
  }
  grep -q "android:minSdkVersion=\"$ANDROID_MIN_SDK\"" <<<"$m" || {
    echo "::error::$1 no exige la API mínima $ANDROID_MIN_SDK: la protección automática de Play lo rechazaría." >&2
    return 1
  }
  grep -q "android:scheme=\"$3\"" <<<"$m" || {
    echo "::error::$1 no declara el esquema $3." >&2
    return 1
  }
  # La demo (eu.sytue.fyne.demo) no es un producto y va sin widgets ni broker de sesión:
  # android-widgets.py y android-sesion.py no se los ponen, y aquí tampoco se le piden.
  if [ "$2" != eu.sytue.fyne.demo ] && { ! grep -q 'android:name="eu.sytue.nativo.Widgets"' <<<"$m" ||
    ! grep -q 'android:name="android.appwidget.provider"' <<<"$m" ||
    ! grep -q 'android:name="eu.sytue.nativo.SesionProveedor"' <<<"$m" ||
    ! grep -q 'android:name="eu.sytue.nativo.SesionDesbloqueo"' <<<"$m"; }; then
    echo "::error::$1 no declara el proveedor nativo de widgets, el broker de sesión y sus recursos." >&2
    return 1
  fi
  # El proveedor de la cámara de «Escanear» (clase Captura) va en las catorce.
  grep -q "android:authorities=\"$2.sytue.captura\"" <<<"$m" || {
    echo "::error::$1 no declara el proveedor $2.sytue.captura: «Escanear» no podría usar la cámara." >&2
    return 1
  }
  if [ -n "${5:-}" ] && ! grep -q "android:authorities=\"$2.sytue.documentos\"" <<<"$m"; then
    echo "::error::$1 no declara el proveedor de documentos: Drive no saldría en «Archivos»." >&2
    return 1
  fi
  # Uno por servicio en primer plano: con target 34 o más, `startForeground` sin él lanza
  # MissingForegroundServiceTypeException.
  if [ "$(grep -c "foregroundServiceType" <<<"$m")" -ne "$(printf '%s\n' ${4:-} | grep -c .)" ]; then
    echo "::error::$1 no lleva un foregroundServiceType por servicio en primer plano (${4:-}): Android 14+ los mataría al arrancar." >&2
    return 1
  fi
  # Sin el atributo, `enterPictureInPictureMode` lanza IllegalStateException y el botón
  # de la esquina no hace nada.
  if [ "${6:-}" = si ] && ! grep -q 'android:supportsPictureInPicture="true"' <<<"$m"; then
    echo "::error::$1 no declara supportsPictureInPicture: la imagen dentro de imagen no se abriría." >&2
    return 1
  fi
  # El dex es el que se compila aquí (ver DEX_PROPIO) y no el que trae la CLI de Fyne.
  # No se usa `unzip | grep -q`: grep puede encontrar la clase antes de que
  # unzip termine, cerrar el pipe y convertir un acierto en SIGPIPE bajo
  # `set -o pipefail`. Extraer primero conserva el estado real de unzip.
  dex="$(mktemp)"
  if ! unzip -p "$1" base/dex/classes.dex >"$dex"; then
    rm -f "$dex"
    echo "::error::$1 no contiene base/dex/classes.dex; no se puede validar el dex propio." >&2
    return 1
  fi
  if ! grep -qa 'Leu/sytue/nativo/Certificados;' "$dex"; then
    rm -f "$dex"
    echo "::error::$1 lleva el dex de la CLI de Fyne, no el del fork: «Abrir un fichero» cerraría la app." >&2
    return 1
  fi
  if [ "$2" != eu.sytue.fyne.demo ] && { ! grep -qa 'Leu/sytue/nativo/Widgets;' "$dex" || ! unzip -Z1 "$1" base/assets/sytue-widget.json >/dev/null 2>&1; }; then
    rm -f "$dex"
    echo "::error::$1 no contiene la implementación del widget o sus metadatos." >&2
    return 1
  fi
  rm -f "$dex"
  echo "  AAB: $2, versionCode $build, mínimo $ANDROID_MIN_SDK, target $ANDROID_TARGET_SDK, $3:// comprobados"
}

# --- Qué apps -----------------------------------------------------------------
apps=()
if [ "$#" -gt 0 ]; then
  for a in "$@"; do
    [ -f "$FYNE/cmd/$a/FyneApp.toml" ] || {
      echo "::error::apps/fyne/cmd/$a no existe o no tiene FyneApp.toml" >&2
      exit 1
    }
    apps+=("$a")
  done
else
  for t in "$FYNE"/cmd/*/FyneApp.toml; do
    [ -f "$t" ] || continue
    apps+=("$(basename "$(dirname "$t")")")
  done
fi
[ "${#apps[@]}" -gt 0 ] || {
  echo "::error::no hay ninguna app: apps/fyne/cmd/<app>/FyneApp.toml no existe para nadie" >&2
  exit 1
}

# --- La herramienta Fyne fijada ----------------------------------------------
# La CLI (fyne.io/tools) la fija FYNE_TOOLS_VERSION en deploy/sytue-git/versions.env
# y la instalan los workflows; la biblioteca de la app sigue declarada en go.mod.
CLI="$(command -v fyne || true)"
[ -z "$CLI" ] && [ -x "$HOME/go/bin/fyne" ] && CLI="$HOME/go/bin/fyne"
[ -n "$CLI" ] || {
  echo "::error::falta la CLI de Fyne. Instálala con:" >&2
  echo "  go install fyne.io/tools/cmd/fyne@v\$(sed -n 's/^FYNE_TOOLS_VERSION=//p' deploy/sytue-git/versions.env)" >&2
  exit 1
}

# COMPILA LA CLI, NO `go build`: con `--executable`, `fyne package` no compila
# (package.go:196 de fyne.io/tools) y por tanto no inyecta fyne_metadata_init.go
# (build.go:127 solo corre dentro de su propio build). Sin él, Metadata().ID/Name/
# Custom llegan vacíos: los ajustes no se cargan ni se guardan (preferences.go:134
# del fork) y el enchufe de URL de sesion.go no se abre. Windows no lo sufre porque
# ahí `fyne package` recompila. `fyne build` honra CC, GOARCH y GOFLAGS del entorno,
# así que la arquitectura se sigue mandando desde aquí. Y si no puede inyectar, la
# CLI avisa y SIGUE («omitting metadata»): aquí eso es rojo.
compilar_fyne() { # <salida> [--tags …]; GOOS lo pone --target, el resto viene del entorno
  local salida
  salida="$("$CLI" build --tags goolm --target "$objetivo" -o "$@" 2>&1)" || { printf '%s\n' "$salida" >&2; return 1; }
  case "$salida" in *"omitting metadata"*)
    printf '%s\n' "$salida" >&2
    echo "::error::fyne build no inyectó los metadatos de FyneApp.toml en $1" >&2
    return 1 ;;
  esac
  [ -z "$salida" ] || printf '%s\n' "$salida"
}

# iOS, el dispositivo o el Simulador y siempre arm64, con `fyne build` y el clang de su SDK.
# Desde el directorio de la app. En el Simulador, `-platform_version`: sin él el enlazador
# marca el Mach-O como de iPhone y el Simulador se niega a lanzarlo. Va por GOFLAGS, que es
# de donde `fyne build` lee. En el dispositivo, `-w` como pone la CLI en `fyne package`.
minimo_ios() {
  local minimo
  minimo="$(sed -n 's/^IPHONEOS_DEPLOYMENT_TARGET = \([0-9][0-9.]*\)$/\1/p' "$FYNE/xcode/Sytue.xcconfig")"
  [ -n "$minimo" ] && [ "$(printf '%s\n' "$minimo" | wc -l | tr -d ' ')" = 1 ] || {
    echo "::error::Sytue.xcconfig debe declarar un único mínimo de iOS." >&2
    return 1
  }
  printf '%s\n' "$minimo"
}

preparar_widgets_apple() { # <fuentes> <ios|macos> <versión> <build>
  unset SYTUE_WIDGET_PRODUCTOS SYTUE_EXTENSION_PRODUCTOS
  local opciones=() con_widgets=no
  if widgets_apple_activos && [ -n "$(metadato "$1" widget_acciones)" ]; then
    con_widgets=si
  elif [ "$(metadato "$1" archivos)" = si ]; then
    opciones=(--sin-widgets)
  else
    return 0
  fi
  python3 "$AQUI/widgetkit-apple.py" --salida "$ESCENARIO/widgets" --plataforma "$2" \
    --version "$3" --build "$4" ${opciones[@]+"${opciones[@]}"} "$(basename "$1")" || return 1
  export SYTUE_EXTENSION_PRODUCTOS="$ESCENARIO/widgets/$2/productos"
  [ "$con_widgets" != si ] || export SYTUE_WIDGET_PRODUCTOS="$SYTUE_EXTENSION_PRODUCTOS"
}

enlace_widgets_apple() { # <rpath del host>
  [ -n "${SYTUE_WIDGET_PRODUCTOS:-}" ] || return 0
  [ -d "$SYTUE_WIDGET_PRODUCTOS/SytueWidgetBridge.framework" ] || {
    echo "::error::falta el framework WidgetKit que debe cargar el host." >&2
    return 1
  }
  case "$1" in '@executable_path/Frameworks'|'@executable_path/../Frameworks') ;; *)
    echo '::error::ruta de carga WidgetKit no admitida.' >&2; return 1 ;; esac
  # dlsym no aporta un símbolo fuerte: needed_framework conserva el puente en
  # LC_LOAD_DYLIB. Darwin conserva esta flag y el RPATH en go:cgo_ldflag;
  # ambas exigen la excepción finita de abajo.
  # quoted.Split de Go sólo reconoce comillas al principio de cada argumento;
  # -F\"ruta\" las conservaría dentro del token y rompería //go:cgo_ldflag.
  printf '%s\n' "-F \"$SYTUE_WIDGET_PRODUCTOS\" -Wl,-needed_framework,SytueWidgetBridge -Wl,-rpath,$1"
}

permiso_enlace_widgets_apple() {
  # Sólo nuestro puente y estas dos ubicaciones dentro del bundle firmado;
  # ningún otro framework, flag, ruta externa ni argumento de usuario.
  printf '%s\n' '^-Wl,(-needed_framework,SytueWidgetBridge|-rpath,@executable_path/(\.\./)?Frameworks)$'
}

incorporar_widgets_apple() { # <host .app> <fuentes> <ios|macappstore|developerid> <identidad>
  local opciones=() plataforma_widget=macos
  if widgets_apple_activos && [ -n "$(metadato "$2" widget_acciones)" ]; then
    :
  elif [ "$(metadato "$2" archivos)" = si ]; then
    opciones=(--sin-widgets)
  else
    return 0
  fi
  [ -n "${SYTUE_EXTENSION_PRODUCTOS:-}" ] || {
    echo "::error::no se prepararon las extensiones Apple antes de compilar el host." >&2
    return 1
  }
  [ "$3" != ios ] || plataforma_widget=ios
  python3 "$AQUI/widgetkit-apple.py" --salida "$ESCENARIO/widgets" --plataforma "$plataforma_widget" \
    --insertar "$1" --destino "$3" --identidad "$4" ${opciones[@]+"${opciones[@]}"} "$(basename "$2")"
}

compilar_ios() { # <iphoneos|iphonesimulator> <salida> [opciones de fyne build]
  local sdk="$1" salida="$2" minimo="${IPHONEOS_DEPLOYMENT_TARGET:-}" raiz_sdk cc cxx fl ld widgets_fl=""
  shift 2
  [ -n "$minimo" ] || minimo="$(minimo_ios)" || return 1
  # Aparte de cualquier export: dentro, un xcrun que falla no pararía el guion (SC2155).
  raiz_sdk="$(xcrun --sdk "$sdk" --show-sdk-path)" || return 1
  cc="$(xcrun --sdk "$sdk" -f clang)" || return 1
  cxx="$(xcrun --sdk "$sdk" -f clang++)" || return 1
  if [ "$sdk" = iphoneos ]; then
    fl="-miphoneos-version-min=$minimo"
    ld="-ldflags=-w"
  else
    fl="-mios-simulator-version-min=$minimo"
    ld="-ldflags=-extldflags=-Wl,-platform_version,ios-simulator,$minimo,$(xcrun --sdk "$sdk" --show-sdk-version)"
  fi
  fl="-isysroot $raiz_sdk $fl -arch arm64"
  [ -z "${SYTUE_WIDGET_PRODUCTOS:-}" ] || widgets_fl="$(enlace_widgets_apple '@executable_path/Frameworks')" || return 1
  (
    # shellcheck disable=SC2030 # a propósito: el subshell acota el export a esta compilación
    export GOOS=ios GOARCH=arm64 CGO_ENABLED=1 CGO_CFLAGS="$fl" CGO_CXXFLAGS="$fl" CGO_LDFLAGS="$fl${widgets_fl:+ $widgets_fl}" \
      CC="$cc" CXX="$cxx" GOFLAGS="-trimpath $ld"
    if [ -n "$widgets_fl" ]; then
      CGO_LDFLAGS_ALLOW="$(permiso_enlace_widgets_apple)"
      export CGO_LDFLAGS_ALLOW
    fi
    objetivo=ios
    compilar_fyne "$salida" "$@"
  )
}

verificar_minimo_ios() { # <ejecutable> <mínimo canónico>
  local cabecera tipo bibliotecas
  cabecera="$(xcrun vtool -show-build "$1")" || return 1
  tipo="$(xcrun otool -hv "$1")" || return 1
  python3 - "$2" "$cabecera" "$tipo" <<'PY' || return 1
import re, sys
def version(v):
    return tuple((list(map(int, v.split('.'))) + [0, 0])[:3])
comandos = re.findall(r'(?ms)^[ \t]*cmd[ \t]+(LC_BUILD_VERSION|LC_VERSION_MIN_IPHONEOS)[ \t]*\n(.*?)(?=^[ \t]*Load command |\Z)', sys.argv[2])
vistos = []
for comando, bloque in comandos:
    campo = 'minos' if comando == 'LC_BUILD_VERSION' else 'version'
    dato = re.search(r'(?m)^[ \t]*' + campo + r'[ \t]+([0-9.]+)[ \t]*$', bloque)
    if not dato:
        sys.exit('::error::no se pudo leer el mínimo del Mach-O de iOS')
    vistos.append(dato.group(1))
if not vistos or any(version(v) != version(sys.argv[1]) for v in vistos):
    sys.exit('::error::el Mach-O de iOS no declara el mínimo canónico ' + sys.argv[1])
tipos = []
lineas = sys.argv[3].splitlines()
for i, linea in enumerate(lineas[:-1]):
    campos = linea.split()
    if 'filetype' in campos:
        datos = lineas[i + 1].split()
        indice = campos.index('filetype')
        tipos.append(datos[indice] if len(datos) > indice else '')
if not tipos or any(t != 'EXECUTE' for t in tipos):
    sys.exit('::error::el Mach-O de iOS no es un ejecutable MH_EXECUTE')
PY
  bibliotecas="$(xcrun otool -L "$1")" || return 1
  case "$bibliotecas" in *'@rpath/libswift'*)
    echo "::error::$1 enlaza Swift por @rpath sin soporte Swift empaquetado: no se publica." >&2
    return 1 ;;
  esac
  echo "  mínimo real del Mach-O de iOS: $2; sin alias Swift por @rpath"
}

compilar_ios_paquete() { # <fuentes> <.app> <versión> <build>
  local fuentes="$1" app="$2" minimo ejecutable clave valor nuevo
  local metadata=(--release)
  minimo="$(minimo_ios)" || return 1
  # package incrementa el Build original del TOML; no lo deja en --app-build ni
  # --app-version. Solo cambian esos dos campos y el trap restaurar_toml repone
  # después el original completo, también cuando falla el compilador.
  python3 - "$fuentes/FyneApp.toml" "$3" "$4" <<'PY' || return 1
import re, sys
path, version, build = sys.argv[1:]
with open(path, encoding='utf-8') as f:
    texto = f.read()
seccion = re.search(r'(?ms)^\[Details\][^\n]*\n(.*?)(?=^\[|\Z)', texto)
if not seccion:
    sys.exit('::error::FyneApp.toml no declara [Details]')
detalles = seccion.group(1)
for clave, patron, valor in [('Version', r'"[^"\n]*"', '"' + version + '"'), ('Build', r'[0-9]+', build)]:
    detalles, n = re.subn(r'(?m)^([ \t]*' + clave + r'[ \t]*=[ \t]*)' + patron,
                          lambda m: m.group(1) + valor, detalles)
    if n != 1:
        sys.exit('::error::[Details] debe declarar una sola ' + clave)
with open(path, 'w', encoding='utf-8') as f:
    f.write(texto[:seccion.start(1)] + detalles + texto[seccion.end(1):])
PY
  # --release lee [Release]. Los valores que la app usa en ejecución conservan
  # esa tabla si existe y, si faltan, el valor canónico de [Development].
  for clave in esquema client_id extensiones; do
    if valor="$(metadato "$fuentes" "$clave" Release)" || valor="$(metadato "$fuentes" "$clave" Development)"; then
      metadata+=(--metadata "$clave=$valor")
    fi
  done
  ejecutable="$(python3 -c 'import plistlib,sys; print(plistlib.load(open(sys.argv[1], "rb"))["CFBundleExecutable"])' "$app/Info.plist")" || return 1
  # package dejó un Mach-O FAT (también con una sola slice). Go no reconoce su
  # magic como objeto que pueda sobrescribir: se compila a una ruta nueva y solo
  # se sustituye el ejecutable anterior después de verificar el resultado.
  nuevo="$(mktemp -d "$app/.sytue-ios.XXXXXX")" || return 1
  if (cd "$fuentes" && IPHONEOS_DEPLOYMENT_TARGET="$minimo" compilar_ios iphoneos "$nuevo/$ejecutable" "${metadata[@]}") &&
    verificar_arquitectura "$nuevo/$ejecutable" arm64 &&
    verificar_minimo_ios "$nuevo/$ejecutable" "$minimo" &&
    mv -f "$nuevo/$ejecutable" "$app/$ejecutable"; then
    rm -rf "$nuevo"
    return 0
  fi
  rm -rf "$nuevo"
  return 1
}

# macOS UNIVERSAL DE VERDAD, desde el directorio de la app: un Intel que reciba solo arm64
# muere con «Bad CPU type» y sin explicación. Go no pasa `-isysroot` al compilador de C al
# cruzar, y sin él no encuentra stdlib.h. Y EL MÍNIMO DE macOS LO FIJAMOS NOSOTROS: sin esto
# clang graba como minos la versión de la máquina (26.5 en Codemagic) y ningún Mac anterior
# abre la app. 13.0 es lo que exige Go 1.27 (Ventura).
compilar_mac() { # <salida> [--tags …]
  local salida="$1" cc widgets_fl=""
  shift
  cc="$(xcrun -f clang)"
  cc="$cc -arch x86_64 -isysroot $(xcrun -sdk macosx -show-sdk-path)"
  [ -z "${SYTUE_WIDGET_PRODUCTOS:-}" ] || widgets_fl="$(enlace_widgets_apple '@executable_path/../Frameworks')" || return 1
  (
    # shellcheck disable=SC2030,SC2031 # a propósito: el subshell acota el export a esta compilación
    export MACOSX_DEPLOYMENT_TARGET=13.0 GOFLAGS=-trimpath CGO_ENABLED=1
    if [ -n "$widgets_fl" ]; then
      export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }$widgets_fl"
      CGO_LDFLAGS_ALLOW="$(permiso_enlace_widgets_apple)"
      export CGO_LDFLAGS_ALLOW
    fi
    objetivo=darwin
    GOARCH=arm64 compilar_fyne "$salida.arm64" "$@"
    CC="$cc" GOARCH=amd64 compilar_fyne "$salida.x86_64" "$@"
  )
  lipo -create -output "$salida" "$salida.arm64" "$salida.x86_64"
  rm -f "$salida.arm64" "$salida.x86_64"
}

# EL Info.plist DE macOS E iOS: lo que ni la CLI de Fyne ni Xcode escriben solos. Lo usan
# el bucle de abajo y la fase de Xcode, y se puede repetir sobre el mismo plist (cada clave
# se borra antes de escribirse: una compilación incremental de Xcode no lo regenera).
#   - CFBundleURLTypes: el esquema con el que las demás apps la abren.
#   - Solo macOS: los tipos de documento (`plist_documentos`; iOS no abre ficheros
#     sueltos), y LSMinimumSystemVersion y la categoría, que la plantilla de la CLI graba
#     mal (10.11 y «public.app-category.»; Go 1.27 exige 13).
#   - LOS PERMISOS QUE SE EXPLICAN. Sin su clave, macOS e iOS MATAN la app al pedir el
#     permiso, sin diálogo: Face ID (solo iOS: cualquiera de las quince lo pide al abrirse
#     con el bloqueo puesto, `sesion.desbloquear`) y micrófono y cámara (las apps con
#     `llamadas`). El texto sale del catálogo común, en inglés en el plist y en cada idioma
#     en su `InfoPlist.strings` (en macOS dentro de Contents/Resources).
plist_apple() { # <macos|ios|iossimulator> <dir de la app> <Info.plist> <carpeta de recursos>
  local p="$1" dir="$2" plist="$3" recursos="$4" appid esquema documentos o
  local claves=()
  appid="$(metadato "$dir" ID)"
  esquema="$(metadato "$dir" esquema)"
  documentos="$(plist_documentos "$dir")" || return 1
  /usr/libexec/PlistBuddy -c "Delete :CFBundleURLTypes" "$plist" >/dev/null 2>&1 || true
  # PlistBuddy y no `plutil -insert`: el segundo no sabe crear un array de diccionarios
  # anidado en una sola orden.
  /usr/libexec/PlistBuddy \
    -c "Add :CFBundleURLTypes array" \
    -c "Add :CFBundleURLTypes:0 dict" \
    -c "Add :CFBundleURLTypes:0:CFBundleURLName string $appid" \
    -c "Add :CFBundleURLTypes:0:CFBundleURLSchemes array" \
    -c "Add :CFBundleURLTypes:0:CFBundleURLSchemes:0 string $esquema" \
    "$plist"
  # Una orden por llamada: con más de ~16 `-c` PlistBuddy aborta (SIGABRT).
  if [ -n "$documentos" ]; then
    /usr/libexec/PlistBuddy -c "Delete :CFBundleDocumentTypes" "$plist" >/dev/null 2>&1 || true
    /usr/libexec/PlistBuddy -c "Delete :UTImportedTypeDeclarations" "$plist" >/dev/null 2>&1 || true
    while IFS= read -r o; do /usr/libexec/PlistBuddy -c "$o" "$plist"; done <<<"$documentos"
  fi
  # `Set` falla si la clave no existe, y el plist de iOS no las trae.
  [ "$p" != macos ] || /usr/libexec/PlistBuddy \
    -c "Set :LSMinimumSystemVersion 13.0" \
    -c "Set :LSApplicationCategoryType public.app-category.productivity" "$plist"
  if [ "$p" != macos ]; then
    /usr/libexec/PlistBuddy -c "Delete :UIFileSharingEnabled" "$plist" >/dev/null 2>&1 || true
    /usr/libexec/PlistBuddy -c "Delete :LSSupportsOpeningDocumentsInPlace" "$plist" >/dev/null 2>&1 || true
    /usr/libexec/PlistBuddy \
      -c "Add :UIFileSharingEnabled bool true" \
      -c "Add :LSSupportsOpeningDocumentsInPlace bool true" "$plist"
    claves=(NSFaceIDUsageDescription=shell.lock.faceIdUsage)
  fi
  [ "$(metadato "$dir" llamadas)" != "si" ] || claves+=(NSMicrophoneUsageDescription=shell.call.microphoneUsage NSCameraUsageDescription=shell.call.cameraUsage)
  [ "${#claves[@]}" -gt 0 ] || return 0
  python3 - "$RAIZ/apps/nucleo/i18n/locales" "$plist" "$recursos" "${claves[@]}" <<'PY'
import json, os, subprocess, sys
locales, plist, recursos, pares = sys.argv[1], sys.argv[2], sys.argv[3], [a.split("=", 1) for a in sys.argv[4:]]
catalogo = {f[:-5]: json.load(open(os.path.join(locales, f), encoding="utf-8"))
            for f in os.listdir(locales) if f.endswith(".json")}
for clave_plist, clave in pares:
    subprocess.run(["/usr/libexec/PlistBuddy", "-c", "Delete :%s" % clave_plist, plist], capture_output=True)
    subprocess.run(["/usr/libexec/PlistBuddy", "-c", "Add :%s string %s" % (clave_plist, catalogo["en"][clave]), plist],
                   check=True)
for lang in sorted(catalogo):
    lineas = []
    for clave_plist, clave in pares:
        v = catalogo[lang][clave].replace("\\", "\\\\").replace('"', '\\"')
        lineas.append('"%s" = "%s";\n' % (clave_plist, v))
    # Apple llama «pt» al de Brasil; el catálogo es el de Portugal, y va en los dos.
    for carpeta in [lang] + (["pt-PT"] if lang == "pt" else []):
        os.makedirs(os.path.join(recursos, carpeta + ".lproj"), exist_ok=True)
        with open(os.path.join(recursos, carpeta + ".lproj", "InfoPlist.strings"), "w", encoding="utf-8") as s:
            s.writelines(lineas)
PY
}

# --- LA FASE DE XCODE (apps/fyne/xcode/Sytue.xcodeproj, Xcode Cloud) -------------
# Xcode arma el .app, lo firma, lo archiva y lo sube a App Store Connect; esta fase solo
# pone dentro lo que Xcode no sabe hacer: el ejecutable de Go de la app del target
# (`SYTUE_APP`) y lo que su FyneApp.toml declara en el Info.plist. Todo lo demás sale de
# lo que Xcode exporta a sus fases. El macOS de ese proyecto es SIEMPRE el de la Mac App
# Store (etiqueta `tienda`, con sus derechos de sandbox en el target): el .dmg sigue
# saliendo de `empaquetar.sh macos`. Ver «Xcode Cloud» en docs/operations.md.
if [ "$plataforma" = xcode ]; then
  dir="$FYNE/cmd/${SYTUE_APP:?lo define cada target de Sytue.xcodeproj}"
  salida="${TARGET_BUILD_DIR:?esto solo lo lanza una fase de Xcode}/${EXECUTABLE_PATH:?}"
  mkdir -p "$(dirname "$salida")"
  # Las dependencias WidgetKit ya están compiladas por Xcode; el ejecutable Go
  # también debe cargarlas para que WidgetCenter reciba Notify/Clear.
  if widgets_apple_activos && [ -n "$(metadato "$dir" widget_acciones)" ]; then
    export SYTUE_WIDGET_PRODUCTOS="${BUILT_PRODUCTS_DIR:?}"
  fi
  # ponytail: iOS sale solo arm64 aunque Xcode pida también x86_64 para el Simulador; un
  # Mac Intel no podría probarla en el suyo.
  case "${PLATFORM_NAME:?}" in
  iphoneos | iphonesimulator)
    (cd "$dir" && compilar_ios "$PLATFORM_NAME" "$salida")
    verificar_arquitectura "$salida" arm64
    destino_plist=ios
    ;;
  macosx)
    (cd "$dir" && compilar_mac "$salida" --tags tienda,goolm)
    destino_plist=macos
    ;;
  *)
    echo "::error::Sytue.xcodeproj no sabe compilar para $PLATFORM_NAME" >&2
    exit 1
    ;;
  esac
  plist_apple "$destino_plist" "$dir" "$TARGET_BUILD_DIR/${INFOPLIST_PATH:?}" \
    "$TARGET_BUILD_DIR/${UNLOCALIZED_RESOURCES_FOLDER_PATH:?}"
  exit 0
fi

# --- Versión del paquete ------------------------------------------------------
# `fyne package` exige x, x.y o x.y.z y rechaza cualquier otra forma, así que se
# compone aquí igual que en el empaquetado móvil de Gio: el tag de la release, o la
# última etiqueta del repositorio, o 0.0.0. El número de build es el de la
# ejecución del CI, que es monótono —lo que Play Store exige— y >0, que es lo que
# exige la herramienta.
version="${TAG:-}"
[ -n "$version" ] || version="$(git -C "$RAIZ" describe --tags --abbrev=0 2>/dev/null || true)"
version="${version#v}"
grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' <<<"$version" || version="0.0.0"
# En Codemagic no existe GITHUB_RUN_NUMBER: su contador por workflow es BUILD_NUMBER.
build="${GITHUB_RUN_NUMBER:-${BUILD_NUMBER:-1}}"
[ "$build" -gt 0 ] 2>/dev/null || build=1
# En Android, el del TAG (ver `version_code`): es lo que Play compara. Sin tag queda el
# de la ejecución, y ese paquete no puede ir a Play —release-mobile.yml lo comprueba—.
if [ "$plataforma" = android ] && [ "$version" != 0.0.0 ]; then
  build="$(version_code "$version")"
fi

# --- Preparación por plataforma ------------------------------------------------
# GOARCH de lo que se publica en cada destino. Android va a arm64 y solo a arm64:
# es lo que llevan todos los teléfonos con los que se prueba, y cada ABI de más
# multiplica el tamaño del APK y el tiempo de compilación.
case "$plataforma" in
linux)
  arch=amd64
  case "$(uname -m)" in
  x86_64 | amd64)
    # shellcheck disable=SC2031 # el CC de quien llama, no el que compilar_ios/compilar_mac acotan a su subshell
    CC_DESTINO="${CC:-gcc}"
    ;;
  aarch64 | arm64)
    command -v x86_64-linux-gnu-gcc >/dev/null 2>&1 || {
      echo "::error::desde ARM64 hace falta x86_64-linux-gnu-gcc para publicar Linux/amd64." >&2
      echo "  Lo instala el perfil 'escritorio-amd64' de .github/actions/toolchain/accion.sh." >&2
      exit 1
    }
    CC_DESTINO=x86_64-linux-gnu-gcc
    export PKG_CONFIG_PATH=
    export PKG_CONFIG_LIBDIR=/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig
    ;;
  *)
    echo "::error::no sé publicar Linux/amd64 desde $(uname -m)." >&2
    exit 1
    ;;
  esac
  # Las cabeceras de GL, X11 y xkbcommon, y sin root salen del sysroot de
  # `bash scripts/fyne-sdk-linux.sh` y `source ~/fyne-sdk/entorno.sh`. Verificado el
  # 2026-09-21 compilando apps/fyne/cmd/demo con él en el piloto.
  pkg-config --exists gl x11 xkbcommon wayland-client wayland-cursor wayland-egl 2>/dev/null || {
    echo "::error::faltan las cabeceras de GL/X11/Wayland/xkbcommon que Fyne necesita en Linux." >&2
    echo "  En el CI: .github/actions/toolchain/accion.sh escritorio-amd64" >&2
    echo "  Sin root: bash scripts/fyne-sdk-linux.sh && source ~/fyne-sdk/entorno.sh" >&2
    exit 1
  }
  ;;
windows)
  arch=amd64
  # Sin NSIS no hay bundle —el instalador es el que registra esquemas, tipos y AUMID de
  # cada app— y un .exe suelto ya no se publica: se para antes de compilar catorce apps.
  command -v makensis >/dev/null 2>&1 || {
    echo "::error::falta makensis (paquete nsis): el bundle de Windows es su instalador." >&2
    exit 1
  }
  # shellcheck disable=SC2031 # el CC de quien llama, como arriba
  CC_DESTINO="${CC:-}"
  [ -n "$CC_DESTINO" ] || CC_DESTINO="$(command -v x86_64-w64-mingw32-gcc || true)"
  [ -n "$CC_DESTINO" ] || {
    echo "::error::falta x86_64-w64-mingw32-gcc para cruzar a Windows/amd64 (paquete gcc-mingw-w64-x86-64)." >&2
    exit 1
  }
  # FIRMA (Azure Artifact Signing, ver «Windows: Azure Artifact Signing» en
  # docs/operations.md): con AZURE_SIGNING_ENDPOINT en el entorno se firman los .exe de
  # las apps, el desinstalador y el instalador, y cualquier fallo es rojo. Sin él (ci-desktop,
  # empaquetar-test.sh, una prueba a mano) sale sin firmar y lo dice; lo que NO puede pasar
  # es publicarlo así, y eso lo para release-desktop.yml verificando antes de adjuntar.
  FIRMAR_WIN="$RAIZ/deploy/sytue-git/bin/firmar-windows.sh"
  if [ -n "${AZURE_SIGNING_ENDPOINT:-}" ]; then
    for h in jsign osslsigncode openssl curl; do
      command -v "$h" >/dev/null 2>&1 || {
        echo "::error::falta $h para firmar Windows: .github/actions/toolchain/accion.sh jdk jsign osslsigncode" >&2
        exit 1
      }
    done
  else
    FIRMAR_WIN=""
    echo "::warning::sin AZURE_SIGNING_ENDPOINT: los instaladores de Windows salen SIN FIRMAR (no publicables)" >&2
  fi
  ;;
macos)
  arch=universal
  [ "$(uname -s)" = Darwin ] || {
    echo "::error::macOS se empaqueta en un Mac: el bundle .app y el DMG piden lipo y hdiutil." >&2
    echo "  En CI es el job 'macos' de release-desktop.yml, en el Mac propio (runner sytue-macos)." >&2
    echo "  El portátil del dueño NO se usa para builds." >&2
    exit 1
  }
  for h in lipo hdiutil xcrun; do
    command -v "$h" >/dev/null 2>&1 || {
      echo "::error::falta $h: hacen falta las Command Line Tools de Xcode." >&2
      exit 1
    }
  done
  # La variante de tienda no sale sin firmar: un .pkg sin las dos firmas no sirve para nada.
  if [ -n "$tienda" ]; then
    [ -n "${SYTUE_MAS_IDENTIDAD:-}" ] && [ -n "${SYTUE_MAS_INSTALADOR:-}" ] || {
      echo "::error::faltan SYTUE_MAS_IDENTIDAD (el certificado de distribución, «Apple Distribution: …») y" >&2
      echo "  SYTUE_MAS_INSTALADOR («3rd Party Mac Developer Installer: …»): los da" >&2
      echo "  «apple-firma.sh preparar macappstore <App ID>…»; en CI es el workflow 'macappstore-fyne'." >&2
      exit 1
    }
    for h in productbuild pkgutil codesign security; do
      command -v "$h" >/dev/null 2>&1 || {
        echo "::error::falta $h para armar y firmar el .pkg de la Mac App Store." >&2
        exit 1
      }
    done
  fi
  ;;
android)
  arch=arm64
  # EN LINUX ARM64, CON LA CLI DE FYNE DE AMD64. `archNDK()` (mobile/env.go de
  # fyne.io/tools) elige el NDK por la arquitectura CON LA QUE SE COMPILÓ LA CLI, y Google
  # no publica NDK para Linux ARM64: la nativa entra en pánico («unsupported GOARCH:
  # arm64»). Compilada con GOARCH=amd64 corre por qemu-user, igual que el clang del NDK
  # x86_64 (`accion.sh android-sdk`), y el `go build` que lanza sigue siendo el nativo.
  # Así compila release-mobile.yml en nuestros runners, sin Codemagic.
  if [ "$(uname -s)" = Linux ] && [ "$(uname -m)" != x86_64 ] &&
    ! grep -qE '^[[:space:]]*build[[:space:]]+GOARCH=amd64$' <<<"$(go version -m "$CLI" 2>/dev/null)"; then
    echo "::error::Android desde Linux/$(uname -m) necesita la CLI de Fyne de amd64: GOARCH=amd64 go install fyne.io/tools/cmd/fyne@v… (y qemu-user)." >&2
    exit 1
  fi
  export ANDROID_HOME="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}"
  [ -n "$ANDROID_HOME" ] && [ -d "$ANDROID_HOME" ] || {
    echo "::error::sin ANDROID_HOME (ni ANDROID_SDK_ROOT): hace falta el SDK de Android." >&2
    echo "  En el piloto: source ~/env-android.sh" >&2
    exit 1
  }
  if [ -z "${ANDROID_NDK_HOME:-}" ] && [ -n "${ANDROID_NDK_ROOT:-}" ]; then
    export ANDROID_NDK_HOME="$ANDROID_NDK_ROOT"
  fi
  # El SDK de Codemagic trae los NDK en `ndk/<versión>` sin exportar ninguna de las dos.
  if [ -z "${ANDROID_NDK_HOME:-}" ] && [ -d "$ANDROID_HOME/ndk" ]; then
    ANDROID_NDK_HOME="$(find "$ANDROID_HOME/ndk" -mindepth 1 -maxdepth 1 -type d | sort -V | tail -1)"
    export ANDROID_NDK_HOME
  fi
  # El empaquetador de Fyne lee ANDROID_NDK_HOME; el de Gio leía ANDROID_NDK_ROOT.
  # Se aceptan los dos y se exporta el que mira ESTA herramienta, que es la clase de
  # detalle que hace fallar el build dentro y no aquí.
  [ -n "${ANDROID_NDK_HOME:-}" ] && [ -d "$ANDROID_NDK_HOME" ] || {
    echo "::error::sin ANDROID_NDK_HOME (ni ANDROID_NDK_ROOT): el NDK hace falta para el cgo de Android." >&2
    exit 1
  }
  command -v javac >/dev/null 2>&1 || [ -x "${JAVA_HOME:-/nada}/bin/javac" ] || {
    echo "::error::falta un JDK." >&2
    exit 1
  }
  # Lo que pasa el APK a AAB (ver `apk_a_aab`), comprobado antes de compilar catorce apps.
  for h in bundletool python3 zip unzip; do
    command -v "$h" >/dev/null 2>&1 || {
      echo "::error::falta $h para armar el AAB. bundletool: .github/actions/toolchain/accion.sh bundletool" >&2
      exit 1
    }
  done
  AAPT2="$(find "$ANDROID_HOME/build-tools" -name aapt2 -type f 2>/dev/null | sort -V | tail -1)"
  [ -n "$AAPT2" ] && [ -x "$AAPT2" ] || {
    echo "::error::sin aapt2 en $ANDROID_HOME/build-tools: ni se verifica el APK ni se arma el AAB." >&2
    exit 1
  }
  # EL DEX DE LA APK SE COMPILA AQUÍ, NO LO PONE LA CLI. `fyne package` mete el dex que
  # fyne.io/tools lleva incrustado (dex.go), compilado del GoNativeActivity.java de la
  # Fyne de upstream, no del fork. El fork cambió `showFileOpen` a (String, boolean) y la
  # biblioteca busca esa firma al arrancar: con el dex viejo no la encuentra
  # («cannot find method showFileOpen») y «Abrir un fichero» tumbaba la app entera
  # («JNI DETECTED ERROR: mid == null»), medido en el emulador con v3.6.26. Y las clases
  # propias de apps/fyne/android/java (Certificados, Avisos…) no entraban en la APK.
  # Se hace lo mismo que gendex.go de fyne.io/tools, con las fuentes que de verdad van.
  D8="$(dirname "$AAPT2")/d8"
  ANDROID_JAR="$ANDROID_HOME/platforms/android-$ANDROID_TARGET_SDK/android.jar"
  [ -x "$D8" ] && [ -f "$ANDROID_JAR" ] || {
    echo "::error::falta d8 ($D8) o android.jar de la API $ANDROID_TARGET_SDK: sdkmanager 'platforms;android-$ANDROID_TARGET_SDK'." >&2
    exit 1
  }
  # `go list -m` da el directorio de un módulo solo si ya está en la caché: en una máquina
  # recién hecha (Codemagic) salía VACÍO y javac buscaba `/internal/driver/mobile/app`
  # (build 6ab946479c453db833436cc8). Se descarga antes, y un vacío para aquí.
  (cd "$FYNE" && go mod download fyne.io/fyne/v2)
  fork_fyne="$(cd "$FYNE" && go list -m -f '{{with .Replace}}{{.Dir}}{{else}}{{.Dir}}{{end}}' fyne.io/fyne/v2)"
  [ -n "$fork_fyne" ] && [ -d "$fork_fyne/internal/driver/mobile/app" ] || {
    echo "::error::no se encuentra el fork de Fyne en la caché de módulos («$fork_fyne»): sin él no hay dex." >&2
    exit 1
  }
  dex_tmp="$(mktemp -d)"
  # Listas en arrays y no `$(find …)` troceado: una ruta con espacios se partiría. Con
  # `while read` y no mapfile, que bash 3.2 (el de macOS) no tiene.
  java_propias=()
  while IFS= read -r fuente; do java_propias+=("$fuente"); done < <(find "$FYNE/android/java" -name '*.java')
  # Pins finales por paquete, no la firma de subida del AAB. La activación queda en
  # el catálogo revisado; nunca la cambia un env del runner ni una preferencia.
  python3 "$AQUI/android-sesion.py" java "$dex_tmp/generated/eu/sytue/nativo/SesionFirmas.java"
  java_propias+=("$dex_tmp/generated/eu/sytue/nativo/SesionFirmas.java")
  # `-encoding UTF-8`: los comentarios van en castellano, y un JDK con el locale en ASCII
  # (el contenedor del frontal) no compila ni una clase sin él.
  "$(command -v javac || echo "$JAVA_HOME/bin/javac")" --release 17 -encoding UTF-8 -nowarn -cp "$ANDROID_JAR" -d "$dex_tmp/cls" \
    "$fork_fyne"/internal/driver/mobile/app/*.java ${java_propias[@]+"${java_propias[@]}"} >/dev/null
  # El cliente WebDAV del proveedor de Drive es Java sin Android: su prueba corre aquí, en
  # la JVM del mismo JDK, antes de meterlo en ninguna APK.
  "$(command -v javac || echo "$JAVA_HOME/bin/javac")" --release 17 -encoding UTF-8 -nowarn -d "$dex_tmp/prueba" \
    "$FYNE/android/java/eu/sytue/nativo/WebDAV.java" "$FYNE/android/prueba/eu/sytue/nativo/WebDAVPrueba.java" >/dev/null
  "$(command -v java || echo "$JAVA_HOME/bin/java")" -cp "$dex_tmp/prueba" eu.sytue.nativo.WebDAVPrueba
  "$(command -v javac || echo "$JAVA_HOME/bin/javac")" --release 17 -encoding UTF-8 -nowarn -d "$dex_tmp/prueba" \
    "$FYNE/android/java/eu/sytue/nativo/DocumentosIdentidad.java" "$FYNE/android/prueba/eu/sytue/nativo/DocumentosIdentidadPrueba.java"
  "$(command -v java || echo "$JAVA_HOME/bin/java")" -cp "$dex_tmp/prueba" eu.sytue.nativo.DocumentosIdentidadPrueba
  "$(command -v javac || echo "$JAVA_HOME/bin/javac")" --release 17 -encoding UTF-8 -nowarn -d "$dex_tmp/prueba" \
    "$FYNE/android/java/eu/sytue/nativo/DrivePreferencia.java" "$FYNE/android/java/eu/sytue/nativo/SesionConfianza.java" \
    "$dex_tmp/generated/eu/sytue/nativo/SesionFirmas.java" "$FYNE/android/prueba/eu/sytue/nativo/DrivePreferenciaPrueba.java"
  "$(command -v java || echo "$JAVA_HOME/bin/java")" -cp "$dex_tmp/prueba" eu.sytue.nativo.DrivePreferenciaPrueba
  "$(command -v javac || echo "$JAVA_HOME/bin/javac")" --release 17 -encoding UTF-8 -nowarn -d "$dex_tmp/prueba" \
    "$FYNE/android/java/eu/sytue/nativo/WidgetSnapshot.java" "$FYNE/android/prueba/eu/sytue/nativo/WidgetSnapshotPrueba.java"
  "$(command -v java || echo "$JAVA_HOME/bin/java")" -cp "$dex_tmp/prueba" eu.sytue.nativo.WidgetSnapshotPrueba
  "$(command -v javac || echo "$JAVA_HOME/bin/javac")" --release 17 -encoding UTF-8 -nowarn -d "$dex_tmp/prueba" \
    "$FYNE/android/java/eu/sytue/nativo/GestosRegion.java" "$FYNE/android/prueba/eu/sytue/nativo/GestosRegionPrueba.java"
  "$(command -v java || echo "$JAVA_HOME/bin/java")" -cp "$dex_tmp/prueba" eu.sytue.nativo.GestosRegionPrueba
  SYTUE_AAPT2="$AAPT2" SYTUE_ANDROID_JAR="$ANDROID_JAR" python3 "$AQUI/android-widgets-test.py"
  clases=()
  while IFS= read -r fuente; do clases+=("$fuente"); done < <(find "$dex_tmp/cls" -name '*.class')
  "$D8" --min-api "$ANDROID_MIN_SDK" --lib "$ANDROID_JAR" --output "$dex_tmp" ${clases[@]+"${clases[@]}"}
  DEX_PROPIO="$dex_tmp/classes.dex"
  ;;
ios)
  arch=arm64
  # UN PERFIL POR APP: el de App Store de su App ID. `apple-firma.sh preparar ios` los
  # deja instalados y cada app busca el suyo por su identificador (`perfil_ios`, más
  # abajo). SYTUE_IOS_PROFILE lo fija a mano, y entonces solo vale para UNA app: con
  # el perfil de otra, xcodebuild falla o, peor, firma con un App ID que no es el suyo.
  if [ -n "${SYTUE_IOS_PROFILE:-}" ] && [ "${#apps[@]}" -ne 1 ]; then
    echo "::error::SYTUE_IOS_PROFILE nombra UN perfil y se han pedido ${#apps[@]} apps: cada app lleva el suyo." >&2
    echo "  Quítalo (cada app busca su perfil instalado) o empaqueta una sola app." >&2
    exit 1
  fi
  # NO SE PUEDE HOY, Y EL MENSAJE LO DICE ENTERO. La alternativa —dejar que falle
  # dentro de xcodebuild dentro de fyne— es el fallo opaco que dentro de un año
  # nadie sabrá leer.
  if [ "$(uname -s)" != Darwin ]; then
    echo "::error::iOS se empaqueta en un Mac con Xcode; Apple no deja compilar para iOS fuera de su hardware." >&2
    echo "  En CI lo archiva Xcode Cloud con el tag tienda-v…." >&2
    exit 1
  fi
  command -v xcodebuild >/dev/null 2>&1 || {
    echo "::error::falta xcodebuild (Xcode completo, no solo las Command Line Tools)." >&2
    exit 1
  }
  # La identidad con la que firma xcodebuild y con la que se VUELVE a firmar después de
  # ponerle el esquema, las descripciones de los permisos y los Universal Links
  # (`firmar_ipa`). La imprime `apple-firma.sh preparar ios`.
  [ -n "${SYTUE_IOS_IDENTIDAD:-}" ] || {
    echo "::error::falta SYTUE_IOS_IDENTIDAD (p. ej. «Apple Distribution: Sytue Projects, S.L. (ABCDE12345)»):" >&2
    echo "  la da «apple-firma.sh preparar ios <App ID>…», que trae el certificado y los perfiles;" >&2
    echo "  en CI firma Xcode Cloud (tag tienda-v…). Para el Simulador no hace falta:" >&2
    echo "  empaquetar.sh iossimulator <app>" >&2
    exit 1
  }
  ;;
iossimulator)
  arch=arm64
  { [ "$(uname -s)" = Darwin ] && command -v xcrun >/dev/null 2>&1 &&
    xcrun --sdk iphonesimulator --show-sdk-path >/dev/null 2>&1; } || {
    echo "::error::el Simulador de iOS solo existe en un Mac con Xcode (falta el SDK iphonesimulator)." >&2
    exit 1
  }
  # ponytail: solo arm64 (Apple Silicon); un Mac Intel pediría GOARCH=amd64 aquí.
  [ "$(uname -m)" = arm64 ] || {
    echo "::error::el .app del Simulador sale arm64 y este Mac es $(uname -m)." >&2
    exit 1
  }
  ;;
esac

mkdir -p "$SALIDA"

# En escritorio lo de cada app se deja en un ESCENARIO y lo que llega a `dist/` es el
# bundle, que se arma después del bucle: `bundles/<clave>/` junta lo de sus apps.
ESCENARIO="$(mktemp -d)"
# `fyne package` REESCRIBE el FyneApp.toml que lee: sube el Build y lo guarda sin los
# comentarios. Cada app se empaqueta con su fichero y al salir —bien o mal— se deja el
# original, byte a byte; si no, un empaquetado en el Mac deja el árbol sucio.
mkdir -p "$ESCENARIO/toml"
restaurar_toml() {
  local c
  for c in "$ESCENARIO"/toml/*; do
    [ -e "$c" ] && cp "$c" "$FYNE/cmd/$(basename "$c")/FyneApp.toml"
  done
  return 0
}
trap 'restaurar_toml; rm -rf "$ESCENARIO"' EXIT
[ "$plataforma" != windows ] || preparar_mesa

# La CLI de Fyne genera su propio proyecto con iOS 9. Xcode 27 solo admite desde
# iOS 15: el mínimo del proyecto versionado también debe llegar a ese proyecto
# temporal. XCODE_XCCONFIG_FILE prevalece sobre sus ajustes, sin cambiar la firma.
if [ "$plataforma" = ios ]; then
  minimo_proyecto_ios="$(minimo_ios)"
  printf 'IPHONEOS_DEPLOYMENT_TARGET = %s\n' "$minimo_proyecto_ios" >"$ESCENARIO/ios.xcconfig"
fi

for app in "${apps[@]}"; do
  dir="$FYNE/cmd/$app"
  [ -f "$dir/FyneApp.toml" ] && cp "$dir/FyneApp.toml" "$ESCENARIO/toml/$app"
  appid="$(metadato "$dir" ID)"
  nombre="$(metadato "$dir" Name)"
  esquema="$(metadato "$dir" esquema)"
  # Las apps con LLAMADAS (Rooms, y Cast al emitir) usan el micrófono: piden su permiso en Android y
  # declaran para qué en el Info.plist, o macOS e iOS las matan al abrirlo. Solo ellas:
  # un permiso que nadie usa está prohibido (encargo v4 §4).
  llamadas="$(metadato "$dir" llamadas)"
  # La imagen dentro de imagen de Android (Cast): el atributo va en el binario (apk_a_aab).
  pip="$(metadato "$dir" imagenEnImagen)"
  # Las apps DUEÑAS DE UN HOST de la web (Rooms, `enlaces = "rooms"`): sus enlaces
  # https —el de una reunión, `rooms.<dominio>/?rooms=<id>`— abren la app en Android
  # (App Links, `autoVerify`) si el sitio publica su `assetlinks.json`. El dominio es
  # el de la suite que se empaqueta.
  enlaces="$(metadato "$dir" enlaces)"
  host_enlaces=""
  [ -z "$enlaces" ] || host_enlaces="$enlaces.${SYTUE_DOMINIO:-sytue.eu}"
  # USE_BIOMETRIC, en las catorce: BiometricPrompt la exige para el bloqueo de la sesión y
  # los cuadernos de Notepads (`escritorio.Verificar`, clase Biometria).
  permisos_extra=(android.permission.USE_BIOMETRIC)
  [ "$llamadas" != "si" ] || permisos_extra+=(android.permission.CAMERA android.permission.FOREGROUND_SERVICE android.permission.FOREGROUND_SERVICE_MEDIA_PROJECTION android.permission.MODIFY_AUDIO_SETTINGS android.permission.RECORD_AUDIO)
  # EL HUB SE QUEDA VIVO para avisar con las apps cerradas: un servicio en primer plano
  # (clase Avisos, `escritorio.MantenerAvisos`) de tipo specialUse, que es lo que es:
  # mantener abierto el flujo del Hub sin Firebase. Solo el Hub; las demás no lo llevan.
  # Y DRIVE ENTRA EN «ARCHIVOS» DE ANDROID: el proveedor de documentos, solo en su app.
  documentos=""
  [ "$app" != drive ] || documentos=si
  servicio_avisos=""
  if [ "$app" = hub ]; then
    servicio_avisos=si
    permisos_extra+=(android.permission.FOREGROUND_SERVICE android.permission.FOREGROUND_SERVICE_SPECIAL_USE)
  fi
  [ -n "$appid" ] && [ -n "$nombre" ] || {
    echo "::error::apps/fyne/cmd/$app: FyneApp.toml sin ID o sin Name" >&2
    exit 1
  }
  # LAS APPS SON INDEPENDIENTES Y SE LLAMAN ENTRE SÍ, así que sin esquema una app
  # queda incomunicada: se instala y nadie puede abrirla desde otra. Es un fallo
  # de empaquetado, no un detalle opcional.
  [ -n "$esquema" ] || {
    echo "::error::apps/fyne/cmd/$app: FyneApp.toml no declara [Development] esquema." >&2
    echo "  Es el esquema de URL con el que las demás apps la abren (y el redirect_uri del login)." >&2
    exit 1
  }
  echo "== $nombre ($plataforma/$arch) · $appid · $esquema://"
  # Lo que se ofrece en «Abrir con». Vacío es lo normal: solo Docus, Calcus y Diapos
  # abren un fichero suelto.
  extensiones="$(extensiones_de "$dir")"
  tipos="$(tipos_de "$dir")" || exit 1
  # Los tipos de documento de macOS, comprobados antes de compilar (`plist_apple`).
  plist_documentos "$dir" >/dev/null || exit 1
  [ -z "$extensiones" ] || echo "   abre con: $extensiones"

  # El EJECUTABLE se llama `sytue-<app>`, no como el directorio temporal: ese
  # nombre acaba en /usr/local/bin y en el Exec del .desktop, así que tiene que
  # estar espaciado de nombres — dieciséis apps comparten ese directorio.
  ejecutable="sytue-$app"

  # La salida de `fyne package` cae al lado del fuente con el nombre de la app; se
  # limpia antes para no recoger la de una ejecución anterior.
  rm -rf "${dir:?}"/*.tar.xz "${dir:?}"/*.exe "${dir:?}"/*.app "${dir:?}"/*.apk "${dir:?}"/*.ipa \
    "${dir:?}/$ejecutable" "${dir:?}/AndroidManifest.xml"

  # `android/arm64` y no `android` a secas: sin la arquitectura, la herramienta
  # mete las CUATRO ABI en el APK —arm64-v8a, armeabi-v7a, x86 y x86_64— y el
  # paquete se multiplica por cuatro. Lo pilló el guardián de arquitectura la
  # primera vez que se ejecutó, que es exactamente para lo que está.
  # Y `darwin`, no `macos`: la herramienta no conoce ese nombre y muere con
  # «unsupported target operating system», ya con los dos binarios compilados.
  objetivo="$plataforma"
  [ "$plataforma" = android ] && objetivo=android/arm64
  [ "$plataforma" = macos ] && objetivo=darwin
  # `fyne build --target ios` no fija GOOS (build.go de fyne.io/tools) y deja el del
  # entorno: así compila para el Simulador y además inyecta los metadatos.
  [ "$plataforma" = iossimulator ] && objetivo=ios
  # `--metadata` manda sobre las tablas del TOML (package.go:280 va antes de
  # mergeMetadata): con `--release` (iOS) la CLI lee [Release] y no [Development]
  # (data.go:51-55) y el esquema se perdería → sesion.go fallaría al entrar.
  comunes=(--target "$objetivo" --app-version "$version" --app-build "$build" --metadata "esquema=$esquema")

  unset SYTUE_WIDGET_PRODUCTOS SYTUE_EXTENSION_PRODUCTOS
  if [ "$plataforma" = ios ] || { [ "$plataforma" = macos ] && { [ -n "$tienda" ] || [ -n "${SYTUE_MACOS_IDENTIDAD:-}" ]; }; }; then
    preparar_widgets_apple "$dir" "$plataforma" "$version" "$build" || exit 1
  fi

  # --- ANDROID: el manifiesto es NUESTRO ---------------------------------------
  # `fyne package` usa el `AndroidManifest.xml` que encuentre en el directorio del
  # paquete y solo genera uno si no hay. Se aprovecha, porque el suyo tiene dos
  # problemas: pide WRITE_EXTERNAL_STORAGE y READ_EXTERNAL_STORAGE «por si acaso»
  # —justo lo que el encargo v4 §4 prohíbe— y no declara ningún deep link, con lo
  # que ninguna app podría abrir a otra.
  #
  # Se ESCRIBE aquí y no se versiona: dieciséis manifiestos a mano es lo que pidió
  # el dueño que no hubiera, y además se desincronizarían del FyneApp.toml.
  #
  # `android.app.lib_name` tiene que ser el que la herramienta usa para nombrar la
  # .so, o la actividad arranca y no encuentra el código. Es `androidPkgName(Name)`
  # —cada carácter que no sea alfanumérico pasa a `_`—; se reproduce aquí y se
  # VERIFICA después contra la .so que hay dentro del APK, que es lo que no se
  # puede dar por bueno.
  #
  # SIN `<uses-sdk>`: la herramienta lo rechaza explícitamente («manual declaration
  # of uses-sdk in AndroidManifest.xml not supported») porque lo escribe ella al
  # compilar los recursos. Medido el 2026-09-21.
  #
  # Eso deja minSdkVersion=21 y targetSdkVersion=29, que son los suyos. Play exige
  # target 36 desde el 31/08/2026 y su protección automática activada exige mínimo 24.
  # `apk_a_aab` fija ANDROID_MIN_SDK y ANDROID_TARGET_SDK en el manifiesto ya compilado
  # (android-binario.py). OJO: el target cambia el
  # comportamiento en ejecución —de borde a borde obligatorio, ámbitos de
  # almacenamiento, permiso de avisos—, y eso solo se ve en un teléfono.
  if [ "$plataforma" = android ]; then
    # `launchMode="singleTask"`: con el modo por omisión (`standard`), volver del
    # navegador o pulsar un documento CREA OTRA actividad encima en vez de llamar a
    # `onNewIntent` de la que ya corre —que es por donde el fork de Fyne entrega la URL
    # (`GoNativeActivity.onNewIntent`)—. Sin esto no llegan ni la vuelta del login ni
    # los enlaces entrantes, y se ven dos ventanas de la misma app.
    # Girar y redimensionar cambia también screenSize/screenLayout. El driver
    # recompone el lienzo con size.Event: TODAS las apps conservan su actividad,
    # incluida una sin imagen en imagen, en vez de recrearla al girar.
    libname="$(printf '%s' "$nombre" | sed 's/[^a-zA-Z0-9]/_/g')"
    sesion_queries=""
    sesion_components=""
    if [ "$app" != demo ]; then
      sesion_queries="$(python3 "$AQUI/android-sesion.py" queries - --package "$appid")"
      sesion_components="$(python3 "$AQUI/android-sesion.py" components - --package "$appid")"
    fi
    # `fyne package` solo admite GoNativeActivity en el manifiesto que lee (rechaza
    # cualquier segunda `<activity>`, como SesionDesbloqueo, y su codificador binres es
    # anterior a `<queries>`). El manifiesto completo con `<queries>`, SesionProveedor y
    # SesionDesbloqueo va a `$ESCENARIO/android-$app.xml`, que `apk_a_aab` enlaza después
    # con el `aapt2 link --manifest` real (API 36).
    manifiesto_android() { # <queries> <components>
      cat <<XML
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
	package="$appid"
	android:versionCode="$build"
	android:versionName="$version">

	<uses-permission android:name="android.permission.INTERNET" />
	<uses-permission android:name="android.permission.POST_NOTIFICATIONS" />$(for p in ${permisos_extra[@]+"${permisos_extra[@]}"}; do printf '\n\t<uses-permission android:name="%s" />' "$p"; done)$([ "$llamadas" != si ] || printf '\n\t<uses-feature android:name="android.hardware.camera" android:required="false" />\n\t<uses-feature android:name="android.hardware.microphone" android:required="false" />')
${1:+
$1}
	<application android:label="$nombre" android:debuggable="false">
	<activity android:name="org.golang.app.GoNativeActivity"
		android:label="$nombre"
		android:configChanges="orientation|keyboardHidden|uiMode|screenSize|smallestScreenSize|screenLayout"
		android:exported="true"
		android:launchMode="singleTask"
		android:theme="@android:style/Theme">
		<meta-data android:name="android.app.lib_name" android:value="$libname" />
		<intent-filter>
			<action android:name="android.intent.action.MAIN" />
			<category android:name="android.intent.category.LAUNCHER" />
		</intent-filter>
		<intent-filter>
			<action android:name="android.intent.action.VIEW" />
			<category android:name="android.intent.category.DEFAULT" />
			<category android:name="android.intent.category.BROWSABLE" />
			<data android:scheme="$esquema" />
		</intent-filter>$(filtro_enlaces "$host_enlaces")$(filtro_documentos "$tipos")
	</activity>

	<receiver android:name="org.golang.app.FyneNotificationReceiver"
		android:exported="false" />
	<provider android:name="eu.sytue.nativo.Captura"
		android:authorities="$appid.sytue.captura"
		android:exported="false"
		android:grantUriPermissions="true" />$(servicio_de_avisos "$servicio_avisos")$(proveedor_documentos "$appid" "$documentos")$([ "$llamadas" != si ] || printf '\n\t<service android:name="eu.sytue.nativo.PantallaServicio" android:exported="false" />')${2:+
$2}
	</application>
</manifest>
XML
    }
    manifiesto_android "" "" >"$dir/AndroidManifest.xml"
    manifiesto_android "$sesion_queries" "$sesion_components" >"$ESCENARIO/android-$app.xml"
  fi

  case "$plataforma" in
  linux | windows | macos)
    exe="$dir/$ejecutable"
    [ "$plataforma" = windows ] && exe="$exe.exe"
    (
      cd "$dir"
      case "$plataforma" in
      linux)
        # UN SOLO BINARIO PARA X11 Y WAYLAND, que elige al arrancar: sin etiqueta `x11`
        # ni `wayland`, el GLFW 3.4 del fork lleva las dos plataformas y Fyne escoge
        # (`forcePlatform`): Wayland nativo donde el compositor decora las ventanas
        # (KWin, wlroots), X11 por XWayland donde exige decorar en el cliente (GNOME), y
        # X11 fuera de Wayland; FYNE_PLATFORM=x11|wayland lo fuerza. En Plasma va por
        # Wayland nativo desde el fork .13 (hoy .14) (el alfa del framebuffer ya es opaco). Las etiquetas
        # `nowayland,novulkan` que llevaba esto eran de Gio y en Fyne no las lee nadie.
        # `-trimpath` por GOFLAGS: `fyne build` solo lo añade con --release.
        # `-buildmode=pie` también (FYP-06): sin él, el ejecutable sale `Type: EXEC`, sin
        # ASLR. Lo comprueba verificar_pie al publicarlo.
        CC="$CC_DESTINO" GOFLAGS="-trimpath -buildmode=pie" CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
          compilar_fyne "$exe"
        ;;
      windows)
        # EXPORTADAS, no delante del `go build`: en Windows `fyne package`
        # RECOMPILA aunque reciba `--executable`, para enlazar el .syso con el
        # icono, y toma CC y GOARCH del entorno. Con las variables solo en esta
        # línea, su recompilación salía con el gcc nativo del runner ARM64 y moría
        # en «windows.h: No such file or directory» (release-desktop, v3.5.5); y
        # el .syso se habría escrito para arm64. El subshell las acota a esta app.
        #
        # `-l:libssp.a`: esa recompilación añade `-fstack-protector-strong
        # -D_FORTIFY_SOURCE=3` (build.go de la CLI), y el MinGW 10 de Debian no
        # trae `__stack_chk_fail` propio: sin libssp el enlazado de GLFW falla. El
        # nombre de fichero y no `-lssp`, porque `ld` preferiría `libssp.dll.a` y
        # el .exe pediría en Windows una libssp-0.dll que nadie instala.
        # GOFLAGS también: la recompilación de `fyne package` solo pone -trimpath con
        # --release, y es su binario el que se publica.
        # shellcheck disable=SC2030,SC2031 # a propósito: el subshell acota el export a esta app
        export CC="$CC_DESTINO" CGO_ENABLED=1 GOOS=windows GOARCH=amd64 GOFLAGS=-trimpath \
          CGO_LDFLAGS="-L$ESCENARIO/gl-diferido ${CGO_LDFLAGS:-} -l:libssp.a -ldelayimp"
        go build -tags goolm -trimpath -ldflags "-H windowsgui" -o "$exe" .
        ;;
      macos)
        # La variante de la Mac App Store, con su etiqueta: sin actualizador propio y con
        # el llavero, la carpeta compartida y el panel de abrir que valen dentro del sandbox.
        etiquetas=()
        [ -z "$tienda" ] || etiquetas=(--tags "tienda,goolm")
        compilar_mac "$exe" ${etiquetas[@]+"${etiquetas[@]}"}
        ;;
      esac
      "$CLI" package --tags goolm "${comunes[@]}" --executable "$(basename "$exe")"
    )
    rm -f "$exe"
    ;;
  iossimulator)
    app_sim="$dir/$nombre.app"
    mkdir -p "$app_sim"
    (cd "$dir" && compilar_ios iphonesimulator "$app_sim/$ejecutable")
    plist_simulador "$dir" >"$app_sim/Info.plist"
    cp "$dir/$(sed -n 's/^[[:space:]]*Icon[[:space:]]*=[[:space:]]*"\(.*\)".*/\1/p' "$dir/FyneApp.toml" | head -1)" "$app_sim/Icon.png"
    ;;
  android | ios)
    # FUERA EL SYSROOT DEL ANFITRIÓN. Quien compile aquí puede venir de haber
    # hecho `source ~/fyne-sdk/entorno.sh` para el destino de Linux, y eso deja
    # `CGO_LDFLAGS` apuntando a las bibliotecas x86_64 del anfitrión. El clang
    # del NDK las coge y muere con «libGLESv2.so is incompatible with
    # aarch64linux», que no dice en ningún sitio que sobre una variable. Medido
    # en el piloto el 2026-09-21.
    unset CGO_CFLAGS CGO_LDFLAGS PKG_CONFIG_PATH PKG_CONFIG_LIBDIR PKG_CONFIG_SYSROOT_DIR

    # Aquí compila la herramienta: una .so por ABI dentro del paquete, que no se
    # puede pre-construir con `go build`.
    extra=()
    # `--certificate`: sin él la CLI deja CODE_SIGN_IDENTITY vacío en el proyecto que
    # genera. El perfil es el de ESTA app (su App ID), no uno para todas.
    if [ "$plataforma" = ios ]; then
      perfil_ios="${SYTUE_IOS_PROFILE:-}"
      [ -n "$perfil_ios" ] || perfil_ios="$(bash "$AQUI/apple-firma.sh" perfil "$appid")" || exit 1
      echo "   perfil: $perfil_ios"
      extra=(--profile "$perfil_ios" --certificate "$SYTUE_IOS_IDENTIDAD" --release)
    fi
    # SIN `--src`: en móvil la herramienta lo rechaza («parameter --source-dir is
    # currently not supported for mobile builds») y hay que estar EN el
    # directorio del paquete principal. Medido el 2026-09-21.
    # `-checklinkname=0`: `wlynxg/anet` (lo que pion usa para listar las interfaces de
    # red en Android, vía la llamada nativa de Rooms) enlaza por `//go:linkname` con
    # `net.zoneCache`, y Go 1.27 lo rechaza al enlazar («invalid reference to
    # net.zoneCache»). Medido en el ADR 0046. Va por GOFLAGS porque `fyne package` no
    # pasa `-ldflags` en móvil sin `--release`, y Go aplica GOFLAGS salvo que la orden
    # lo pise. Se quita cuando anet deje de hacerlo.
    #
    # PÁGINAS DE 16 KiB: Play las exige a toda app con target 35 o más (Android 15).
    # `fyne release` pondría este mismo enlazado; con `fyne package` hay que pedirlo, y
    # se verifica en la .so antes de armar el AAB (`android-binario.py alineacion`).
    entorno_movil=(GOFLAGS=)
    [ "$plataforma" = android ] &&
      entorno_movil=(GOFLAGS=-ldflags=-checklinkname=0 "CGO_LDFLAGS=-Wl,-z,max-page-size=16384")
    [ "$plataforma" != ios ] || entorno_movil+=("XCODE_XCCONFIG_FILE=$ESCENARIO/ios.xcconfig")
    (cd "$dir" && env "${entorno_movil[@]}" "$CLI" package --tags goolm "${comunes[@]}" ${extra[@]+"${extra[@]}"})
    ;;
  esac

  # `fyne package` no tiene `-o`: deja el artefacto al lado del fuente con el
  # nombre de la app. Se recoge por glob y se renombra a la forma que el portal
  # sabe leer (`sytue-<producto>-<plataforma>…`, ver descargas.ts). En escritorio va
  # al ESCENARIO, no a `dist/`: de ahí sale el bundle, que es lo que se publica, y
  # `caja` es el sitio donde ese bundle junta lo de sus apps.
  b="$(bundle_de "$dir")"
  caja="$ESCENARIO/bundles/$b"
  case "$plataforma" in linux | windows | macos) mkdir -p "$caja" ;; esac
  case "$plataforma" in
  linux) producido="$(find "$dir" -maxdepth 1 -name '*.tar.xz' | head -1)"; destino="$ESCENARIO/sytue-$app-linux-amd64.tar.xz" ;;
  windows) producido="$(find "$dir" -maxdepth 1 -name '*.exe' | head -1)"; destino="$ESCENARIO/sytue-$app-windows-amd64.exe" ;;
  macos) producido="$(find "$dir" -maxdepth 1 -name '*.app' | head -1)"; destino="$ESCENARIO/sytue-$app-macos-universal.app" ;;
  # El APK es un paso intermedio: lo que se publica es el AAB, que sale de él más abajo.
  android) producido="$(find "$dir" -maxdepth 1 -name '*.apk' | head -1)"; destino="$ESCENARIO/sytue-$app-android.apk" ;;
  # `fyne package` deja un .app (el .ipa es de `fyne release`): se recoge lo que haya y
  # más abajo sale SIEMPRE un .ipa, que es lo único que acepta App Store Connect.
  ios) producido="$(find "$dir" -maxdepth 1 \( -name '*.ipa' -o -name '*.app' \) | head -1)"; destino="$ESCENARIO/sytue-$app-ios.${producido##*.}" ;;
  iossimulator) producido="$app_sim"; destino="$SALIDA/sytue-$app-iossimulator.app" ;;
  esac
  [ -n "$producido" ] && [ -e "$producido" ] || {
    echo "::error::fyne package terminó bien y no dejó ningún artefacto en $dir" >&2
    exit 1
  }
  rm -rf "$destino"
  mv "$producido" "$destino"
  rm -f "$dir/AndroidManifest.xml"

  # --- QUE LAS DEMÁS APPS PUEDAN ABRIR ESTA -------------------------------------
  # El esquema ya está dentro del paquete en Android (el manifiesto de arriba). En
  # los otros tres destinos la herramienta no lo escribe, así que se añade al
  # artefacto ya construido, que es donde el sistema lo lee.
  case "$plataforma" in
  linux)
    # El .desktop del tar.xz sale con `Exec=<binario>` y sin MimeType, así que el
    # escritorio no sabe que este programa atiende `esquema://`. Se le añade el
    # manejador y el `%u` que le pasa la URL al abrirlo.
    reempaque="$(mktemp -d)"
    tar -xf "$destino" -C "$reempaque"
    entrada="$(find "$reempaque" -name '*.desktop' | head -1)"
    [ -n "$entrada" ] || {
      echo "::error::el tar.xz de $app no trae ningún .desktop: no se podría registrar el esquema." >&2
      exit 1
    }
    # Y los tipos de documento de «Abrir con» van en la MISMA línea: `MimeType` es
    # una sola clave y una segunda la pisaría. `%U` sirve para los dos, porque el gestor
    # de ficheros le pasa la ruta o `file://…`, y los dos los entiende `sesion`.
    # `%U` y no `%u`: con varios ficheros seleccionados, el gestor los pasa todos.
    sed -i.bak "s|^Exec=.*|Exec=$ejecutable %U|" "$entrada" && rm -f "$entrada.bak"
    printf 'MimeType=x-scheme-handler/%s;%s\n' "$esquema" "$(printf '%s' "$tipos" | tr '\n' ';' | sed 's/;*$/;/;s/^;$//')" >>"$entrada"
    # El Makefile de la CLI no viaja: el bundle trae su `instalar.sh`, que no pide
    # `make` (no viene en muchos escritorios) y registra las catorce apps de una vez.
    icono="$(find "$reempaque" -path '*/share/pixmaps/*' -type f | head -1)"
    # El binario va en <ejecutable>/usr/local/bin/ dentro del paquete (o en usr/bin
    # si el runner no tiene /usr/local): se busca, no se supone.
    binario="$(find "$reempaque" -path '*/bin/*' -type f | head -1)"
    [ -n "$icono" ] && [ -n "$binario" ] || {
      echo "::error::el tar.xz de $app no trae icono o ejecutable" >&2; exit 1; }
    # Suelo de glibc: hay DOS runners (Debian 12 cruzando desde ARM64 → glibc 2.36;
    # Ubuntu 24.04 nativo en AMD64 → 2.39) y el guardián de arquitectura no ve la
    # diferencia. Que el runner que gane la carrera no decida dónde arranca la app:
    # Debian 12 / Ubuntu 22.04 son el suelo. Nativo en Ubuntu 24.04 exigiría 2.38 (GLFW
    # usa -D_GNU_SOURCE → __isoc23_strtol): por eso release-desktop corre en node:24-bookworm.
    # El objdump NATIVO de un runner ARM64 no lee ELF x86-64 («file format not
    # recognized») y, con pipefail, el grep vacío mataba el guion sin decir nada
    # (release-desktop v3.5.12, Linux): se usa el del compilador cruzado si está,
    # y si no se puede leer se avisa en vez de morir.
    lector="$(command -v x86_64-linux-gnu-objdump || command -v objdump || true)"
    glibc=""
    [ -z "$lector" ] || glibc="$("$lector" -T "$binario" 2>/dev/null |
      grep -oE 'GLIBC_[0-9]+\.[0-9]+' | sort -uV | tail -1 || true)"
    case "$glibc" in
      "") echo "::warning::no se pudo leer la versión de glibc que exige $destino (lector: ${lector:-ninguno})" >&2 ;;
      GLIBC_2.3[7-9]|GLIBC_2.[4-9][0-9])
        echo "::error::$destino exige $glibc: no arranca en Debian 12 / Ubuntu 22.04. ¿Se compiló nativo en Ubuntu 24.04?" >&2
        exit 1 ;;
      *) echo "  glibc mínima: ${glibc#GLIBC_}" ;;
    esac
    # AL BUNDLE, cada cosa donde la busca `instalar.sh`: el programa en bin/, su
    # entrada del menú (con el esquema y los tipos de arriba) y su icono.
    mkdir -p "$caja/sytue-$b/bin" "$caja/sytue-$b/share/applications" "$caja/sytue-$b/share/pixmaps"
    cp "$binario" "$caja/sytue-$b/bin/$ejecutable"
    cp "$entrada" "$caja/sytue-$b/share/applications/"
    cp "$icono" "$caja/sytue-$b/share/pixmaps/"
    rm -rf "$reempaque"
    ;;
  macos | ios | iossimulator)
    # El esquema, los tipos de documento y los permisos, en el Info.plist del bundle
    # (`plist_apple`). Un .ipa (iOS con `--release`) sale YA FIRMADO: se abre, se edita su .app y se
    # vuelve a firmar al final con los derechos que la plantilla de la CLI no pone
    # (`firmar_ipa`). Sin esto el IPA salía sin esquema, sin las descripciones de los
    # permisos —y iOS mata la app al pedir el micrófono— y sin los Universal Links.
    bundle_ios="$destino"
    ipa_abierto=""
    if [ "$plataforma" = ios ]; then
      ipa_abierto="$(mktemp -d)"
      if [ "${destino##*.}" = ipa ]; then
        unzip -q "$destino" -d "$ipa_abierto"
      else
        # El .app firmado por xcodebuild, a la forma de un IPA: Payload/<Nombre>.app.
        mkdir "$ipa_abierto/Payload"
        mv "$destino" "$ipa_abierto/Payload/$nombre.app"
      fi
      bundle_ios="$(find "$ipa_abierto/Payload" -maxdepth 1 -name '*.app' | head -1)"
      destino="$SALIDA/sytue-$app-ios.ipa"
    fi
    plist="$destino/Contents/Info.plist"
    [ "$plataforma" = macos ] || plist="$bundle_ios/Info.plist"
    if [ -f "$plist" ]; then
      recursos="$bundle_ios"
      [ "$plataforma" != macos ] || recursos="$destino/Contents/Resources"
      plist_apple "$plataforma" "$dir" "$plist" "$recursos" || exit 1
      # La CLI no copia el cumplimiento de exportación del proyecto de Xcode.
      # La misma declaración autorizada llega al artefacto ANTES de volver a firmarlo.
      if [ "$plataforma" = ios ] || [ -n "$tienda" ]; then
        copiar_cifrado_apple "$FYNE/xcode/Info-$plataforma.plist" "$plist" || exit 1
      fi
      [ -z "$ipa_abierto" ] || firmar_ipa "$ipa_abierto" "$bundle_ios" "$destino" "$appid" "$host_enlaces" "$dir"
      # macOS con Developer ID (`SYTUE_MACOS_IDENTIDAD`): runtime endurecido, que es lo que
      # pide la notarización, y con él los derechos de micrófono y cámara, sin los que
      # macOS niega los dos en silencio aunque la persona diga que sí. Sin identidad, el
      # .app sale como siempre (sin endurecer, y ahí no hacen falta).
      if [ -n "$tienda" ]; then
        # La Mac App Store: sandbox, perfil embebido y certificado de distribución.
        firmar_tienda "$destino" "$dir" "$appid"
      elif [ "$plataforma" = macos ] && [ -n "${SYTUE_MACOS_IDENTIDAD:-}" ]; then
        derechos="$(mktemp)"
        if [ "$app" != demo ]; then
          # App Group y Keychain de la suite son derechos restringidos fuera de la tienda:
          # piden un perfil Developer ID (MAC_APP_DIRECT) que los autorice, dentro del .app.
          perfil="$(bash "$AQUI/apple-firma.sh" perfil-ruta "$appid" developerid)" || exit 1
          grupos_sesion_apple || exit 1
          equipo="${GRUPO_LLAVERO_SUITE%%.*}"
          derechos_sesion "$dir" "$equipo" developerid >"$derechos" || exit 1
          grupo_perfil="$GRUPO_CONTENEDOR_SUITE"
          bash "$AQUI/apple-firma.sh" verificar-perfil-grupo "$perfil" "$appid" developerid "$grupo_perfil" "" "$GRUPO_LLAVERO_SUITE" "$derechos" || exit 1
          install -m 0644 "$perfil" "$destino/Contents/embedded.provisionprofile"
          incorporar_widgets_apple "$destino" "$dir" developerid "$SYTUE_MACOS_IDENTIDAD" || exit 1
        else
          # Sin el interruptor, la firma de siempre: sin perfil y solo los derechos de las
          # llamadas. La sesión se queda en el llavero de cada app (compartido_darwin.go
          # mira en tiempo de ejecución si la firma trae el grupo de la suite).
          {
            printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0"><dict>\n'
            [ "$llamadas" != si ] || printf '\t<key>com.apple.security.device.audio-input</key><true/>\n\t<key>com.apple.security.device.camera</key><true/>\n'
            printf '</dict></plist>\n'
          } >"$derechos"
        fi
        codesign -f -s "$SYTUE_MACOS_IDENTIDAD" --options runtime --timestamp --entitlements "$derechos" "$destino"
        rm -f "$derechos"
        # Sin tubería hacia `grep -q`: con pipefail, grep corta al primer acierto y
        # el acierto sale como fallo (ci-sin-azar-test.sh).
        [ "$llamadas" != si ] || grep -q 'device.audio-input' <<<"$(codesign -d --entitlements - "$destino" 2>/dev/null)" || {
          echo "::error::$destino firmado sin el derecho de micrófono: la llamada no oiría a nadie." >&2
          exit 1
        }
      fi
    else
      echo "::error::$destino no trae Info.plist: ni esquema, ni permisos, ni enlaces." >&2
      exit 1
    fi
    ;;
  esac

  # iOS es el único destino cuyo contenido no es un binario de Go legible desde
  # aquí (el ejecutable va dentro del bundle firmado): se verifica al instalarlo.
  if [ "$plataforma" != ios ]; then
    esperada="$arch"
    # En el universal de macOS el GOARCH grabado es el de una de las dos mitades;
    # quien manda es `lipo`, que ve las dos.
    if [ "$plataforma" = iossimulator ]; then
      # Firma AD HOC después de tocar el plist: el Simulador la exige y no mira quién firma.
      codesign -f -s - "$destino" >/dev/null
      verificar_arquitectura "$destino/$ejecutable" arm64
    elif [ "$plataforma" = macos ]; then
      archs="$(lipo -archs "$destino/Contents/MacOS/"* 2>/dev/null | tr ' ' '\n' | sort | tr '\n' ' ')"
      [ "$archs" = "arm64 x86_64 " ] || {
        echo "::error::$destino no es universal: lipo ve «$archs» y se declaró arm64 + x86_64." >&2
        exit 1
      }
      echo "  arquitectura verificada: universal (arm64 + x86_64)"
    else
      verificar_arquitectura "$destino" "$esperada"
      if [ "$plataforma" = linux ] && [ -f "$destino" ]; then
        verificar_pie "$destino" || exit 1
      fi
    fi
  fi

  # --- ANDROID: verificar el paquete, no fiarse de que la orden saliera en 0 ----
  # Tres cosas que solo se ven mirando dentro del APK, y las tres se han roto ya
  # en la cadena anterior: que el deep link esté declarado (si no, ninguna app
  # abre a esta y nadie se entera hasta que alguien pulsa un enlace), que no pida
  # permisos que nadie ha justificado, y que la biblioteca que la actividad busca
  # sea la que está dentro (si no, arranca y se cierra).
  if [ "$plataforma" = android ]; then
    aapt2="$AAPT2"
    if [ -n "$aapt2" ] && [ -x "$aapt2" ]; then
      manifiesto="$("$aapt2" dump xmltree --file AndroidManifest.xml "$destino" 2>/dev/null || true)"
      [ -z "$host_enlaces" ] || grep -q "\"$host_enlaces\"" <<<"$manifiesto" || {
        echo "::error::$destino no declara el App Link de $host_enlaces: los enlaces de reunión no abrirían la app." >&2
        exit 1
      }
      grep -q "\"$esquema\"" <<<"$manifiesto" || {
        echo "::error::$destino no declara el esquema $esquema: ninguna otra app podría abrirla." >&2
        exit 1
      }
      # Compartir la pantalla: sin el servicio de tipo mediaProjection declarado, Android
      # lanza una excepción al pulsar el botón. El permiso lo pide un fragmento
      # (PantallaPermiso), no una actividad: la CLI de Fyne solo empaqueta la
      # GoNativeActivity y rechaza cualquier otra.
      [ "$llamadas" != si ] || grep -q '"eu.sytue.nativo.PantallaServicio"' <<<"$manifiesto" || {
        echo "::error::$destino no declara PantallaServicio: compartir la pantalla fallaría." >&2
        exit 1
      }
      # POST_NOTIFICATIONS es el de los avisos de Android 13+: sin él el sistema se los
      # calla sin error (`escritorio.PrepararAvisos` lo pide en tiempo de ejecución).
      pedidos="$(permisos_pedidos <<<"$manifiesto")"
      justificados="$(printf '%s\n' android.permission.INTERNET android.permission.POST_NOTIFICATIONS ${permisos_extra[@]+"${permisos_extra[@]}"} | sort -u | tr '\n' ' ')"
      [ "$pedidos" = "$justificados" ] || {
        echo "::error::$destino pide «$pedidos» y lo único justificado es «$justificados»." >&2
        echo "  Los permisos «por si acaso» están prohibidos (encargo v4 §4)." >&2
        exit 1
      }
      # El volcado de aapt2 pone el nombre del meta-dato en una línea y su valor
      # en la SIGUIENTE, y las dos llevan `Raw: "…"`: un `sed` de una línea coge
      # el nombre («android.app.lib_name») y busca una .so que no existe.
      lib="$(awk '/"android\.app\.lib_name"/ {v=1; next}
                  v && /:value/ { sub(/.*Raw: "/, ""); sub(/".*/, ""); print; exit }' <<<"$manifiesto")"
      [ -n "$lib" ] || {
        echo "::error::$destino no declara android.app.lib_name." >&2
        exit 1
      }
      unzip -Z1 "$destino" "lib/arm64-v8a/lib$lib.so" >/dev/null 2>&1 || {
        echo "::error::$destino busca lib$lib.so y dentro no está: la actividad arrancaría y se cerraría." >&2
        exit 1
      }
      echo "  esquema $esquema, solo los permisos justificados y lib$lib.so comprobados"
    else
      echo "::error::sin aapt2 no se puede comprobar el esquema ni los permisos de $destino." >&2
      exit 1
    fi
    aab="$SALIDA/sytue-$app-$(sufijo android-sin-firmar)"
    # Los servicios en primer plano y su tipo (FOREGROUND_SERVICE_TYPE_*): specialUse el
    # de avisos del Hub, mediaProjection el de compartir la pantalla de Rooms.
    servicios_fg=""
    [ -z "$servicio_avisos" ] || servicios_fg="eu.sytue.nativo.Avisos:0x40000000"
    [ "$llamadas" != si ] || servicios_fg="$servicios_fg eu.sytue.nativo.PantallaServicio:0x20"
    apk_a_aab "$destino" "$aab" "$lib" "$servicios_fg" "$pip" "$ESCENARIO/android-$app.xml" "$dir"
    verificar_aab "$aab" "$appid" "$esquema" "$servicios_fg" "$documentos" "$pip"
    rm -f "$destino"
    destino="$aab"
  fi

  # --- WINDOWS: lo de esta app en el instalador de su bundle ---------------------
  # En Windows un esquema de URL es una clave del registro, no un metadato del
  # ejecutable, así que sin instalador las apps no se pueden llamar entre sí. El del
  # bundle (NSIS, se arma después del bucle) pone todas las suyas; aquí se deja el
  # ejecutable en la caja y lo que el instalador hace con él, al instalar y al quitar.
  if [ "$plataforma" = windows ]; then
    cp "$destino" "$caja/$ejecutable.exe"
    # El que se publica es el que recompiló `fyne package`: se mira ese.
    opengl32_diferida "$caja/$ejecutable.exe" || exit 1
    [ -f "$caja/icono" ] || printf '%s.exe' "$ejecutable" >"$caja/icono"
    # La que da nombre al bundle (`sytue-one` es el Hub) pone el icono de su entrada en
    # «Aplicaciones instaladas»; si no hay ninguna, la primera.
    [ "$esquema" != "sytue-$b" ] || printf '%s.exe' "$ejecutable" >"$caja/icono"
    cat >>"$caja/instalar.nsh" <<NSI
  ; --- $nombre ($esquema://) ---
  ; Lo que dejó su instalador SUELTO (uno por app, hasta que llegaron los bundles), en
  ; silencio y antes de escribir nada: quita sus claves y su carpeta, y así no quedan dos.
  ReadRegStr \$0 HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\$appid" "UninstallString"
  \${If} \$0 != ""
    \${GetParent} \$0 \$1
    ExecWait '"\$0" /S _?=\$1'
    Delete "\$0"
    RMDir "\$1"
    SetOutPath "\$INSTDIR"
  \${EndIf}
  ; Un ejecutable EN USO no se puede sobrescribir, pero sí renombrar: se aparta el viejo
  ; y se pone el nuevo al lado. Así el Hub pone al día las demás apps en silencio (/S)
  ; aunque estén abiertas, que siguen con el viejo hasta cerrarse.
  Delete "\$INSTDIR\\$ejecutable.exe.viejo"
  Rename "\$INSTDIR\\$ejecutable.exe" "\$INSTDIR\\$ejecutable.exe.viejo"
  File "/oname=$ejecutable.exe" "$caja/$ejecutable.exe"
  Delete /REBOOTOK "\$INSTDIR\\$ejecutable.exe.viejo"
  CreateShortCut "\$SMPROGRAMS\\$nombre.lnk" "\$INSTDIR\\$ejecutable.exe"
  ; EL PROTOCOLO: es lo que permite que otra app de la suite abra esta con
  ; $esquema://…  Va en HKCU porque el instalador es por usuario (sin UAC).
  WriteRegStr HKCU "Software\\Classes\\$esquema" "" "URL:$nombre"
  WriteRegStr HKCU "Software\\Classes\\$esquema" "URL Protocol" ""
  WriteRegStr HKCU "Software\\Classes\\$esquema\\DefaultIcon" "" "\$INSTDIR\\$ejecutable.exe,0"
  WriteRegStr HKCU "Software\\Classes\\$esquema\\shell\\open\\command" "" '"\$INSTDIR\\$ejecutable.exe" "%1"'
$(nsis_abrir_con "$extensiones" "$appid" "$nombre" "$ejecutable")
  ; LA AUMID: una app sin paquete no puede enseñar toasts con un identificador que
  ; Windows no conoce, y los avisos (los de Fyne y los que llevan clic en
  ; apps/nucleo/sistema/escritorio) usan el de la app. Es lo que registra también
  ; ToastNotificationManagerCompat para las apps Win32, sin acceso directo con AUMID.
  WriteRegStr HKCU "Software\\Classes\\AppUserModelId\\$appid" "DisplayName" "$nombre"
NSI
    cat >>"$caja/quitar.nsh" <<NSI
  Delete "\$INSTDIR\\$ejecutable.exe"
  Delete "\$INSTDIR\\$ejecutable.exe.viejo"
  Delete "\$SMPROGRAMS\\$nombre.lnk"
  DeleteRegKey HKCU "Software\\Classes\\$esquema"
  DeleteRegKey HKCU "Software\\Classes\\AppUserModelId\\$appid"
$(nsis_quitar_abrir_con "$extensiones" "$appid" "$nombre")
NSI
  fi

  # macOS: el .app, a la caja de su bundle con el nombre de la app, que es lo que el
  # Finder enseña en /Aplicaciones y en el Dock. El nombre del portal lo lleva el .dmg.
  if [ -n "$tienda" ]; then
    # LA MAC APP STORE: un .pkg POR APP, firmado con el certificado de INSTALADOR (no con
    # el de la app). Dentro va el .app con el nombre que verá el Finder, a /Applications.
    mkdir -p "$ESCENARIO/tienda/$app"
    mv "$destino" "$ESCENARIO/tienda/$app/$nombre.app"
    destino="$SALIDA/sytue-$app-$(sufijo macappstore)"
    rm -f "$destino"
    productbuild --component "$ESCENARIO/tienda/$app/$nombre.app" /Applications \
      --sign "$SYTUE_MAS_INSTALADOR" "$destino" >/dev/null
    firma_pkg="$(pkgutil --check-signature "$destino" 2>&1)" || {
      echo "::error::$destino no va firmado: $firma_pkg" >&2
      exit 1
    }
    grep -qF "$SYTUE_MAS_INSTALADOR" <<<"$firma_pkg" || {
      echo "::error::$destino no lo firma «$SYTUE_MAS_INSTALADOR»: $firma_pkg" >&2
      exit 1
    }
  elif [ "$plataforma" = macos ]; then
    rm -rf "$caja/$nombre.app"
    mv "$destino" "$caja/$nombre.app"
    destino="$caja/$nombre.app"
  fi

  echo "✓ $destino"
done

# --- LOS BUNDLES: lo único que se publica en escritorio --------------------------
case "$plataforma" in linux | windows | macos)
  for caja in "$ESCENARIO"/bundles/*/; do
    # La Mac App Store no lleva bundles: su .pkg por app ya está en `dist/`.
    [ -z "$tienda" ] || break
    caja="${caja%/}"
    b="$(basename "$caja")"
    nb="$(nombre_bundle "$b")"
    publicado="$SALIDA/sytue-$b-$(sufijo "$plataforma")"
    rm -rf "$publicado"
    case "$plataforma" in
    linux)
      # Una carpeta `sytue-<bundle>/` con bin/, share/ y el instalador: se descomprime
      # donde sea y `./instalar.sh` (o `sudo ./instalar.sh`) lo registra todo.
      instalar_sh "$nb" >"$caja/sytue-$b/instalar.sh"
      chmod 0755 "$caja/sytue-$b/instalar.sh"
      tar -C "$caja" -cJf "$publicado" "sytue-$b"
      ;;
    windows)
      # Un instalador por usuario (sin UAC) en %LOCALAPPDATA%\Programs\<bundle>, con
      # una sola entrada en «Aplicaciones instaladas»: la versión y la carpeta que
      # dejan ahí son las que leen `escritorio.Hermanas` y el propio instalador al
      # ponerse al día (`InstallDirRegKey`).
      clave="eu.sytue.bundle.$b"
      desinstalar="Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\$clave"
      nsi="$caja/bundle.nsi"
      # Primero los .exe de las apps, que el instalador mete tal cual; el desinstalador lo
      # escribe makensis y se firma por `!uninstfinalize` (NSIS ≥ 3.08) antes de meterlo.
      firmar_desinstalador=""
      if [ -n "$FIRMAR_WIN" ]; then
        bash "$FIRMAR_WIN" firmar "$caja"/*.exe
        firmar_desinstalador="!uninstfinalize 'bash \"$FIRMAR_WIN\" firmar \"%1\"' = 0"
      fi
      cat >"$nsi" <<NSI
Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
RequestExecutionLevel user
ManifestDPIAware true
SetCompressor /SOLID lzma
Name "$nb"
OutFile "$publicado"
InstallDir "\$LOCALAPPDATA\\Programs\\$nb"
; Si ya estaba instalado, en la MISMA carpeta: el Hub lo pone al día sin preguntar (/S).
InstallDirRegKey HKCU "$desinstalar" "InstallLocation"
BrandingText "Sytue"
$firmar_desinstalador
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Spanish"

Section "Instalar"
  SetOutPath "\$INSTDIR"
$(cat "$caja/instalar.nsh")
$(nsis_mesa "$ESCENARIO/mesa")
  ; UNA entrada para el bundle. Versión y carpeta: con ellas el Hub encuentra las
  ; apps de la suite y sabe si están al día (escritorio.Hermanas).
  WriteRegStr HKCU "$desinstalar" "DisplayName" "$nb"
  WriteRegStr HKCU "$desinstalar" "Publisher" "Sytue"
  WriteRegStr HKCU "$desinstalar" "DisplayIcon" "\$INSTDIR\\$(cat "$caja/icono"),0"
  ; Entre comillas: la carpeta lleva espacio («Sytue One») y Windows probaría antes
  ; «…\Programs\Sytue.exe» (FYP-05).
  WriteRegStr HKCU "$desinstalar" "UninstallString" '"\$INSTDIR\\Desinstalar.exe"'
  WriteRegStr HKCU "$desinstalar" "QuietUninstallString" '"\$INSTDIR\\Desinstalar.exe" /S'
  WriteRegStr HKCU "$desinstalar" "DisplayVersion" "$version"
  WriteRegStr HKCU "$desinstalar" "InstallLocation" "\$INSTDIR"
  WriteUninstaller "\$INSTDIR\\Desinstalar.exe"
SectionEnd

Section "Uninstall"
$(cat "$caja/quitar.nsh")
$(nsis_quitar_mesa)
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
  Delete "\$INSTDIR\\Desinstalar.exe"
  RMDir "\$INSTDIR"
  DeleteRegKey HKCU "$desinstalar"
SectionEnd
NSI
      makensis -V2 "$nsi"
      [ -z "$FIRMAR_WIN" ] || bash "$FIRMAR_WIN" firmar "$publicado"
      ;;
    macos)
      # El DMG es lo que se instala arrastrando a /Aplicaciones, y un .app suelto
      # perdería los permisos al viajar en un zip. Todas las apps del bundle y, a su
      # lado, el alias a Aplicaciones.
      ln -s /Applications "$caja/Applications"
      # TAMAÑO EXPLÍCITO y HFS+. Con solo -srcfolder, hdiutil estima el tamaño de la
      # imagen y en Codemagic se queda corto: «hdiutil: create failed - No space left
      # on device» con 146 GiB libres en la máquina (v3.5.9 en la app 11, v3.5.12 en
      # la 1). Lo que ocupa la carpeta más un 20 % y 20 MB; HFS+ lo abre cualquier macOS.
      tam_mb="$(du -sm "$caja" | cut -f1)"
      hdiutil create -volname "$nb" -srcfolder "$caja" -ov -format UDZO \
        -fs HFS+ -size "$((tam_mb * 12 / 10 + 20))m" "$publicado" >/dev/null
      ;;
    esac
    echo "✓ $publicado: $nb"
  done
  ;;
esac

ls -la "$SALIDA"
