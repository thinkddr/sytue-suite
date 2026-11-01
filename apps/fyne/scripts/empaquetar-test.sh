#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# EL GUARDIÁN DE LA CADENA DE EMPAQUETADO, Y SE VE EN ROJO EN CADA EJECUCIÓN.
#
# Un guardián que nadie ha visto fallar no guarda nada. Este no se limita a
# comprobar el caso bueno: compila a propósito un binario de la arquitectura
# CONTRARIA y EXIGE que `empaquetar.sh verificar` lo rechace. Si algún día la
# comprobación se rompe —un `sed` que deja de casar, un `go version -m` que cambia
# de formato, alguien que la comenta «un momento»— este test se pone rojo por el
# caso negativo, no por el positivo.
#
# Por qué importa: los runners propios son ARM64 y el servidor es x86_64. Cuatro
# versiones se publicaron con la arquitectura del runner y ninguna puerta se puso
# en rojo, porque compilar y empaquetar salen bien igual.
#
#   bash apps/fyne/scripts/empaquetar-test.sh
#
# Solo necesita Go: ni CGO, ni servidor gráfico, ni la CLI de Fyne.

set -uo pipefail

AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FYNE="$(cd "$AQUI/.." && pwd)"
RAIZ="$(cd "$AQUI/../../.." && pwd)"
GUION="$AQUI/empaquetar.sh"
fallos=0
mal() {
  echo "not ok - $1" >&2
  fallos=1
}

if ! grep -qx 'replace fyne.io/fyne/v2 => github.com/thinkddr/fyne/v2 v2.8.2-sytue.2' "$FYNE/go.mod"; then
  mal "apps/fyne debe usar el fork mantenido con URL entrantes y geometría UIKit de Fyne v2.8.2"
fi

command -v go >/dev/null 2>&1 || {
  echo "not ok - falta Go" >&2
  exit 1
}
[ -x "$GUION" ] || mal "apps/fyne/scripts/empaquetar.sh no es ejecutable"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Un programa mínimo: lo que se mide es la cabecera del binario, no lo que hace.
cat >"$tmp/main.go" <<'GO'
package main

func main() {}
GO
cat >"$tmp/go.mod" <<'GO'
module ejemplo

go 1.27.3
GO

construir() { # <goos> <goarch> -> ruta
  (cd "$tmp" && CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -o "$tmp/bin-$1-$2" .) >/dev/null 2>&1 &&
    echo "$tmp/bin-$1-$2"
}

amd64="$(construir linux amd64)"
arm64="$(construir linux arm64)"
[ -n "$amd64" ] && [ -n "$arm64" ] || {
  echo "not ok - no se pudieron compilar los dos binarios de prueba" >&2
  exit 1
}

# --- 1. El caso bueno pasa ----------------------------------------------------
"$GUION" verificar "$amd64" amd64 >/dev/null 2>&1 ||
  mal "el guardián rechaza un binario amd64 declarado amd64: no dejaría publicar nada"

# --- 2. EL CASO MALO FALLA, que es lo que se está comprobando -----------------
# Exactamente el fallo real: un binario ARM64 pasando por amd64.
if "$GUION" verificar "$arm64" amd64 >"$tmp/salida" 2>&1; then
  mal "el guardián dio por bueno un binario arm64 declarado amd64: es el fallo de las cuatro versiones sin desplegar"
else
  grep -q 'es arm64 y se declaró amd64' "$tmp/salida" ||
    mal "el guardián falla pero no dice qué vio ni qué esperaba: $(head -1 "$tmp/salida")"
fi

# --- 3. Y falla también cuando no puede leer la arquitectura ------------------
# El silencio es el peor resultado posible: «no lo sé» tiene que ser rojo, no
# verde. Un fichero que no es un binario de Go es exactamente ese caso.
echo "esto no es un binario" >"$tmp/no-es-binario"
"$GUION" verificar "$tmp/no-es-binario" amd64 >/dev/null 2>&1 &&
  mal "el guardián da por buena una arquitectura que no ha podido leer"

# --- 4. La CLI de Fyne va fijada, nunca desde @latest -------------------------
# Con `@latest` el mismo tag empaquetaba con una herramienta distinta según el día.
# La versión la fija versions.env y nadie la instala desde una referencia mutable.
grep -qE '^FYNE_TOOLS_VERSION=[0-9]+\.[0-9]+\.[0-9]+$' "$RAIZ/deploy/sytue-git/versions.env" ||
  mal "deploy/sytue-git/versions.env no fija FYNE_TOOLS_VERSION=X.Y.Z"
if mutable="$(grep -n 'fyne.io/tools/cmd/fyne@latest' "$GUION" "$RAIZ"/.github/workflows/*.yml)"; then
  mal "la CLI de Fyne se instala desde @latest: $mutable"
fi

# --- 5. Cada app empaquetable declara sus metadatos ---------------------------
# `FyneApp.toml` es lo que hace que un directorio de `cmd/` entre en la cadena, y
# lo que lleva el identificador de paquete. Un `cmd/` sin él se queda fuera en
# silencio, que es como se publica una release a la que le falta una app.
for dir in "$RAIZ"/apps/fyne/cmd/*/; do
  app="$(basename "$dir")"
  if [ ! -f "$dir/FyneApp.toml" ]; then
    mal "apps/fyne/cmd/$app no tiene FyneApp.toml: la cadena de empaquetado lo ignora sin decir nada"
    continue
  fi
  grep -qE '^\s*ID\s*=\s*"eu\.sytue\.' "$dir/FyneApp.toml" ||
    mal "apps/fyne/cmd/$app no declara un ID eu.sytue.*: no se puede cambiar una vez subido a una tienda"
  icono="$(sed -n 's/^[[:space:]]*Icon[[:space:]]*=[[:space:]]*"\(.*\)"[[:space:]]*$/\1/p' "$dir/FyneApp.toml")"
  [ -n "$icono" ] || {
    mal "apps/fyne/cmd/$app no declara Icon: fyne package se niega a empaquetar sin icono"
    continue
  }
  [ -f "$dir/$icono" ] ||
    mal "apps/fyne/cmd/$app apunta a un icono que no existe ($icono)"
done

# --- 6. «Abrir con»: lo que se declara al sistema es lo que la app abre ---------
# Docus se ofrece para documentos y plantillas con su tipo MIME, que es lo que leen el
# `.desktop` y el `intent-filter`.
quiero="application/vnd.openxmlformats-officedocument.wordprocessingml.document
application/vnd.openxmlformats-officedocument.wordprocessingml.template
application/vnd.oasis.opendocument.text
application/vnd.oasis.opendocument.text-template
application/vnd.apple.pages"
[ "$("$GUION" tipos docus 2>&1)" = "$quiero" ] ||
  mal "empaquetar.sh tipos docus no da los cinco tipos de Docus: $("$GUION" tipos docus 2>&1 | tr '\n' ' ')"
quiero="application/vnd.apple.numbers
application/vnd.oasis.opendocument.spreadsheet
application/vnd.oasis.opendocument.spreadsheet-template
application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
application/vnd.openxmlformats-officedocument.spreadsheetml.template"
[ "$("$GUION" tipos calcus 2>&1)" = "$quiero" ] || mal "Calcus no declara los tipos MIME de documentos y plantillas"
quiero="application/vnd.apple.keynote
application/vnd.oasis.opendocument.presentation
application/vnd.oasis.opendocument.presentation-template
application/vnd.openxmlformats-officedocument.presentationml.template
application/vnd.openxmlformats-officedocument.presentationml.presentation"
[ "$("$GUION" tipos diapos 2>&1)" = "$quiero" ] || mal "Diapos no declara los tipos MIME de documentos y plantillas"
# CertiPDF abre PDF: sin el tipo estándar, Linux y Android no lo registran en
# «Abrir con» y el empaquetado debe detectarlo antes de publicar un artefacto roto.
[ "$("$GUION" tipos certipdf 2>&1)" = 'application/pdf' ] ||
  mal "empaquetar.sh tipos certipdf no da application/pdf: $("$GUION" tipos certipdf 2>&1 | tr '\n' ' ')"
# Y una extensión sin tipo conocido FALLA: saltársela dejaría la app fuera de «Abrir
# con» para ese formato sin que nadie se entere.
mkdir -p "$tmp/rara"
printf '[Development]\n  extensiones = "docx,zzz"\n' >"$tmp/rara/FyneApp.toml"
"$GUION" tipos "$tmp/rara" >/dev/null 2>&1 &&
  mal "una extensión sin tipo MIME conocido se ha dado por buena"

# --- 6b. Y POR DEFECTO: macOS y Windows ------------------------------------------------
# macOS: cada extensión es un tipo de documento del Info.plist con su UTI y con
# `LSHandlerRank Owner`; sin eso el Finder no la ofrece ni deja ponerla por defecto. Una
# extensión sin UTI conocido falla, como sin tipo MIME. Visto en rojo quitando `pptx` de
# `uti_de` y la línea de LSHandlerRank.
for par in calcus:org.oasis-open.opendocument.spreadsheet calcus:org.openxmlformats.spreadsheetml.sheet \
  docus:org.oasis-open.opendocument.text docus:org.openxmlformats.wordprocessingml.document \
  diapos:org.oasis-open.opendocument.presentation diapos:org.openxmlformats.presentationml.presentation \
  calcus:org.oasis-open.opendocument.spreadsheet-template calcus:org.openxmlformats.spreadsheetml.template \
  docus:org.oasis-open.opendocument.text-template docus:org.openxmlformats.wordprocessingml.template \
  diapos:org.oasis-open.opendocument.presentation-template diapos:org.openxmlformats.presentationml.template \
  certipdf:com.adobe.pdf; do
  a="${par%%:*}"
  ordenes="$("$GUION" documentos-mac "$a" 2>&1)" || { mal "empaquetar.sh documentos-mac $a falla: $(tail -1 <<<"$ordenes")"; continue; }
  grep -qE "^Add :CFBundleDocumentTypes:[0-9]+:LSItemContentTypes:0 string ${par#*:}\$" <<<"$ordenes" ||
    mal "el Info.plist de $a no declara el tipo ${par#*:}"
done
[ "$("$GUION" documentos-mac diapos 2>&1 | grep -c 'LSHandlerRank string Owner$')" = 4 ] ||
  mal "los tipos de Diapos no van con LSHandlerRank Owner: no podría ser la predeterminada"
# Los de iWork se abren convertidos y no se guardan en su formato: se declaran para leer
# y sin quitarle el sitio a Pages, Numbers o Keynote.
for par in docus:com.apple.iwork.pages.sffpages calcus:com.apple.iwork.numbers.sffnumbers \
  diapos:com.apple.iwork.keynote.sffkey; do
  ordenes="$("$GUION" documentos-mac "${par%%:*}" 2>&1)"
  n="$(grep -E "LSItemContentTypes:0 string ${par#*:}\$" <<<"$ordenes" | sed -E 's/^Add :CFBundleDocumentTypes:([0-9]+):.*/\1/')"
  [ -n "$n" ] && grep -qx "Add :CFBundleDocumentTypes:$n:CFBundleTypeRole string Viewer" <<<"$ordenes" &&
    grep -qx "Add :CFBundleDocumentTypes:$n:LSHandlerRank string Alternate" <<<"$ordenes" ||
    mal "${par%%:*} no declara ${par#*:} como Viewer y Alternate"
done
grep -q UTImportedTypeDeclarations <<<"$("$GUION" documentos-mac certipdf 2>&1)" &&
  mal "CertiPDF importa com.adobe.pdf, que es del sistema"
[ -z "$("$GUION" documentos-mac hub 2>&1)" ] || mal "el Hub declara tipos de documento sin abrir ninguno"
"$GUION" documentos-mac "$tmp/rara" >/dev/null 2>&1 &&
  mal "una extensión sin UTI conocido se ha dado por buena en macOS"
# Windows: además de «Abrir con», Capabilities\FileAssociations y RegisteredApplications,
# que es lo que la pone en «Aplicaciones predeterminadas», y el desinstalador lo quita.
# Visto en rojo quitando la línea de RegisteredApplications del desinstalador.
reg="$("$GUION" registro-windows calcus 2>&1)"
for l in 'WriteRegStr HKCU "Software\Sytue\eu.sytue.calcus\Capabilities\FileAssociations" ".xlsx" "eu.sytue.calcus.documento"' \
  'WriteRegStr HKCU "Software\Sytue\eu.sytue.calcus\Capabilities\FileAssociations" ".ods" "eu.sytue.calcus.documento"' \
  'WriteRegStr HKCU "Software\Sytue\eu.sytue.calcus\Capabilities\FileAssociations" ".ots" "eu.sytue.calcus.documento"' \
  'WriteRegStr HKCU "Software\Sytue\eu.sytue.calcus\Capabilities\FileAssociations" ".xltx" "eu.sytue.calcus.documento"' \
  'WriteRegStr HKCU "Software\RegisteredApplications" "Sytue Calcus" "Software\Sytue\eu.sytue.calcus\Capabilities"' \
  'WriteRegStr HKCU "Software\Sytue\eu.sytue.calcus\Capabilities" "ApplicationName" "Sytue Calcus"' \
  "System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'" \
  'DeleteRegValue HKCU "Software\RegisteredApplications" "Sytue Calcus"' \
  'DeleteRegKey HKCU "Software\Sytue\eu.sytue.calcus"'; do
  grep -qxF "  $l" <<<"$reg" || mal "el instalador de Windows no lleva «$l»"
done

# Windows: el OpenGL de reserva (Mesa) va en `$INSTDIR\mesa` y NUNCA junto a los .exe,
# donde Windows lo cargaría antes que el controlador de la tarjeta en TODOS los equipos.
# Se mira dónde deja cada `File` sus DLL, siguiendo el último SetOutPath; y que al
# desinstalar la carpeta se va. Visto en rojo poniendo `SetOutPath "$INSTDIR"` en nsis_mesa.
mesa="$("$GUION" mesa-windows /m 2>&1)"
fuera="$(awk '/^  SetOutPath / { d = $2 } /^  File .*\.dll"$/ && d != "\"$INSTDIR\\mesa\"" { print }' <<<"$mesa")"
[ -z "$fuera" ] || mal "el instalador de Windows deja una DLL de Mesa fuera de \$INSTDIR\\mesa: $fuera"
for f in opengl32.dll libgallium_wgl.dll LICENCIAS.txt; do
  grep -qxF "  File \"/m/$f\"" <<<"$mesa" || mal "el instalador de Windows no lleva $f del Mesa de reserva"
done
grep -qxF '  RMDir "$INSTDIR\mesa"' <<<"$mesa" || mal "el desinstalador de Windows no quita la carpeta mesa"

# --- 7. Los bundles de escritorio son los que decidió el dueño -------------------
# En escritorio se publica un instalador por bundle y ninguno por app. Qué app va en
# cuál lo dice su FyneApp.toml (`bundle`); una app de la suite que no lo diga saldría
# en un bundle suyo, que es justo lo que se quitó, así que aquí es rojo.
for dir in "$RAIZ"/apps/fyne/cmd/*/; do
  app="$(basename "$dir")"
  [ "$app" = demo ] && continue
  grep -qE '^[[:space:]]*bundle[[:space:]]*=[[:space:]]*"[a-z0-9]+"' "$dir/FyneApp.toml" ||
    mal "apps/fyne/cmd/$app no declara en qué bundle de escritorio va (bundle = \"…\")"
done
quiero="demo demo
drive drive
one bases calcus calendar cast certipdf contacts diapos docus erp hub mail notes photos rooms"
[ "$("$GUION" paquetes 2>&1)" = "$quiero" ] ||
  mal "los bundles no son «Sytue One» (Hub y las de trabajo) y «Sytue Drive»: $("$GUION" paquetes 2>&1 | tr '\n' '|')"
# Lo que se publica: dos instaladores por sistema, con el nombre que leen el Hub, el
# portal y traer-instaladores.sh; la demo no.
for p in linux:linux-amd64.tar.xz windows:windows-amd64-installer.exe macos:macos-universal.dmg; do
  [ "$("$GUION" publicables "${p%%:*}" 2>&1)" = "sytue-drive-${p#*:}
sytue-one-${p#*:}" ] || mal "empaquetar.sh publicables ${p%%:*} no da los dos bundles: $("$GUION" publicables "${p%%:*}" 2>&1 | tr '\n' ' ')"
done
# Y con directorio, falla si falta uno: es lo que impide publicar una release coja.
mkdir -p "$tmp/dist" && : >"$tmp/dist/sytue-one-linux-amd64.tar.xz"
if salida="$("$GUION" publicables linux "$tmp/dist" 2>&1)"; then
  mal "publicables da por completa una salida sin el bundle de Drive"
else
  grep -q 'sytue-drive-linux-amd64.tar.xz' <<<"$salida" || mal "publicables falla sin decir qué falta: $salida"
fi

# --- 8. El instalar.sh del bundle de Linux, ejecutado de verdad -------------------
# Contra un bundle de mentira con la forma que deja empaquetar.sh (los nombres son los
# del .desktop de `fyne package`, el ID de la app): pone el programa, reescribe Exec e
# Icon con rutas completas —`~/.local/bin` no está en el PATH de todos— y --quitar lo
# deja como estaba. Visto en rojo quitando el `sed` de Exec y el `rm` de --quitar.
if [ "$(uname -s)" = Linux ]; then
  b="$tmp/sytue-prueba"
  mkdir -p "$b/bin" "$b/share/applications" "$b/share/pixmaps"
  printf '#!/bin/sh\necho hola\n' >"$b/bin/sytue-uno" && chmod +x "$b/bin/sytue-uno"
  printf '[Desktop Entry]\nType=Application\nName=Uno\nExec=sytue-uno %%U\nIcon=eu.sytue.uno\nMimeType=x-scheme-handler/sytue-uno;application/pdf;application/vnd.oasis.opendocument.text;application/vnd.apple.pages;\n' \
    >"$b/share/applications/eu.sytue.uno.desktop"
  : >"$b/share/pixmaps/eu.sytue.uno.png"
  "$GUION" instalar-sh "Sytue Prueba" >"$b/instalar.sh" && chmod +x "$b/instalar.sh"
  pre="$tmp/prefijo"
  # Un xdg-mime de mentira con el mimeapps.list de verdad: el PDF lo tenía otra app, el
  # ODT nadie. Sin XDG_CONFIG_HOME propio escribiría en el del que corre el test. Como el
  # de verdad (xdg-utils 1.1.3, medido en el frontal), sin nada elegido contesta con
  # cualquier .desktop instalado que declare el tipo: por eso lo de antes se apunta ANTES
  # de instalar los nuestros.
  export XDG_CONFIG_HOME="$tmp/config"
  mkdir -p "$XDG_CONFIG_HOME" "$tmp/xdg"
  printf '[Default Applications]\napplication/pdf=otra.desktop\n' >"$XDG_CONFIG_HOME/mimeapps.list"
  cat >"$tmp/xdg/xdg-mime" <<'SH'
#!/bin/sh
f="$XDG_CONFIG_HOME/mimeapps.list"
case "$1" in
query)
  v=$(sed -n "s|^$3=||p" "$f" | tr -d ';')
  [ -n "$v" ] || v=$(cd "$PREFIJO/share/applications" 2>/dev/null && grep -l "^MimeType=.*$3;" -- *.desktop | head -1)
  printf '%s\n' "$v" ;;
default) grep -v "^$3=" "$f" >"$f.t"; echo "$3=$2" >>"$f.t"; mv "$f.t" "$f" ;;
esac
SH
  chmod +x "$tmp/xdg/xdg-mime"
  PATH="$tmp/xdg:$PATH" PREFIJO="$pre" "$b/instalar.sh" >"$tmp/instalar.log" 2>&1 || mal "instalar.sh falla: $(cat "$tmp/instalar.log")"
  # Visto en rojo quitando cada `xdg-mime default`, el `awk`, la comprobación de que ya
  # se había hecho y apuntando lo de antes después de instalar los .desktop.
  for m in application/pdf application/vnd.oasis.opendocument.text; do
    grep -qx "$m=eu.sytue.uno.desktop" "$XDG_CONFIG_HOME/mimeapps.list" || mal "instalar.sh no deja la app por defecto para $m"
  done
  grep -q '^x-scheme-handler' "$XDG_CONFIG_HOME/mimeapps.list" && mal "instalar.sh pone por defecto el esquema, que no es un documento"
  grep -q '^application/vnd.apple' "$XDG_CONFIG_HOME/mimeapps.list" && mal "instalar.sh pone por defecto un fichero de Apple, que solo va en «Abrir con»"
  # El usuario elige después otra app para el PDF: una puesta al día no se la quita, y al
  # desinstalar tampoco (más abajo).
  PATH="$tmp/xdg:$PATH" xdg-mime default elegida.desktop application/pdf
  PATH="$tmp/xdg:$PATH" PREFIJO="$pre" "$b/instalar.sh" >/dev/null 2>&1 || mal "instalar.sh falla la segunda vez"
  grep -qx 'application/pdf=elegida.desktop' "$XDG_CONFIG_HOME/mimeapps.list" ||
    mal "una puesta al día vuelve a imponer las predeterminadas"
  command -v wl-copy >/dev/null 2>&1 || grep -q "instala wl-clipboard" "$tmp/instalar.log" ||
    mal "instalar.sh no sugiere wl-clipboard, la dependencia opcional de Wayland"
  [ "$("$pre/bin/sytue-uno" 2>&1)" = hola ] || mal "instalar.sh no deja el programa ejecutable en PREFIJO/bin"
  grep -qx "Exec=$pre/bin/sytue-uno %U" "$pre/share/applications/eu.sytue.uno.desktop" 2>/dev/null ||
    mal "el .desktop instalado no lleva la ruta completa del programa en Exec"
  grep -qx "Icon=$pre/share/pixmaps/eu.sytue.uno.png" "$pre/share/applications/eu.sytue.uno.desktop" 2>/dev/null ||
    mal "el .desktop instalado no lleva la ruta completa del icono"
  # Al quitar, la que eligió el usuario se queda y lo que no tenía nadie deja de tenerlo.
  PATH="$tmp/xdg:$PATH" PREFIJO="$pre" "$b/instalar.sh" --quitar >/dev/null 2>&1 || mal "instalar.sh --quitar falla"
  grep -qx 'application/pdf=elegida.desktop' "$XDG_CONFIG_HOME/mimeapps.list" ||
    mal "instalar.sh --quitar pisa la app que el usuario eligió después"
  grep -q 'eu.sytue.uno' "$XDG_CONFIG_HOME/mimeapps.list" && mal "instalar.sh --quitar deja la app por defecto de lo que no tenía nadie"
  [ -z "$(find "$pre" -type f)" ] || mal "instalar.sh --quitar deja ficheros: $(find "$pre" -type f | tr '\n' ' ')"
  # Y si nadie lo tocó, el PDF vuelve a la app que lo tenía.
  printf '[Default Applications]\napplication/pdf=otra.desktop\n' >"$XDG_CONFIG_HOME/mimeapps.list"
  PATH="$tmp/xdg:$PATH" PREFIJO="$pre" "$b/instalar.sh" >/dev/null 2>&1
  PATH="$tmp/xdg:$PATH" PREFIJO="$pre" "$b/instalar.sh" --quitar >/dev/null 2>&1
  grep -qx 'application/pdf=otra.desktop' "$XDG_CONFIG_HOME/mimeapps.list" || mal "instalar.sh --quitar no devuelve el PDF a la app que lo tenía"
  unset XDG_CONFIG_HOME
  PREFIJO="$tmp/con espacio" "$b/instalar.sh" >/dev/null 2>&1 &&
    mal "instalar.sh acepta un PREFIJO con espacios, que el menú no sabría abrir"
fi

# --- 9. Android para Google Play (Play App Signing) ------------------------------
# Un AAB por app, con el versionCode del tag, firmado SOLO con la clave de subida y
# sin que vuelvan los secretos del keystore antiguo, que no existen en git.gay: el job
# los pedía y por eso no firmó nunca. Visto en rojo quitando una app de la carpeta,
# rompiendo la fórmula y devolviendo ANDROID_KEYSTORE_BASE64 a release-mobile.yml.
MOVIL="$RAIZ/.github/workflows/release-mobile.yml"
quiero="$(for a in bases calcus calendar cast certipdf contacts diapos docus drive erp hub mail notes photos rooms; do
  printf 'sytue-%s-android.aab\n' "$a"
done)"
[ "$("$GUION" publicables android 2>&1)" = "$quiero" ] ||
  mal "empaquetar.sh publicables android no da un AAB por cada una de las quince apps: $("$GUION" publicables android 2>&1 | tr '\n' ' ')"
mkdir -p "$tmp/aab"
for f in $quiero; do : >"$tmp/aab/$f"; done
"$GUION" publicables android "$tmp/aab" >/dev/null 2>&1 || mal "publicables android no da por completos los quince AAB"
rm -f "$tmp/aab/sytue-rooms-android.aab"
if salida="$("$GUION" publicables android "$tmp/aab" 2>&1)"; then
  mal "publicables android da por completa una release sin el AAB de Rooms"
else
  grep -q 'sytue-rooms-android.aab' <<<"$salida" || mal "publicables android falla sin decir qué AAB falta: $salida"
fi
# El versionCode: el del tag, creciente y sin colisiones.
[ "$("$GUION" version-code v3.6.23 2>&1)" = 3006023 ] ||
  mal "el versionCode de v3.6.23 no es 3006023: $("$GUION" version-code v3.6.23 2>&1)"
anterior=0
for t in v3.6.23 v3.6.999 v3.7.0 v3.10.1 v4.0.0 v2099.999.999; do
  c="$("$GUION" version-code "$t" 2>/dev/null)" || { mal "sin versionCode para $t"; continue; }
  [ "$c" -gt "$anterior" ] || mal "el versionCode de $t ($c) no crece sobre el anterior ($anterior)"
  [ "$c" -le 2100000000 ] || mal "el versionCode de $t ($c) pasa del tope de Play"
  anterior="$c"
done
for t in v3.1000.0 v3.6.1000 v0.1.0 v2100.0.0 main v3.6; do
  "$GUION" version-code "$t" >/dev/null 2>&1 && mal "version-code acepta «$t», que colisionaría o no es un tag"
done
# Lo que firma: la clave de SUBIDA, con la contraseña fuera de argv, y nada del keystore viejo.
if viejo="$(grep -n 'ANDROID_KEYSTORE_\|ANDROID_KEY_ALIAS' "$RAIZ"/.github/workflows/*.yml)"; then
  mal "se vuelven a pedir los secretos ANDROID_KEYSTORE_* (no existen; la clave es ANDROID_UPLOAD_*): $viejo"
fi
for s in ANDROID_UPLOAD_KEYSTORE_BASE64 ANDROID_UPLOAD_KEYSTORE_PASSWORD ANDROID_UPLOAD_KEY_ALIAS \
  ANDROID_MIN_SDK 'android:minSdkVersion' \
  'publicables android-sin-firmar sin-firmar' 'publicables android firmados' 'storepass:env' 'RUNNER_TEMP' "trap 'rm -f"; do
  grep -qF -- "$s" "$MOVIL" || mal "release-mobile.yml no contiene «$s»"
done
if enclaro="$(grep -nE -- '-storepass +[^:]|-storepass "|--ks-pass|pass:\$' "$MOVIL")"; then
  mal "release-mobile.yml pasa la contraseña del keystore por argv: $enclaro"
fi
if apk="$(grep -n '\.apk' "$MOVIL")"; then
  mal "release-mobile.yml vuelve a publicar APK sueltos (Play App Signing: no se podrían actualizar desde Play): $apk"
fi
grep -q 'empaquetar.sh android$' "$MOVIL" || mal "release-mobile.yml ya no construye los AAB de Android"
# El mínimo de la protección de Play, el target y las páginas de 16 KiB, sobre un
# XML binario y ELF de verdad. No basta cambiar la constante: hay que reescribirlo
# en el manifiesto que se convierte a AAB y verificar el resultado.
grep -qx 'ANDROID_MIN_SDK=24' "$GUION" || mal "el mínimo de Android no es API 24, requerido por la protección automática de Play"
grep -qF 'min-sdk "$t/apk/AndroidManifest.xml" "$ANDROID_MIN_SDK"' "$GUION" ||
  mal "apk_a_aab no aplica ANDROID_MIN_SDK al manifiesto compilado"
grep -qF 'android:minSdkVersion=' "$GUION" || mal "verificar_aab no comprueba el mínimo del manifiesto real"
if command -v python3 >/dev/null 2>&1; then
  python3 - "$tmp/manifiesto.bin" <<'PY'
import struct, sys
cad = ["minSdkVersion", "targetSdkVersion", "uses-sdk", "manifest"]
cuerpo = b"".join(bytes([len(c), len(c)]) + c.encode() + b"\0" for c in cad)
offs, o = [], 0
for c in cad:
    offs.append(o); o += len(c) + 3
cuerpo += b"\0" * (-len(cuerpo) % 4)
inicio = 28 + 4 * len(cad)
pool = struct.pack("<HHIIIIII", 1, 28, inicio + len(cuerpo), len(cad), 0, 0x100, inicio, 0) + struct.pack("<%dI" % len(cad), *offs) + cuerpo
mapa = struct.pack("<HHI", 0x180, 8, 16) + struct.pack("<II", 0x0101020C, 0x01010270)
atr = lambda n, v: struct.pack("<IIIHBBI", 0xFFFFFFFF, n, 0xFFFFFFFF, 8, 0, 0x10, v)
def elem(nombre, atrs):
    ext = struct.pack("<IIHHHHHH", 0xFFFFFFFF, nombre, 20, 20, len(atrs), 0, 0, 0) + b"".join(atrs)
    return struct.pack("<HHIII", 0x102, 16, 16 + len(ext), 1, 0xFFFFFFFF) + ext
cuerpo = pool + mapa + elem(3, []) + elem(2, [atr(0, 21), atr(1, 29)])
open(sys.argv[1], "wb").write(struct.pack("<HHI", 3, 8, 8 + len(cuerpo)) + cuerpo)
PY
  BIN="$AQUI/android-binario.py"
  [ "$(python3 "$BIN" leer-min-sdk "$tmp/manifiesto.bin" 2>&1)" = 21 ] ||
    mal "android-binario.py no lee el mínimo 21 del manifiesto original"
  python3 "$BIN" target-sdk "$tmp/manifiesto.bin" 36 >/dev/null 2>&1 &&
    [ "$(python3 "$BIN" leer-target-sdk "$tmp/manifiesto.bin" 2>&1)" = 36 ] ||
    mal "android-binario.py no deja el targetSdkVersion en 36: $(python3 "$BIN" leer-target-sdk "$tmp/manifiesto.bin" 2>&1)"
  [ "$(python3 "$BIN" leer-min-sdk "$tmp/manifiesto.bin" 2>&1)" = 21 ] ||
    mal "cambiar targetSdkVersion ha cambiado también minSdkVersion"
  tam_sdk="$(wc -c <"$tmp/manifiesto.bin")"
  python3 "$BIN" min-sdk "$tmp/manifiesto.bin" 24 >/dev/null 2>&1 &&
    [ "$(python3 "$BIN" leer-min-sdk "$tmp/manifiesto.bin" 2>&1)" = 24 ] ||
    mal "android-binario.py no deja minSdkVersion en 24 para la protección de Play"
  [ "$(python3 "$BIN" leer-target-sdk "$tmp/manifiesto.bin" 2>&1)" = 36 ] ||
    mal "cambiar minSdkVersion ha perdido targetSdkVersion 36"
  [ "$(wc -c <"$tmp/manifiesto.bin")" = "$tam_sdk" ] || mal "cambiar el mínimo ha movido el tamaño del XML binario"
  cp "$tmp/manifiesto.bin" "$tmp/minimo-24.bin"
  python3 "$BIN" min-sdk "$tmp/manifiesto.bin" 24 >/dev/null 2>&1 &&
    cmp -s "$tmp/manifiesto.bin" "$tmp/minimo-24.bin" || mal "aplicar mínimo 24 por segunda vez cambia el manifiesto"
  # El tipo del servicio en primer plano: un <service> con name y exported, más una
  # cadena de VALOR detrás del mapa (como en los manifiestos de verdad), y el atributo
  # nuevo tiene que quedar leíble, ordenado por identificador y sin tocar lo demás.
  python3 - "$tmp/servicio.bin" <<'PY'
import struct, sys
cad = ["name", "exported", "service", "eu.sytue.nativo.Avisos", "http://schemas.android.com/apk/res/android", "manifest", "eu.sytue.nativo.PantallaServicio"]
cuerpo = b"".join(bytes([len(c), len(c)]) + c.encode() + b"\0" for c in cad)
offs, o = [], 0
for c in cad:
    offs.append(o); o += len(c) + 3
cuerpo += b"\0" * (-len(cuerpo) % 4)
inicio = 28 + 4 * len(cad)
pool = struct.pack("<HHIIIIII", 1, 28, inicio + len(cuerpo), len(cad), 0, 0x100, inicio, 0) + struct.pack("<%dI" % len(cad), *offs) + cuerpo
mapa = struct.pack("<HHI", 0x180, 8, 16) + struct.pack("<II", 0x01010003, 0x01010010)
def elem(nombre, atrs):
    ext = struct.pack("<IIHHHHHH", 0xFFFFFFFF, nombre, 20, 20, len(atrs), 0, 0, 0) + b"".join(atrs)
    return struct.pack("<HHIII", 0x102, 16, 16 + len(ext), 1, 0xFFFFFFFF) + ext
nombre = struct.pack("<IIIHBBI", 4, 0, 3, 8, 0, 0x03, 3)
exportado = struct.pack("<IIIHBBI", 4, 1, 0xFFFFFFFF, 8, 0, 0x12, 0)
fin = struct.pack("<HHIIIII", 0x103, 16, 24, 1, 0xFFFFFFFF, 0xFFFFFFFF, 2)
otro = struct.pack("<IIIHBBI", 4, 0, 6, 8, 0, 0x03, 6)
cuerpo = pool + mapa + elem(5, []) + elem(2, [nombre, exportado]) + fin + elem(2, [otro, exportado]) + fin
open(sys.argv[1], "wb").write(struct.pack("<HHI", 3, 8, 8 + len(cuerpo)) + cuerpo)
PY
  python3 "$BIN" servicio-en-primer-plano "$tmp/servicio.bin" eu.sytue.nativo.Avisos 0x40000000 >/dev/null 2>&1 &&
    [ "$(python3 "$BIN" leer-servicio "$tmp/servicio.bin" eu.sytue.nativo.Avisos 2>&1)" = 0x40000000 ] ||
    mal "android-binario.py no deja el servicio de avisos con foregroundServiceType specialUse: $(python3 "$BIN" leer-servicio "$tmp/servicio.bin" eu.sytue.nativo.Avisos 2>&1)"
  python3 "$BIN" servicio-en-primer-plano "$tmp/servicio.bin" eu.sytue.nativo.Avisos 0x40000000 >/dev/null 2>&1 &&
    mal "android-binario.py mete foregroundServiceType dos veces"
  # Un segundo servicio en el mismo manifiesto (el de compartir pantalla de Rooms) reutiliza
  # la cadena y no pisa al primero.
  python3 "$BIN" servicio-en-primer-plano "$tmp/servicio.bin" eu.sytue.nativo.PantallaServicio 0x20 >/dev/null 2>&1 &&
    [ "$(python3 "$BIN" leer-servicio "$tmp/servicio.bin" eu.sytue.nativo.PantallaServicio 2>&1)" = 0x20 ] &&
    [ "$(python3 "$BIN" leer-servicio "$tmp/servicio.bin" eu.sytue.nativo.Avisos 2>&1)" = 0x40000000 ] ||
    mal "android-binario.py no da su tipo a un segundo servicio sin tocar el primero"
  python3 "$BIN" servicio-en-primer-plano "$tmp/manifiesto.bin" eu.sytue.nativo.Avisos 0x40000000 >/dev/null 2>&1 &&
    mal "android-binario.py da por bueno un manifiesto sin el servicio de avisos"
  # La imagen dentro de imagen: supportsPictureInPicture en la GoNativeActivity, leíble, una
  # sola vez, y sin confundirse con una segunda <activity> (SesionDesbloqueo).
  python3 - "$tmp/actividad.bin" <<'PY'
import struct, sys
cad = ["name", "configChanges", "activity", "org.golang.app.GoNativeActivity", "http://schemas.android.com/apk/res/android", "manifest", "eu.sytue.nativo.SesionDesbloqueo"]
cuerpo = b"".join(bytes([len(c), len(c)]) + c.encode() + b"\0" for c in cad)
offs, o = [], 0
for c in cad:
    offs.append(o); o += len(c) + 3
cuerpo += b"\0" * (-len(cuerpo) % 4)
inicio = 28 + 4 * len(cad)
pool = struct.pack("<HHIIIIII", 1, 28, inicio + len(cuerpo), len(cad), 0, 0x100, inicio, 0) + struct.pack("<%dI" % len(cad), *offs) + cuerpo
mapa = struct.pack("<HHI", 0x180, 8, 16) + struct.pack("<II", 0x01010003, 0x0101001F)
def elem(nombre, atrs):
    ext = struct.pack("<IIHHHHHH", 0xFFFFFFFF, nombre, 20, 20, len(atrs), 0, 0, 0) + b"".join(atrs)
    return struct.pack("<HHIII", 0x102, 16, 16 + len(ext), 1, 0xFFFFFFFF) + ext
nombre = struct.pack("<IIIHBBI", 4, 0, 3, 8, 0, 0x03, 3)
nombre_sesion = struct.pack("<IIIHBBI", 4, 0, 6, 8, 0, 0x03, 6)
cambios = struct.pack("<IIIHBBI", 4, 1, 0xFFFFFFFF, 8, 0, 0x11, 0x4a0)
fin = struct.pack("<HHIIIII", 0x103, 16, 24, 1, 0xFFFFFFFF, 0xFFFFFFFF, 2)
cuerpo = pool + mapa + elem(5, []) + elem(2, [nombre, cambios]) + fin + elem(2, [nombre_sesion]) + fin
open(sys.argv[1], "wb").write(struct.pack("<HHI", 3, 8, 8 + len(cuerpo)) + cuerpo)
PY
  [ -z "$(python3 "$BIN" leer-imagen-en-imagen "$tmp/actividad.bin" 2>&1)" ] ||
    mal "android-binario.py lee supportsPictureInPicture en una actividad que no lo lleva"
  python3 "$BIN" imagen-en-imagen "$tmp/actividad.bin" >/dev/null 2>&1 &&
    [ "$(python3 "$BIN" leer-imagen-en-imagen "$tmp/actividad.bin" 2>&1)" = true ] ||
    mal "android-binario.py no deja la actividad con supportsPictureInPicture: $(python3 "$BIN" leer-imagen-en-imagen "$tmp/actividad.bin" 2>&1)"
  python3 "$BIN" imagen-en-imagen "$tmp/actividad.bin" >/dev/null 2>&1 &&
    mal "android-binario.py mete supportsPictureInPicture dos veces"
  python3 "$BIN" imagen-en-imagen "$tmp/servicio.bin" >/dev/null 2>&1 &&
    mal "android-binario.py da por bueno un manifiesto sin actividad para la imagen dentro de imagen"
  printf 'no es xml' >"$tmp/no-manifiesto.bin"
  python3 "$BIN" target-sdk "$tmp/no-manifiesto.bin" 36 >/dev/null 2>&1 &&
    mal "android-binario.py da por bueno un fichero que no es un manifiesto"
  # Go alinea sus PT_LOAD a 64 KiB en arm64 y a 4 KiB en amd64: uno pasa y otro no.
  python3 "$BIN" alineacion "$arm64" 16384 >/dev/null 2>&1 ||
    mal "android-binario.py rechaza un ELF alineado a 64 KiB"
  python3 "$BIN" alineacion "$amd64" 16384 >/dev/null 2>&1 &&
    mal "android-binario.py da por buena una .so alineada a 4 KiB: Play la rechazaría"
else
  mal "falta python3: no se puede comprobar android-binario.py"
fi

# --- 10. El Simulador de iOS, sin cuenta de Apple -----------------------------------
# Para repetir el barrido de iOS: un .app que el Simulador lanza sin Team ID ni perfil.
# Su Info.plist lo escribe empaquetar.sh y aquí se lee con plistlib: el identificador es
# el de la tienda y el ejecutable el que compila el guion, o el Simulador instala y no
# abre. Visto en rojo cambiando el CFBundleExecutable y mandando iossimulator por la
# rama de `ios`.
if command -v python3 >/dev/null 2>&1; then
  for a in rooms hub; do
    "$GUION" plist-simulador "$a" 2>&1 | python3 -c '
import plistlib, sys
p = plistlib.loads(sys.stdin.buffer.read())
assert p["CFBundleIdentifier"] == sys.argv[1], p["CFBundleIdentifier"]
assert p["CFBundleExecutable"] == "sytue-" + sys.argv[2], p["CFBundleExecutable"]
assert p["LSRequiresIPhoneOS"] is True
' "$(sed -n 's/^[[:space:]]*ID[[:space:]]*=[[:space:]]*"\(.*\)".*/\1/p' "$FYNE/cmd/$a/FyneApp.toml" | head -1)" "$a" 2>"$tmp/plist.err" ||
      mal "el Info.plist del Simulador de $a no vale: $(tail -1 "$tmp/plist.err")"
  done
fi
# Fuera de un Mac tiene que fallar por lo suyo —no hay SDK del Simulador— y no por la
# rama de `ios`, que pide la cuenta. En un Mac compilaría de verdad, y eso no va aquí.
if [ "$(uname -s)" != Darwin ]; then
  # Con una CLI de mentira: la de verdad no hace falta para llegar a esa rama.
  mkdir -p "$tmp/cli" && printf '#!/bin/sh\nexit 0\n' >"$tmp/cli/fyne" && chmod +x "$tmp/cli/fyne"
  salida="$(PATH="$tmp/cli:$PATH" SYTUE_IOS_PROFILE='' "$GUION" iossimulator demo 2>&1)"
  grep -q 'el Simulador de iOS solo existe en un Mac' <<<"$salida" ||
    mal "empaquetar.sh iossimulator no va por su rama (¿pide la cuenta de Apple?): $(head -1 <<<"$salida")"
fi

# El proyecto temporal que genera la CLI de Fyne también recibe el mínimo de iOS.
# Una CLI de prueba observa el entorno justo antes de compilar y termina ahí: no
# necesita un Mac, certificados ni SDK. Sin el xcconfig, Xcode 27 rechaza iOS 9.
mkdir -p "$tmp/cli-ios"
cat >"$tmp/cli-ios/uname" <<'SH'
#!/bin/sh
case "$1" in -s) echo Darwin ;; -m) echo arm64 ;; esac
SH
cat >"$tmp/cli-ios/xcodebuild" <<'SH'
#!/bin/sh
exit 99
SH
cat >"$tmp/cli-ios/fyne" <<'SH'
#!/bin/sh
[ "$1" = package ] || exit 99
[ -f "${XCODE_XCCONFIG_FILE:-}" ] || exit 98
[ "$(cat "$XCODE_XCCONFIG_FILE")" = 'IPHONEOS_DEPLOYMENT_TARGET = 15.0' ] || exit 97
echo minimo-ios-15-recibido
exit 71
SH
chmod +x "$tmp/cli-ios"/*
salida="$(PATH="$tmp/cli-ios:$PATH" GOPRIVATE='github.com/thinkddr/*' SYTUE_IOS_PROFILE=perfil-de-prueba \
  SYTUE_IOS_IDENTIDAD=identidad-de-prueba "$GUION" ios demo 2>&1)"
grep -qx minimo-ios-15-recibido <<<"$salida" ||
  mal "el empaquetado iOS no pasa su mínimo a Xcode: $(tail -1 <<<"$salida")"

# En el Mac propio el llavero efímero NO entra en `security list-keychains`, así que
# `xcodebuild` de la CLI de Fyne debe construir el envoltorio `.app` SIN firmar
# (`CODE_SIGNING_ALLOWED=NO`, sin `DEVELOPMENT_TEAM=VHC5QW7BZ3`) y `firmar_ipa` embebe
# el perfil `0644`, deriva los derechos base (`application-identifier`, `team-identifier`,
# `get-task-allow=false`, `beta-reports-active=true`) y firma con `codesign`.
cat >"$tmp/cli-ios/xcrun" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$XCRUN_ARGV"
exit 72
SH
cat >"$tmp/cli-ios/fyne" <<'SH'
#!/usr/bin/env bash
[ "$1" = package ] || exit 99
exec xcrun xcodebuild -configuration Release -project /tmp/main.xcodeproj -allowProvisioningUpdates DEVELOPMENT_TEAM=VHC5QW7BZ3
SH
chmod +x "$tmp/cli-ios/xcrun" "$tmp/cli-ios/fyne"
XCRUN_ARGV="$tmp/cli-ios-xcrun.argv" PATH="$tmp/cli-ios:$PATH" GOPRIVATE='github.com/thinkddr/*' \
  SYTUE_IOS_PROFILE=perfil-de-prueba SYTUE_IOS_IDENTIDAD=identidad-de-prueba "$GUION" ios demo >/dev/null 2>&1 || true
if [ ! -s "$tmp/cli-ios-xcrun.argv" ] ||
  grep -qx 'DEVELOPMENT_TEAM=VHC5QW7BZ3' "$tmp/cli-ios-xcrun.argv" ||
  ! grep -qx 'CODE_SIGNING_ALLOWED=NO' "$tmp/cli-ios-xcrun.argv" ||
  ! grep -qx 'CODE_SIGNING_REQUIRED=NO' "$tmp/cli-ios-xcrun.argv" ||
  ! grep -qx 'DEVELOPMENT_TEAM=' "$tmp/cli-ios-xcrun.argv"; then
  mal "empaquetar.sh ios deja que xcodebuild de la CLI pida el certificado en list-keychains"
fi

# `firmar_ipa` sobre un `.app` sin firmar salido de `xcodebuild` (`CODE_SIGNING_ALLOWED=NO`)
# y con `Info.plist` en `0600` como escribe `fyne`: copia el perfil en `0644`, extrae los
# derechos base + `beta-reports-active` + sesión + `associated-domains`, normaliza permisos
# y produce un `.ipa` verificable.
ipa_sin_firma="$tmp/ipa-sin-firma"
mkdir -p "$ipa_sin_firma/bin" "$ipa_sin_firma/aqui" "$ipa_sin_firma/raiz/apps/nucleo/ajustes" \
  "$ipa_sin_firma/abierto/Payload/Sytue Rooms.app" "$ipa_sin_firma/fuentes/rooms" "$ipa_sin_firma/dist"
cat >"$ipa_sin_firma/raiz/apps/nucleo/ajustes/grupo_apple.go" <<'GO'
package ajustes
const GrupoSuite = "VHC5QW7BZ3.eu.sytue.suite"
const GrupoContenedorSuite = "group.eu.sytue.suite"
GO
cat >"$ipa_sin_firma/fuentes/rooms/FyneApp.toml" <<'TOML'
[Details]
Name = "Sytue Rooms"
ID = "eu.sytue.rooms"
Version = "3.15.3"
Build = 56
[Development]
esquema = "sytue-rooms"
enlaces = "rooms"
TOML
python3 - "$ipa_sin_firma/abierto/Payload/Sytue Rooms.app/Info.plist" "$ipa_sin_firma/perfil.mobileprovision" <<'PY'
import os, plistlib, sys
with open(sys.argv[1], 'wb') as f:
    plistlib.dump({'CFBundleExecutable': 'main'}, f)
os.chmod(sys.argv[1], 0o600)
with open(sys.argv[2], 'wb') as f:
    plistlib.dump({
        'TeamIdentifier': ['VHC5QW7BZ3'],
        'Platform': ['iOS'],
        'Entitlements': {
            'application-identifier': 'VHC5QW7BZ3.eu.sytue.rooms',
            'com.apple.developer.team-identifier': 'VHC5QW7BZ3',
            'get-task-allow': False,
            'beta-reports-active': True,
            'com.apple.security.application-groups': ['group.eu.sytue.suite'],
            'keychain-access-groups': ['VHC5QW7BZ3.*'],
            'com.apple.developer.associated-domains': '*',
        },
    }, f)
os.chmod(sys.argv[2], 0o600)
PY
printf 'macho-inicial' >"$ipa_sin_firma/abierto/Payload/Sytue Rooms.app/main"
cat >"$ipa_sin_firma/aqui/apple-firma.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
perfil-ruta)
  [ "$2 $3" = "eu.sytue.rooms ios" ] || exit 1
  printf '%s\n' "$IPA_SIN_FIRMA/perfil.mobileprovision" ;;
verificar-perfil-grupo)
  [ -f "$2" ] && [ "$3 $4" = "eu.sytue.rooms ios" ] ;;
*) exit 1 ;;
esac
SH
cat >"$ipa_sin_firma/aqui/apple_perfil.py" <<'PY'
#!/usr/bin/env python3
import pathlib, sys
assert sys.argv[1] == 'decodificar' and sys.argv[3] == 'ios'
sys.stdout.buffer.write(pathlib.Path(sys.argv[2]).read_bytes())
PY
chmod +x "$ipa_sin_firma/aqui/apple-firma.sh" "$ipa_sin_firma/aqui/apple_perfil.py"
cat >"$ipa_sin_firma/bin/codesign" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = -d ]; then exit 1; fi
if [ "$1" = --verify ]; then exit 0; fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = --entitlements ]; then cp "$2" "$IPA_SIN_FIRMA/firmados.plist"; break; fi
  shift
done
SH
chmod +x "$ipa_sin_firma/bin/codesign"
if ! command -v zip >/dev/null 2>&1; then
  cat >"$ipa_sin_firma/bin/zip" <<'PY'
#!/usr/bin/env python3
import os, sys, zipfile
args = [a for a in sys.argv[1:] if not a.startswith('-')]
destino, raices = args[0], args[1:]
with zipfile.ZipFile(destino, 'w', zipfile.ZIP_DEFLATED) as zf:
    for raiz in raices:
        for base, dirs, ficheros in os.walk(raiz):
            dirs.sort()
            ficheros.sort()
            st = os.lstat(base)
            info = zipfile.ZipInfo(base.rstrip('/') + '/')
            info.external_attr = ((st.st_mode & 0xFFFF) << 16) | 0x10
            zf.writestr(info, b'')
            for nombre in ficheros:
                ruta = os.path.join(base, nombre)
                st = os.lstat(ruta)
                info = zipfile.ZipInfo(ruta)
                info.external_attr = (st.st_mode & 0xFFFF) << 16
                with open(ruta, 'rb') as f:
                    zf.writestr(info, f.read())
PY
  chmod +x "$ipa_sin_firma/bin/zip"
fi
{
  for funcion in metadato grupos_sesion_apple derechos_sesion verificar_permisos_apple firmar_ipa; do
    awk -v fn="$funcion" '
      $0 ~ "^" fn "\\(\\) \\{" { dentro = 1 }
      dentro {
        print
        if (!py && $0 ~ /<<\x27PY\x27/) py = 1
        else if (py && $0 == "PY") py = 0
        else if (!py && $0 == "}") dentro = 0
      }
    ' "$GUION"
  done
  cat <<'SH'
compilar_ios_paquete() { printf 'macho-go' >"$2/main"; chmod 0755 "$2/main"; }
incorporar_widgets_apple() { mkdir -p "$1/../../SwiftSupport/iphoneos"; }
SH
} >"$ipa_sin_firma/funciones.sh"
chmod 0700 "$ipa_sin_firma/abierto" "$ipa_sin_firma/abierto/Payload"
if PATH="$ipa_sin_firma/bin:$PATH" IPA_SIN_FIRMA="$ipa_sin_firma" \
  RAIZ="$ipa_sin_firma/raiz" AQUI="$ipa_sin_firma/aqui" \
  SYTUE_IOS_IDENTIDAD="Apple Distribution: Prueba (VHC5QW7BZ3)" version=3.15.3 build=56 \
  bash -c 'set -euo pipefail; source "$1/funciones.sh"; firmar_ipa "$1/abierto" "$1/abierto/Payload/Sytue Rooms.app" "$1/dist/sytue-rooms-ios.ipa" eu.sytue.rooms rooms.sytue.eu "$1/fuentes/rooms"' _ "$ipa_sin_firma" >"$ipa_sin_firma/salida" 2>&1; then
  python3 - "$ipa_sin_firma/firmados.plist" "$ipa_sin_firma/dist/sytue-rooms-ios.ipa" <<'PY' || mal "firmar_ipa no graba los derechos o permisos esperados en el IPA"
import plistlib, sys, zipfile
derechos = plistlib.loads(open(sys.argv[1], 'rb').read())
assert derechos == {
    'application-identifier': 'VHC5QW7BZ3.eu.sytue.rooms',
    'com.apple.developer.team-identifier': 'VHC5QW7BZ3',
    'get-task-allow': False,
    'beta-reports-active': True,
    'com.apple.security.application-groups': ['group.eu.sytue.suite'],
    'keychain-access-groups': ['VHC5QW7BZ3.eu.sytue.rooms', 'VHC5QW7BZ3.eu.sytue.suite'],
    'com.apple.developer.associated-domains': ['applinks:rooms.sytue.eu'],
}, derechos
with zipfile.ZipFile(sys.argv[2]) as zf:
    nombres = set(zf.namelist())
    assert 'Payload/Sytue Rooms.app/embedded.mobileprovision' in nombres, nombres
    assert not any(n.startswith('SwiftSupport') for n in nombres), nombres
    for info in zf.infolist():
        modo = (info.external_attr >> 16) & 0o777
        minimo = 0o555 if info.is_dir() else 0o444
        assert modo & minimo == minimo, (info.filename, oct(modo))
PY
else
  mal "firmar_ipa rechaza un .app sin firma previa de xcodebuild: $(tail -2 "$ipa_sin_firma/salida")"
fi

# El Mach-O lo compila Go ANTES de Xcode: la xcconfig no cambia sus flags. Se
# ejecutan los helpers reales con una CLI y un SDK de prueba; el mínimo del fixture
# es 16 para detectar un 15 copiado, y el TOML simula el Build incrementado por CLI.
ios_fixture="$tmp/ios-enlace"
mkdir -p "$ios_fixture/bin" "$ios_fixture/fyne/xcode" "$ios_fixture/fuentes" "$ios_fixture/Sytue Rooms.app"
printf 'IPHONEOS_DEPLOYMENT_TARGET = 16.0\n' >"$ios_fixture/fyne/xcode/Sytue.xcconfig"
cat >"$ios_fixture/fuentes/FyneApp.toml" <<'TOML'
# comentario que también debe conservarse
[Details]
Name = "Sytue Rooms"
ID = "eu.sytue.rooms"
Version = "0.1.0"
Build = 99
[Development]
esquema = "sytue-rooms"
client_id = "sytue-chat"
extensiones = "txt"
[Release]
client_id = "cliente-release"
extensiones = "json"
propia = "se conserva"
TOML
python3 - "$ios_fixture/Sytue Rooms.app/Info.plist" <<'PY'
import plistlib, sys
with open(sys.argv[1], 'wb') as f:
    plistlib.dump({'CFBundleExecutable': 'main'}, f)
PY
# El wrapper firmado real es FAT. Go1.27 no permite sobrescribir ese magic;
# compilar a un destino nuevo debe mantenerlo intacto hasta tener un Mach-O válido.
printf '\312\376\272\276ejecutable-firmado-anterior' >"$ios_fixture/Sytue Rooms.app/main"
cat >"$ios_fixture/bin/fyne" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
[ "$1" = build ] || exit 91
printf '%s\n' "$@" >"$IOS_FIXTURE/argv"
printf '%s\n' "$GOOS" "$GOARCH" "$CGO_ENABLED" "$CGO_CFLAGS" "$CGO_CXXFLAGS" "$CGO_LDFLAGS" "$GOFLAGS" >"$IOS_FIXTURE/entorno"
cp FyneApp.toml "$IOS_FIXTURE/metadata.toml"
salida=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then salida="$2"; break; fi
  shift
done
[ -n "$salida" ] || exit 96
python3 - "$salida" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
if p.exists() and p.read_bytes()[:4] == bytes.fromhex('cafebabe'):
    sys.exit('build output already exists and is not an object file')
PY
if [ "${IOS_FALLA_BUILD:-no}" = si ]; then
  echo salida-parcial >"$salida"
  exit 92
fi
cp "$IOS_BINARIO" "$salida"
SH
cat >"$ios_fixture/bin/xcrun" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
--sdk)
  [ "$2" = iphoneos ] || exit 93
  case "$3" in
  --show-sdk-path) echo /sdk/iphoneos ;;
  -f) echo "/sdk/$4" ;;
  *) exit 94 ;;
  esac ;;
vtool)
  : >"$IOS_FIXTURE/verificado"
  if [ "${IOS_MACHO_LEGACY:-no}" = si ]; then
    printf 'Load command 1\n      cmd LC_VERSION_MIN_IPHONEOS\n  version %s\n      sdk 27.1\n' "${IOS_MINIMO_REAL:-16.0}"
  else
    printf 'Load command 1\n      cmd LC_BUILD_VERSION\n  cmdsize 32\n platform IOS\n    minos %s\n      sdk 27.1\n   ntools 1\n     tool LD\n  version %s\n' "${IOS_MINIMO_REAL:-16.0}" "${IOS_VERSION_TOOL:-1222.1}"
  fi ;;
otool)
  if [ "$2" = -hv ]; then
    printf 'Mach header\n      magic cputype cpusubtype caps filetype ncmds sizeofcmds flags\nMH_MAGIC_64 ARM64 ALL 0x00 %s 37 4000 NOUNDEFS\n' "${IOS_TIPO_MACHO:-EXECUTE}"
  elif [ "${IOS_ALIAS_SWIFT:-no}" = si ]; then
    echo '@rpath/libswiftCoreMedia.dylib (compatibility version 1.0.0)'
  else
    echo '/System/Library/Frameworks/CoreMedia.framework/CoreMedia (compatibility version 1.0.0)'
  fi ;;
*) exit 95 ;;
esac
SH
chmod +x "$ios_fixture/bin"/*
# Sin copia de implementación: se cargan las funciones del empaquetador de verdad.
for funcion in metadato binario_de arquitectura_de verificar_arquitectura compilar_fyne minimo_ios enlace_widgets_apple permiso_enlace_widgets_apple compilar_ios verificar_minimo_ios compilar_ios_paquete; do
  sed -n "/^$funcion() {/,/^}/p" "$GUION"
done >"$ios_fixture/funciones.sh"
ejecutar_ios_fixture() {
  PATH="$ios_fixture/bin:$PATH" CLI="$ios_fixture/bin/fyne" FYNE="$ios_fixture/fyne" \
    IOS_FIXTURE="$ios_fixture" IOS_BINARIO="$arm64" \
    bash -c 'set -euo pipefail; source "$1/funciones.sh"; compilar_ios_paquete "$1/fuentes" "$1/Sytue Rooms.app" 3.13.4 31' _ "$ios_fixture"
}
ejecutar_ios_fixture >"$ios_fixture/salida" 2>&1 ||
  mal "la recompilación iOS no conserva su contrato: $(tail -2 "$ios_fixture/salida")"
python3 - "$ios_fixture" <<'PY' || mal "el binario iOS perdería mínimo, versión o callback OAuth"
import pathlib, re, sys
p = pathlib.Path(sys.argv[1])
args = (p / 'argv').read_text().splitlines()
env = (p / 'entorno').read_text().splitlines()
assert env[:3] == ['ios', 'arm64', '1'], env[:3]
assert all('-miphoneos-version-min=16.0' in f and '-arch arm64' in f for f in env[3:6]), env[3:6]
assert '--release' in args and args[args.index('--tags') + 1] == 'goolm', args
assert 'esquema=sytue-rooms' in args and 'client_id=cliente-release' in args and 'extensiones=json' in args, args
toml = (p / 'metadata.toml').read_text()
assert re.search(r'^Version = "3.13.4"$', toml, re.M), toml
assert re.search(r'^Build = 31$', toml, re.M), toml
assert 'propia = "se conserva"' in toml and '# comentario que también debe conservarse' in toml, toml
PY
mkdir -p "$ios_fixture/productos con espacios/SytueWidgetBridge.framework"
SYTUE_WIDGET_PRODUCTOS="$ios_fixture/productos con espacios" ejecutar_ios_fixture >"$ios_fixture/salida" 2>&1 ||
  mal "iOS no enlaza el puente WidgetKit: $(tail -2 "$ios_fixture/salida")"
python3 - "$ios_fixture/entorno" <<'PY' || mal "el enlace Go descarta el puente público o no puede encontrarlo"
import pathlib, sys
flags = pathlib.Path(sys.argv[1]).read_text().splitlines()[5]
assert '-F "' in flags and 'productos con espacios"' in flags, flags
assert '-Wl,-needed_framework,SytueWidgetBridge' in flags, flags
assert '-Wl,-rpath,@executable_path/Frameworks' in flags, flags
PY
rm -rf "$ios_fixture/productos con espacios/SytueWidgetBridge.framework"
if SYTUE_WIDGET_PRODUCTOS="$ios_fixture/productos con espacios" ejecutar_ios_fixture >"$ios_fixture/salida" 2>&1; then
  mal "iOS compila un host con WidgetKit sin haber construido su framework"
fi
IOS_MACHO_LEGACY=si ejecutar_ios_fixture >"$ios_fixture/salida" 2>&1 ||
  mal "iOS rechaza el mínimo válido de LC_VERSION_MIN_IPHONEOS"
for tipo in OBJECT DYLIB; do
  if IOS_TIPO_MACHO="$tipo" ejecutar_ios_fixture >"$ios_fixture/salida" 2>&1; then
    mal "iOS acepta un Mach-O $tipo como ejecutable de la app"
  else
    grep -q 'no es un ejecutable MH_EXECUTE' "$ios_fixture/salida" ||
      mal "iOS no comprueba el tipo Mach-O $tipo"
  fi
done
# Una clave Release vacía es explícita: no se sustituye por Development.
python3 - "$ios_fixture/fuentes/FyneApp.toml" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
p.write_text(p.read_text().replace('extensiones = "json"', 'extensiones = ""'))
PY
ejecutar_ios_fixture >"$ios_fixture/salida" 2>&1 || mal "iOS no acepta un metadato Release vacío"
grep -qx 'extensiones=' "$ios_fixture/argv" || mal "iOS sustituye un metadato Release vacío por Development"
if IOS_MINIMO_REAL=7.0 IOS_VERSION_TOOL=16.0 ejecutar_ios_fixture >"$ios_fixture/salida" 2>&1; then
  mal "iOS permite publicar un Mach-O con mínimo 7 aunque su Info.plist diga 16"
else
  grep -q 'Mach-O de iOS no declara el mínimo canónico' "$ios_fixture/salida" ||
    mal "el rechazo del mínimo iOS no explica qué se ha medido"
fi
if IOS_ALIAS_SWIFT=si ejecutar_ios_fixture >"$ios_fixture/salida" 2>&1; then
  mal "iOS permite publicar el alias Swift CoreMedia sin soporte Swift"
else
  grep -q 'enlaza Swift por @rpath' "$ios_fixture/salida" || mal "el rechazo del alias Swift no se ha ejecutado"
fi
rm -f "$ios_fixture/verificado"
cp "$ios_fixture/Sytue Rooms.app/main" "$ios_fixture/main.antes"
IOS_FALLA_BUILD=si ejecutar_ios_fixture >"$ios_fixture/salida" 2>&1 &&
  mal "iOS continúa aunque falle la recompilación del binario"
[ ! -e "$ios_fixture/verificado" ] || mal "iOS comprueba un ejecutable viejo después de fallar su recompilación"
cmp -s "$ios_fixture/Sytue Rooms.app/main" "$ios_fixture/main.antes" ||
  mal "un fallo al recompilar iOS destruye el ejecutable anterior"
[ -z "$(find "$ios_fixture/Sytue Rooms.app" -maxdepth 1 -name '.sytue-ios.*' -print)" ] ||
  mal "la recompilación iOS deja su temporal dentro de la app firmada"

# --- 11. Empaquetar no reescribe el FyneApp.toml --------------------------------------
# `fyne package` sube el Build y guarda el fichero sin comentarios. Con una CLI de
# mentira que hace lo mismo (y falla, para no seguir), el FyneApp.toml tiene que quedar
# como estaba. Visto en rojo quitando `restaurar_toml` del trap.
if [ "$(uname -s)" = Linux ]; then
  mkdir -p "$tmp/cli-toml"
  cat >"$tmp/cli-toml/fyne" <<'SH'
#!/bin/sh
case "$1" in
build) while [ $# -gt 0 ]; do [ "$1" = -o ] && : >"$2"; shift; done ;;
package) echo 'Build = 99' >>FyneApp.toml; exit 1 ;;
esac
SH
  printf '#!/bin/sh
exit 0
' >"$tmp/cli-toml/pkg-config"
  cp "$tmp/cli-toml/pkg-config" "$tmp/cli-toml/x86_64-linux-gnu-gcc"
  chmod +x "$tmp/cli-toml"/*
  toml="$FYNE/cmd/hub/FyneApp.toml"
  cp "$toml" "$tmp/toml.antes"
  PATH="$tmp/cli-toml:$PATH" "$GUION" linux hub >/dev/null 2>&1
  rm -f "$FYNE/cmd/hub/sytue-hub" # lo que deja el `build` de mentira
  cmp -s "$toml" "$tmp/toml.antes" || {
    mal "empaquetar.sh deja el FyneApp.toml reescrito por fyne package"
    cp "$tmp/toml.antes" "$toml"
  }
fi

# --- 12. Google Play: la ficha y el secreto ------------------------------------------
# `play-publicar.sh --seco` sobre el árbol real, y en rojo sobre una copia estropeada
# (un título de 31, una sola captura, un CSV sin cabecera): si la validación dejara de
# mirar algo, el caso malo saldría verde. Y el secreto: se sondea contra un curl y un
# openssl de mentira que apuntan su argv, y ni la clave privada ni el token pueden
# aparecer ahí. Visto en rojo quitando el `-H @cabecera` y el `--data @-`.
PLAY_GUION="$AQUI/play-publicar.sh"
[ -x "$PLAY_GUION" ] || mal "apps/fyne/scripts/play-publicar.sh no es ejecutable"
salida="$("$PLAY_GUION" --seco 2>&1)" || mal "play-publicar.sh --seco no pasa en el árbol: $(grep -m3 error <<<"$salida")"
grep -q '15 apps válidas' <<<"$salida" || mal "play-publicar.sh --seco no valida las quince apps: $(tail -1 <<<"$salida")"
PLAY_CONFIRMAR_PRODUCCION=NO "$PLAY_GUION" --produccion >/dev/null 2>&1 &&
  mal "play-publicar.sh deja promover producción sin la confirmación exacta"
{ grep -q 'tracks/production' "$PLAY_GUION" && grep -q 'ERROR_IF_IN_REVIEW' "$PLAY_GUION"; } ||
  mal "play-publicar.sh no protege la promoción a producción frente a una revisión existente"
for app in "$FYNE"/cmd/*/; do
  app="$(basename "$app")"
  [ "$app" = demo ] && continue
  [ -s "$FYNE/play/$app/seguridad-datos.csv" ] || mal "$app no tiene play/$app/seguridad-datos.csv"
  ls "$FYNE/play/$app/graficos/telefono-"*.png >/dev/null 2>&1 || mal "$app no tiene capturas de teléfono"
done
roto="$tmp/play-roto"
mkdir -p "$roto" && cp -R "$FYNE/play/mail" "$roto/"
printf 'Sytue Mail con un nombre largo.\n' >"$roto/mail/es-ES/title.txt"
rm -f "$roto"/mail/graficos/telefono-[2-9].png
echo 'a,b' >"$roto/mail/seguridad-datos.csv"
salida="$(PLAY_DIR="$roto" "$PLAY_GUION" --seco mail 2>&1)" && mal "play-publicar.sh --seco da por buena una ficha rota"
for que in 'title.txt: 31 caracteres' '1 capturas de teléfono' 'cabecera de la exportación'; do
  grep -q "$que" <<<"$salida" || mal "play-publicar.sh --seco no ve «$que»"
done
if command -v openssl >/dev/null 2>&1 && command -v jq >/dev/null 2>&1; then
  falso="$tmp/play-falso" && mkdir -p "$falso/bin"
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$falso/clave.pem" 2>/dev/null
  jq -n --rawfile k "$falso/clave.pem" '{client_email:"ci@ejemplo.iam.gserviceaccount.com", private_key:$k}' >"$falso/cuenta.json"
  real_openssl="$(command -v openssl)"
  cat >"$falso/bin/openssl" <<SH
#!/bin/sh
printf '%s\n' "\$*" >>"$falso/argv"
exec "$real_openssl" "\$@"
SH
  export PLAY_FALSO_ROOT="$falso"
  # La misma API falsa para sondear y producción. Solo persiste un track al hacer commit.
  cat >"$falso/bin/curl" <<'SH'
#!/bin/bash
raiz="$PLAY_FALSO_ROOT"
printf '%s\n' "$*" >>"$raiz/argv"
salida=/dev/stdout w="" metodo=GET cuerpo="" codigo=200
while [ $# -gt 0 ]; do
  case "$1" in
  -o) salida="$2"; shift ;;
  -w) w="$2"; shift ;;
  -X) metodo="$2"; shift ;;
  --data | --data-binary)
    cuerpo="$2"; shift
    [ "$cuerpo" != @- ] || cuerpo="$(cat)"
    ;;
  esac
  url="$1"; shift
done
mkdir -p "$raiz/tracks" "$raiz/pendientes" "$raiz/internos"
paquete="${url#*/applications/}"; paquete="${paquete%%/*}"
if [ "${url#*oauth2}" != "$url" ]; then
  echo OAuth >>"$raiz/autenticaciones"
else
  printf '%s %s\n' "$metodo" "$url" >>"$raiz/peticiones"
fi
# Una sola fuente para el track editable y el resumen fuera de la edición. Los
# escenarios pin-* reproducen el borrador real anterior, no el tag que se sube.
produccion_actual() {
  if [ -f "$raiz/tracks/$paquete.json" ]; then
    cat "$raiz/tracks/$paquete.json"
    return
  fi
  case "${PLAY_ESCENARIO:-}:$paquete" in
  estable:eu.sytue.rooms) echo '{"track":"production","releases":[{"versionCodes":["3012004"],"status":"completed"}]}' ;;
  otro-borrador:eu.sytue.rooms) echo '{"track":"production","releases":[{"versionCodes":["3012005"],"status":"draft"}]}' ;;
  mismo:*) echo '{"track":"production","releases":[{"name":"Conservar","versionCodes":["3012004"],"status":"draft","releaseNotes":[{"language":"es-ES","text":"Notas revisadas"}]}]}' ;;
  pin-*:*)
    base='{"track":"production","releases":[{"name":"v3.12.24","versionCodes":["3012024"],"status":"draft","releaseNotes":[{"language":"es-ES","text":"Notas revisadas"},{"language":"en-US","text":"Reviewed notes"}],"inAppUpdatePriority":3}]}'
    case "${PLAY_ESCENARIO:-}:$paquete" in
    pin-parcial:eu.sytue.bases|pin-parcial:eu.sytue.calcus|pin-parcial:eu.sytue.calendar|pin-parcial:eu.sytue.cast|pin-parcial:eu.sytue.certipdf|pin-parcial:eu.sytue.contacts|pin-ya-completed-*:*)
      jq '.releases[0] |= (.name="v3.14.0" | .versionCodes=["3014000"] | .status="completed")' <<<"$base"
      return
      ;;
    esac
    if [ "$paquete" != eu.sytue.rooms ]; then echo "$base"; return; fi
    case "$PLAY_ESCENARIO" in
    pin-otro) jq '.releases[0].versionCodes=["3012023"]' <<<"$base" ;;
    pin-mezclado) jq '.releases[0].versionCodes=["3012024","3014000"]' <<<"$base" ;;
    pin-futuro) jq '.releases[0].versionCodes=["3014001"]' <<<"$base" ;;
    pin-completed) jq '.releases[0].status="completed"' <<<"$base" ;;
    pin-inProgress) jq '.releases[0].status="inProgress"' <<<"$base" ;;
    pin-halted) jq '.releases[0].status="halted"' <<<"$base" ;;
    pin-dos) jq '.releases += [.releases[0]]' <<<"$base" ;;
    pin-campo-desconocido) jq '.releases[0].futuro="no adivinar"' <<<"$base" ;;
    pin-countryTargeting) jq '.releases[0].countryTargeting={countries:["ES"]}' <<<"$base" ;;
    pin-userFraction) jq '.releases[0].userFraction=0.1' <<<"$base" ;;
    pin-notas-invalidas) jq '.releases[0].releaseNotes=[{language:"es-ES",text:42}]' <<<"$base" ;;
    pin-prioridad-invalida) jq '.releases[0].inAppUpdatePriority=6' <<<"$base" ;;
    *) echo "$base" ;;
    esac
    ;;
  *) echo '{"track":"production","releases":[]}' ;;
  esac
}
respuesta='{}'
case "$url" in
*oauth2*) respuesta='{"access_token":"TOKEN-QUE-NO-SE-VE"}' ;;
*/generatedApks/*)
  case "${PLAY_ESCENARIO:-}" in
  sin-huellas) respuesta='{"generatedApks":[]}' ;;
  huella-rota) respuesta='{"generatedApks":[{"certificateSha256Hash":"no-es-un-certificado"}]}' ;;
  falta-docus)
    if [ "$paquete" = eu.sytue.docus ]; then
      codigo=404; respuesta='{"error":{"code":404,"status":"NOT_FOUND","message":"App Bundle version not found"}}'
    else
      respuesta='{"generatedApks":[{"certificateSha256Hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}'
    fi
    ;;
  huellas-base64) respuesta='{"generatedApks":[{"certificateSha256Hash":"//////////////////////////////////////////8="},{"certificateSha256Hash":"__________________________________________8"},{"certificateSha256Hash":"FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF:FF"}]}' ;;
  *) respuesta='{"generatedApks":[{"certificateSha256Hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},{"certificateSha256Hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},{"certificateSha256Hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}]}' ;;
  esac
  ;;
*/edits) respuesta='{"id":"e1"}' ;;
*/bundles)
  if [ "${PLAY_ESCENARIO:-}:$paquete" = interno-falta:eu.sytue.rooms ]; then
    respuesta='{"bundles":[]}'
  elif [[ "${PLAY_ESCENARIO:-}" == interno-coexistente* ]]; then
    respuesta='{"bundles":[{"versionCode":3014001}]}'
  else
    respuesta='{"bundles":[{"versionCode":3013004}]}'
  fi
  ;;
*/testers/internal) respuesta='{"googleGroups":["testers-privados@example.com"]}' ;;
*/tracks/internal)
  if [ "$metodo" = PUT ]; then
    if [ "${PLAY_ESCENARIO:-}:$paquete" = interno-rechazado:eu.sytue.rooms ]; then
      codigo=400; respuesta='{"error":{"code":400,"message":"Only releases with status draft may be created on draft app"}}'
    else
      printf '%s' "$cuerpo" >"$raiz/pendientes/$paquete-interno.json"
    fi
  else
  case "${PLAY_ESCENARIO:-}:$paquete" in
  interno-par-*:*)
    respuesta='{"track":"internal","releases":[{"name":"Borrador","versionCodes":["3013004"],"status":"draft","releaseNotes":[{"language":"es-ES","text":"Notas del borrador"}],"countryTargeting":{"countries":["ES"]},"inAppUpdatePriority":3},{"name":"Activo","versionCodes":["3013003"],"status":"completed","releaseNotes":[{"language":"es-ES","text":"Notas del activo"}],"countryTargeting":{"countries":["DE"]},"inAppUpdatePriority":1}]}'
    case "${PLAY_ESCENARIO:-}:$paquete" in
    interno-par-activo-primero:*) respuesta="$(jq '.releases |= reverse' <<<"$respuesta")" ;;
    interno-par-dos-drafts:eu.sytue.rooms) respuesta="$(jq '.releases[1].status="draft"' <<<"$respuesta")" ;;
    interno-par-dos-completed:eu.sytue.rooms) respuesta="$(jq '.releases[0].status="completed"' <<<"$respuesta")" ;;
    interno-par-parcial:eu.sytue.rooms) respuesta="$(jq '.releases[1].status="inProgress" | .releases[1].userFraction=0.5' <<<"$respuesta")" ;;
    interno-par-otro-tag:eu.sytue.rooms) respuesta="$(jq '.releases[0].versionCodes=["3013005"]' <<<"$respuesta")" ;;
    interno-par-mezclado:eu.sytue.rooms) respuesta="$(jq '.releases[0].versionCodes=["3013004","3013005"]' <<<"$respuesta")" ;;
    interno-par-igual:eu.sytue.rooms) respuesta="$(jq '.releases[1].versionCodes=["3013004"]' <<<"$respuesta")" ;;
    interno-par-futuro:eu.sytue.rooms) respuesta="$(jq '.releases[1].versionCodes=["3013005"]' <<<"$respuesta")" ;;
    interno-par-duplicado:eu.sytue.rooms) respuesta="$(jq '.releases[1].versionCodes=["3013003","3013003"]' <<<"$respuesta")" ;;
    interno-par-texto:eu.sytue.rooms) respuesta="$(jq '.releases[1].versionCodes=["no-numerico"]' <<<"$respuesta")" ;;
    interno-par-vacio:eu.sytue.rooms) respuesta="$(jq '.releases[1].versionCodes=[]' <<<"$respuesta")" ;;
    interno-par-fraccion:eu.sytue.rooms) respuesta="$(jq '.releases[0].userFraction=0.5' <<<"$respuesta")" ;;
    interno-par-halted:eu.sytue.rooms) respuesta="$(jq '.releases[1].status="halted"' <<<"$respuesta")" ;;
    interno-par-tres:eu.sytue.rooms) respuesta="$(jq '.releases += [.releases[1]]' <<<"$respuesta")" ;;
    esac
    ;;
  interno-vacio:*) respuesta='{"track":"internal","releases":[]}' ;;
  interno-coexistente*:*)
    respuesta='{"track":"internal","releases":[{"versionCodes":["3014001"],"status":"draft","name":"NO-MOSTRAR","releaseNotes":[{"language":"es-ES","text":"NO-MOSTRAR"}],"inAppUpdatePriority":2},{"versionCodes":["3014000"],"status":"completed","name":"NO-MOSTRAR"}]}'
    case "$PLAY_ESCENARIO" in
    interno-coexistente-inverso) respuesta="$(jq '.releases |= reverse' <<<"$respuesta")" ;;
    interno-coexistente-duplicado) respuesta="$(jq '.releases[1].versionCodes=["3014001"]' <<<"$respuesta")" ;;
    interno-coexistente-mixto) respuesta="$(jq '.releases[0].versionCodes += ["3014000"]' <<<"$respuesta")" ;;
    interno-coexistente-ausente) respuesta="$(jq '.releases[0].versionCodes=["3014002"]' <<<"$respuesta")" ;;
    interno-coexistente-ajeno-draft) respuesta="$(jq '.releases[1].status="draft"' <<<"$respuesta")" ;;
    interno-coexistente-ajeno-nuevo) respuesta="$(jq '.releases[1].versionCodes=["3014002"]' <<<"$respuesta")" ;;
    interno-coexistente-ajeno-mixto) respuesta="$(jq '.releases[1].versionCodes += ["3013999"]' <<<"$respuesta")" ;;
    interno-coexistente-ajeno-numero) respuesta="$(jq '.releases[1].versionCodes=[3014000]' <<<"$respuesta")" ;;
    interno-coexistente-ajeno-enorme) respuesta="$(jq '.releases[1].versionCodes=["999999999999999999999999999999"]' <<<"$respuesta")" ;;
    interno-coexistente-desconocido) respuesta="$(jq '.releases[0].status="future"' <<<"$respuesta")" ;;
    interno-coexistente-fraccion) respuesta="$(jq '.releases[0].userFraction=0.5' <<<"$respuesta")" ;;
    interno-coexistente-tres) respuesta="$(jq '.releases += [{versionCodes:["3013999"],status:"completed"}]' <<<"$respuesta")" ;;
    esac
    ;;
  interno-track-invalido:eu.sytue.rooms) respuesta='{"track":"qa","releases":[{"versionCodes":["3013004"],"status":"draft","name":"NO-MOSTRAR","releaseNotes":[{"language":"es-ES","text":"NO-MOSTRAR"}]}]}' ;;
  interno-multiples:eu.sytue.rooms) respuesta='{"track":"internal","releases":[{"versionCodes":["3013003"],"status":"draft","name":"NO-MOSTRAR"},{"versionCodes":["3013004"],"status":"draft"}]}' ;;
  interno-parcial:eu.sytue.rooms) respuesta='{"track":"internal","releases":[{"versionCodes":["3013004"],"status":"inProgress","userFraction":0.5,"name":"NO-MOSTRAR"}]}' ;;
  interno-completado:*) respuesta='{"track":"internal","releases":[{"versionCodes":["3013004"],"status":"completed"}]}' ;;
  interno-anterior:*) respuesta='{"track":"internal","releases":[{"name":"Anterior","versionCodes":["3013003"],"status":"completed","releaseNotes":[{"language":"es-ES","text":"Notas revisadas"}],"countryTargeting":{"countries":["ES"]},"inAppUpdatePriority":2}]}' ;;
  interno-*:*) respuesta='{"track":"internal","releases":[{"name":"Conservar","versionCodes":["3013004"],"status":"draft","releaseNotes":[{"language":"es-ES","text":"Notas revisadas"}],"countryTargeting":{"countries":["ES"]},"inAppUpdatePriority":2}]}' ;;
  vacio:eu.sytue.rooms) respuesta='{"track":"internal","releases":[]}' ;;
  nuevo:eu.sytue.rooms) respuesta='{"track":"internal","releases":[{"versionCodes":["3012005"],"status":"draft"}]}' ;;
  mezclado:eu.sytue.rooms) respuesta='{"track":"internal","releases":[{"versionCodes":["3012004","3012005"],"status":"draft"}]}' ;;
  objetivo-v5:*) respuesta='{"track":"internal","releases":[{"versionCodes":["3012005"],"status":"draft"}]}' ;;
  pin-interno-mal:eu.sytue.rooms) respuesta='{"track":"internal","releases":[{"versionCodes":["3012024"],"status":"draft"}]}' ;;
  pin-*:*) respuesta='{"track":"internal","releases":[{"versionCodes":["3014000"],"status":"draft"}]}' ;;
  *) respuesta='{"track":"internal","releases":[{"versionCodes":["3012004"],"status":"draft"}]}' ;;
  esac
  fi
  ;;
*/tracks/production/releases)
  respuesta="$(produccion_actual | jq '{releases:[.releases[]? | {track:"production",activeArtifacts:[.versionCodes[] | {versionCode:tonumber}],releaseLifecycleState:(if .status == "draft" then "RELEASE_LIFECYCLE_STATE_DRAFT" else "RELEASE_LIFECYCLE_STATE_PUBLISHED" end)}]}')"
  case "${PLAY_ESCENARIO:-}" in
  pin-ya-completed-in-review) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_IN_REVIEW"' <<<"$respuesta")" ;;
  pin-ya-completed-approved) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_APPROVED_NOT_PUBLISHED"' <<<"$respuesta")" ;;
  esac
  if [ "$paquete" = eu.sytue.rooms ]; then
    case "${PLAY_ESCENARIO:-}" in
    pin-ciclo-in-review) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_IN_REVIEW"' <<<"$respuesta")" ;;
    pin-ciclo-approved) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_APPROVED_NOT_PUBLISHED"' <<<"$respuesta")" ;;
    pin-ciclo-desconocido) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_FUTURE"' <<<"$respuesta")" ;;
    pin-ciclo-ausente) respuesta="$(jq 'del(.releases[0].releaseLifecycleState)' <<<"$respuesta")" ;;
    pin-ciclo-vacio) respuesta='{"releases":[]}' ;;
    pin-ciclo-doble) respuesta="$(jq '.releases += [.releases[0]]' <<<"$respuesta")" ;;
    pin-ciclo-track) respuesta="$(jq '.releases[0].track="internal"' <<<"$respuesta")" ;;
    pin-ciclo-version) respuesta="$(jq '.releases[0].activeArtifacts=[{versionCode:3012023}]' <<<"$respuesta")" ;;
    pin-ciclo-mixto) respuesta="$(jq '.releases[0].activeArtifacts += [{versionCode:3014000}]' <<<"$respuesta")" ;;
    pin-ciclo-texto) respuesta="$(jq '.releases[0].activeArtifacts[0].versionCode |= tostring' <<<"$respuesta")" ;;
    pin-ciclo-campo) respuesta="$(jq '.releases[0].futuro="no adivinar"' <<<"$respuesta")" ;;
    pin-ciclo-error) codigo=503; respuesta='{"error":{"code":503,"message":"Consulta de revisión no disponible"}}' ;;
    pin-ya-completed-draft) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_DRAFT"' <<<"$respuesta")" ;;
    pin-ya-completed-not-approved) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_NOT_APPROVED"' <<<"$respuesta")" ;;
    pin-ya-completed-not-sent) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_NOT_SENT_FOR_REVIEW"' <<<"$respuesta")" ;;
    pin-ya-completed-unknown) respuesta="$(jq '.releases[0].releaseLifecycleState="RELEASE_LIFECYCLE_STATE_UNSPECIFIED"' <<<"$respuesta")" ;;
    pin-ya-completed-version) respuesta="$(jq '.releases[0].activeArtifacts=[{versionCode:3012024}]' <<<"$respuesta")" ;;
    vacio-con-resumen) respuesta='{"releases":[{"track":"production","activeArtifacts":[{"versionCode":3012004}],"releaseLifecycleState":"RELEASE_LIFECYCLE_STATE_DRAFT"}]}' ;;
    esac
  fi
  ;;
*/tracks/production)
  if [ "$metodo" = PUT ]; then
    printf '%s' "$cuerpo" >"$raiz/pendientes/$paquete.json"
  else
    respuesta="$(produccion_actual)"
  fi
  ;;
*:commit*)
  if [ "${PLAY_ESCENARIO:-}" = rechazar-commit ]; then
    codigo=400; respuesta='{"error":{"code":400,"message":"No se admite este commit"}}'
  elif [ -f "$raiz/pendientes/$paquete.json" ]; then
    mv "$raiz/pendientes/$paquete.json" "$raiz/tracks/$paquete.json"
  elif [ -f "$raiz/pendientes/$paquete-interno.json" ]; then
    mv "$raiz/pendientes/$paquete-interno.json" "$raiz/internos/$paquete.json"
  fi
  ;;
*) [ "$metodo" != DELETE ] || rm -f "$raiz/pendientes/$paquete.json" "$raiz/pendientes/$paquete-interno.json" ;;
esac
printf '%s\n' "$respuesta" >"$salida"
[ -z "$w" ] || printf '%s' "$codigo"
exit 0
SH
  chmod +x "$falso/bin/"*
  PATH="$falso/bin:$PATH" PLAY_CREDENCIALES="$falso/cuenta.json" RUNNER_TEMP="$falso" "$PLAY_GUION" --sondear mail >"$falso/salida" 2>&1 ||
    mal "play-publicar.sh --sondear contra la API de mentira falla: $(tail -1 "$falso/salida")"
  grep -q 'eu.sytue.mail' "$falso/salida" || mal "play-publicar.sh --sondear no sondea eu.sytue.mail"

  interno_falso() {
    local escenario="$1" estado="$2" tag="${3:-v3.13.4}" guion="${4:-$PLAY_GUION}"
    : >"$falso/peticiones"
    rm -f "$falso/internos/"*.json "$falso/pendientes/"*.json
    PATH="$falso/bin:$PATH" PLAY_ESCENARIO="$escenario" PLAY_CREDENCIALES="$falso/cuenta.json" \
      RUNNER_TEMP="$falso" "$guion" --interno --tag "$tag" --estado "$estado" mail rooms \
      >"$falso/interno" 2>&1
  }
  interno_falso interno-normal sondear || mal "no se puede consultar internal: $(tail -1 "$falso/interno")"
  grep -qE '^(PUT|PATCH)|:commit' "$falso/peticiones" && mal "la consulta internal modifica una pista"
  [ "$(grep -c '^GET .*testers/internal$' "$falso/peticiones")" = 2 ] || mal "no se consulta la configuración de probadores"
  grep -q 'testers-privados@example.com' "$falso/interno" && mal "el sondeo internal revela los grupos privados"
  [ "$(grep -c '^DELETE .*edits/e1$' "$falso/peticiones")" = 2 ] || mal "el sondeo internal deja ediciones abiertas"

  for escenario in interno-coexistente interno-coexistente-inverso; do
    interno_falso "$escenario" sondear v3.14.1 || mal "no se consulta el borrador junto a la versión activa: $escenario"
    grep -qE '^(PUT|PATCH)|:commit' "$falso/peticiones" && mal "consultar dos releases modifica una pista"
    [ "$(grep -c '^GET .*testers/internal$' "$falso/peticiones")" = 2 ] || mal "la consulta de dos releases pierde la lectura de probadores"
    [ "$(grep -c '^DELETE .*edits/e1$' "$falso/peticiones")" = 2 ] || mal "la consulta de dos releases deja ediciones abiertas"
    grep -qE 'NO-MOSTRAR|testers-privados@example.com' "$falso/interno" && mal "la consulta de dos releases revela notas, nombres o grupos"
    python3 - "$falso/interno" <<'PY' || mal "el sondeo no conserva las dos versiones y sus estados sin confundir el tag"
import json, sys
filas = [json.loads(linea) for linea in open(sys.argv[1]) if linea.startswith('{"app":')]
assert len(filas) == 2
for fila in filas:
    assert set(fila) == {"app", "expectedVersionCode", "internal", "googleGroups", "emailListsVisibleViaAPI"}
    assert fila["expectedVersionCode"] == "3014001"
    assert fila["googleGroups"] == 1 and fila["emailListsVisibleViaAPI"] is False
    assert sorted(fila["internal"], key=lambda release: release["versionCodes"]) == [
        {"versionCodes": ["3014000"], "status": "completed"},
        {"versionCodes": ["3014001"], "status": "draft"},
    ]
PY
    interno_falso "$escenario" completed v3.14.1 || mal "no se completa el borrador exacto junto a la versión anterior"
    [ "$(grep -c '^PUT .*tracks/internal$' "$falso/peticiones")" = 2 ] || mal "no se completa exactamente el borrador de las dos apps"
    [ "$(grep -c '^POST .*:commit' "$falso/peticiones")" = 2 ] || mal "no se confirman exactamente las dos pistas internas"
    grep -qE 'tracks/production|/listings/|/bundles\?uploadType|/testers/.*$' <(grep -E '^(PUT|PATCH|POST)' "$falso/peticiones") &&
      mal "completar el borrador coexistente modifica producción, ficha, bundles o probadores"
  done
  for escenario in duplicado mixto ausente ajeno-draft ajeno-nuevo ajeno-mixto ajeno-numero ajeno-enorme desconocido fraccion tres; do
    interno_falso "interno-coexistente-$escenario" sondear v3.14.1 && mal "el sondeo acepta releases ambiguas: $escenario"
    grep -qE '^(PUT|PATCH)|:commit' "$falso/peticiones" && mal "un sondeo ambiguo modifica pistas"
    [ "$(grep -c '^DELETE .*edits/e1$' "$falso/peticiones")" = 2 ] || mal "un sondeo ambiguo deja ediciones abiertas"
    grep -qE 'NO-MOSTRAR|testers-privados@example.com' "$falso/interno" && mal "el diagnóstico ambiguo revela notas, nombres o grupos"
  done
  echo 'ok - sondear admite el borrador exacto y la anterior activa; completed selecciona ese borrador; los estados ambiguos siguen bloqueados sin escrituras'

  for escenario in interno-track-invalido interno-multiples interno-parcial; do
    interno_falso "$escenario" sondear && mal "internal acepta $escenario"
    grep -qE '^(PUT|PATCH)|:commit' "$falso/peticiones" && mal "un sondeo internal rechazado modifica pistas"
    [ "$(grep -c '^DELETE .*edits/e1$' "$falso/peticiones")" = 2 ] || mal "un sondeo internal rechazado deja ediciones abiertas"
    grep -qE 'NO-MOSTRAR|testers-privados@example.com' "$falso/interno" && mal "el diagnóstico internal revela datos libres o grupos privados"
    python3 - "$falso/interno" <<'PY' || mal "internal no explica el rechazo con metadatos filtrados"
import json, sys
filas = [json.loads(linea) for linea in open(sys.argv[1]) if linea.startswith('{"app":')]
rechazo = next(fila for fila in filas if fila["app"] == "rooms")
assert set(rechazo) == {"app", "internal"}
assert set(rechazo["internal"]) == {"track", "releases"}
assert isinstance(rechazo["internal"]["track"], str)
assert rechazo["internal"]["releases"]
assert all(set(release) == {"versionCodes", "status"} for release in rechazo["internal"]["releases"])
PY
  done

  for escenario in interno-normal interno-anterior; do
    interno_falso "$escenario" completed || mal "no se completa internal con el AAB ya presente"
    [ "$(grep -c '^PUT .*tracks/internal$' "$falso/peticiones")" = 2 ] || mal "internal no actualiza exactamente las dos apps"
    grep -qE 'tracks/production|/listings/|/bundles\?uploadType|/testers/.*$' <(grep -E '^(PUT|PATCH|POST)' "$falso/peticiones") &&
      mal "completar internal modifica producción, ficha, AAB o probadores"
    jq -e '.releases[0] | .versionCodes == ["3013004"] and .status == "completed" and
      .releaseNotes[0].text == "Notas revisadas" and .countryTargeting.countries == ["ES"] and
      .inAppUpdatePriority == 2' "$falso/internos/eu.sytue.mail.json" >/dev/null ||
      mal "internal pierde el pin, notas, targeting o prioridad existentes"
  done
  interno_falso interno-completado completed || mal "internal completed no es idempotente"
  grep -qE '^PUT|:commit' "$falso/peticiones" && mal "internal ya completado se vuelve a publicar"
  interno_falso interno-falta completed && mal "se completa internal sin el bundle del tag"
  grep -qE '^PUT|:commit' "$falso/peticiones" && mal "un preflight incompleto modifica otra app"
  interno_falso interno-normal completed v3.13.5 && mal "internal ignora el tag esperado"
  grep -qE '^PUT|:commit' "$falso/peticiones" && mal "un tag diferente modifica una pista"
  interno_falso interno-rechazado completed && mal "se oculta el rechazo draft de Google"
  grep -q 'Only releases with status draft' "$falso/interno" || mal "se pierde el motivo del rechazo internal"
  interno_falso interno-vacio completed || mal "internal vacío no admite el bundle ya subido"
  for escenario in interno-par-borrador-primero interno-par-activo-primero; do
    interno_falso "$escenario" sondear || mal "el sondeo rechaza el draft del tag junto al completed anterior"
    grep -qE '^(PUT|PATCH)|:commit' "$falso/peticiones" && mal "sondear el par modifica pistas"
    [ "$(grep -c '^DELETE .*edits/e1$' "$falso/peticiones")" = 2 ] || mal "sondear el par deja ediciones abiertas"
    interno_falso "$escenario" completed || mal "no se completa el draft del tag junto al completed anterior"
    [ "$(grep -c '^PUT .*tracks/internal$' "$falso/peticiones")" = 2 ] || mal "el par no publica exactamente las dos apps"
    jq -e '.releases | length == 1 and .[0].versionCodes == ["3013004"] and .[0].status == "completed" and
      .[0].releaseNotes[0].text == "Notas del borrador" and .[0].countryTargeting.countries == ["ES"] and
      .[0].inAppUpdatePriority == 3' "$falso/internos/eu.sytue.mail.json" >/dev/null ||
      mal "el par hereda campos del activo por posición o pierde el pin"
  done
  for escenario in dos-drafts dos-completed parcial otro-tag mezclado igual futuro duplicado texto vacio fraccion halted tres; do
    interno_falso "interno-par-$escenario" completed && mal "internal acepta un par ambiguo: $escenario"
    grep -qE '^(PUT|PATCH)|:commit' "$falso/peticiones" && mal "el par inválido modifica otra app: $escenario"
    [ "$(grep -c '^DELETE .*edits/e1$' "$falso/peticiones")" = 2 ] || mal "el par inválido deja ediciones: $escenario"
  done
  # Producción mantiene un pin distinto: no admite aún un completed anterior.
  : >"$falso/peticiones"
  PATH="$falso/bin:$PATH" PLAY_ESCENARIO=interno-par-borrador-primero PLAY_CREDENCIALES="$falso/cuenta.json" \
    RUNNER_TEMP="$falso" PLAY_CONFIRMAR_PRODUCCION=PUBLICAR_15_APPS \
    "$PLAY_GUION" --produccion --tag v3.13.4 --estado draft >"$falso/produccion-par" 2>&1 &&
    mal "producción acepta el par antes de completar la actualización interna"
  grep -qE '^(PUT|PATCH)|:commit' "$falso/peticiones" && mal "el par interno permite modificar production"
  [ "$(grep -c '^DELETE .*edits/e1$' "$falso/peticiones")" = 15 ] || mal "el rechazo de production deja ediciones abiertas"
  [ "$(grep -c 'internal debe contener exclusivamente versionCode 3013004' "$falso/produccion-par")" = 15 ] ||
    mal "el rechazo de production no procede de su pin independiente"
  echo 'ok - internal consulta sin publicar y completa sólo el tag conservando probadores y targeting'
  "$PLAY_GUION" --interno --tag v3.13.4 --estado completed mail mail >/dev/null 2>&1 &&
    mal "internal acepta dos ediciones simultáneas de la misma app"
  clave="$(sed -n 2p "$falso/clave.pem")"
  if grep -qF -e "$clave" -e TOKEN-QUE-NO-SE-VE "$falso/argv" "$falso/salida"; then
    mal "play-publicar.sh deja la clave privada o el token en argv o en el log"
  fi
  ! compgen -G "$falso/play-*" >/dev/null || mal "play-publicar.sh no borra su directorio temporal (la clave)"

  : >"$falso/peticiones"
  PATH="$falso/bin:$PATH" PLAY_CREDENCIALES="$falso/cuenta.json" RUNNER_TEMP="$falso" \
    "$PLAY_GUION" --huellas --tag v3.13.4 mail >"$falso/huellas" 2>"$falso/error-huellas" ||
    mal "la consulta de huellas de Play falla: $(tail -1 "$falso/error-huellas")"
  jq -e '.package_name == "eu.sytue.mail" and .version_code == 3013004 and .certificate_sha256 == ["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"]' \
    "$falso/huellas" >/dev/null || mal "Play no conserva las firmas finales rotadas y normalizadas del tag"
  [ "$(cat "$falso/peticiones")" = 'GET https://androidpublisher.googleapis.com/androidpublisher/v3/applications/eu.sytue.mail/generatedApks/3013004' ] ||
    mal "consultar huellas de Play abre una edición, publica o consulta otra versión"
  PATH="$falso/bin:$PATH" PLAY_CREDENCIALES="$falso/cuenta.json" RUNNER_TEMP="$falso" PLAY_ESCENARIO=huellas-base64 \
    "$PLAY_GUION" --huellas --tag v3.13.4 mail >"$falso/huellas" 2>"$falso/error-huellas" ||
    mal "Play no acepta las codificaciones públicas de SHA256 de 32 bytes"
  jq -e '.certificate_sha256 == ["ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"]' "$falso/huellas" >/dev/null ||
    mal "las codificaciones Base64, URL y hexadecimal no coinciden byte a byte"
  for escenario in sin-huellas huella-rota; do
    PATH="$falso/bin:$PATH" PLAY_CREDENCIALES="$falso/cuenta.json" RUNNER_TEMP="$falso" PLAY_ESCENARIO="$escenario" \
      "$PLAY_GUION" --huellas --tag v3.13.4 mail >"$falso/huellas" 2>"$falso/error-huellas" &&
      mal "la consulta de huellas acepta $escenario"
  done
  : >"$falso/peticiones"
  PATH="$falso/bin:$PATH" PLAY_CREDENCIALES="$falso/cuenta.json" RUNNER_TEMP="$falso" PLAY_ESCENARIO=falta-docus \
    "$PLAY_GUION" --huellas --tag v3.13.4 docus mail >"$falso/huellas" 2>"$falso/error-huellas" &&
    mal "un AAB ausente se considera catálogo completo"
  jq -e '.package_name == "eu.sytue.mail"' "$falso/huellas" >/dev/null || mal "la consulta se para antes de comprobar las otras apps"
  grep -q 'catálogo incompleto; faltan certificados finales de: docus' "$falso/error-huellas" || mal "no se identifica la app sin firma final"
  [ "$(grep -c '^GET .*generatedApks/3013004$' "$falso/peticiones")" -eq 2 ] || mal "un AAB ausente cambia las consultas de versión o escribe en Play"
  "$PLAY_GUION" --huellas mail >/dev/null 2>&1 && mal "la consulta de huellas acepta una versión implícita"
  "$PLAY_GUION" --huellas --tag v3.13.4 --aab /tmp mail >/dev/null 2>&1 && mal "la consulta de huellas permite subir AAB"
  "$PLAY_GUION" --huellas --tag v3.13.4 ../mail >/dev/null 2>&1 && mal "la consulta de huellas acepta una ruta como app"
  if grep -qF -e "$clave" -e TOKEN-QUE-NO-SE-VE "$falso/argv" "$falso/huellas" "$falso/error-huellas"; then
    mal "la consulta de huellas expone credenciales"
  fi

  reiniciar_play() {
    rm -rf "$falso/tracks" "$falso/pendientes"
    : >"$falso/peticiones"
    : >"$falso/autenticaciones"
  }
  produccion_falsa() {
    local guion="$1" escenario="$2" estado="$3" tag="${4:-v3.12.4}" anterior="${5:-}"
    local args=(--produccion --tag "$tag" --estado "$estado")
    [ -z "$anterior" ] || args+=(--borrador-anterior "$anterior")
    PATH="$falso/bin:$PATH" PLAY_CREDENCIALES="$falso/cuenta.json" RUNNER_TEMP="$falso" \
      PLAY_CONFIRMAR_PRODUCCION=PUBLICAR_15_APPS PLAY_ESCENARIO="$escenario" \
      "$guion" "${args[@]}" >"$falso/produccion" 2>&1
  }
  rechaza_sin_promover() {
    reiniciar_play
    produccion_falsa "$1" "$2" draft "${3:-v3.12.4}" "${4:-}" && return 1
    ! grep -Eq '^PUT |^POST .*:commit' "$falso/peticiones"
  }
  payload_anterior_correcto() {
    jq -e --arg s "$1" '. == {track:"production",releases:[{name:"v3.14.0",versionCodes:["3014000"],status:$s,
      releaseNotes:[{language:"es-ES",text:"Notas revisadas"},{language:"en-US",text:"Reviewed notes"}],inAppUpdatePriority:3}]}' "$2" >/dev/null
  }
  for escenario in vacio nuevo mezclado estable otro-borrador vacio-con-resumen; do
    rechaza_sin_promover "$PLAY_GUION" "$escenario" ||
      mal "producción modifica releases pese a preflight $escenario en la última app"
    [ "$(grep -c '^GET .*/tracks/production' "$falso/peticiones")" -ge 14 ] ||
      mal "el caso $escenario no alcanzó la última app antes de rechazar"
  done
  rechaza_sin_promover "$PLAY_GUION" normal v3.12.5 || mal "producción no fija el versionCode al tag solicitado"

  # Ni el tag ni el estado tienen un valor implícito, y una selección parcial no publica.
  for args in '--estado draft' '--tag v3.12.4' '--tag v3.12.4 --estado raro' '--tag v3.12.4 --estado draft mail'; do
    reiniciar_play
    # shellcheck disable=SC2086
    PATH="$falso/bin:$PATH" PLAY_CREDENCIALES="$falso/cuenta.json" RUNNER_TEMP="$falso" \
      PLAY_CONFIRMAR_PRODUCCION=PUBLICAR_15_APPS "$PLAY_GUION" --produccion $args >"$falso/produccion" 2>&1 &&
      mal "producción acepta parámetros incompletos o parciales: $args"
    [ ! -s "$falso/peticiones" ] || mal "parámetros inválidos de producción abren una edición"
  done

  reiniciar_play
  produccion_falsa "$PLAY_GUION" normal draft || mal "no se preparan los quince borradores: $(tail -1 "$falso/produccion")"
  [ "$(grep -c '^PUT ' "$falso/peticiones")" -eq 15 ] || mal "draft no escribe quince tracks production"
  [ "$(grep -c '^POST .*:commit?' "$falso/peticiones")" -eq 15 ] || mal "draft no confirma quince ediciones"
  [ "$(grep -c '^POST .*:commit?changesInReviewBehavior=ERROR_IF_IN_REVIEW&changesNotSentForReview=true$' "$falso/peticiones")" -eq 15 ] ||
    mal "draft puede enviar cambios a revisión o cancelar una revisión existente"
  awk '/^PUT / {exit} /^GET .*\/tracks\/production$/ {n++} END {exit n != 15}' "$falso/peticiones" ||
    mal "el primer PUT de producción precede a los quince preflights"
  awk '/^PUT / {exit} /^GET .*\/applications\/eu\.sytue\.[a-z]+\/tracks\/production\/releases$/ {n++} END {exit n != 15}' "$falso/peticiones" ||
    mal "el primer PUT de producción precede a los quince controles de revisión fuera de la edición"
  for track in "$falso"/tracks/*.json; do
    jq -e '.track == "production" and .releases == [{name:"v3.12.4",versionCodes:["3012004"],status:"draft"}]' "$track" >/dev/null ||
      mal "draft usa otro versionCode/estado en ${track##*/}"
  done
  [ "$(grep -c 'sin envío a revisión ni publicación pública' "$falso/produccion")" -eq 15 ] ||
    mal "draft no explica que preparar un borrador no publica las apps"
  if grep -E 'bundles|listings|details|dataSafety' "$falso/peticiones"; then mal "producción toca AAB, ficha o declaraciones"; fi

  : >"$falso/peticiones"
  produccion_falsa "$PLAY_GUION" normal draft || mal "repetir el mismo borrador no es idempotente"
  ! grep -Eq '^PUT |^POST .*:commit' "$falso/peticiones" || mal "repetir el mismo borrador vuelve a modificar production"
  [ "$(grep -c '^DELETE ' "$falso/peticiones")" -eq 15 ] || mal "el rerun deja ediciones abiertas"

  reiniciar_play
  produccion_falsa "$PLAY_GUION" objetivo-v5 draft v3.12.5 || mal "el pin no admite un nuevo tag explícito con sus quince AAB"
  for track in "$falso"/tracks/*.json; do
    jq -e '.releases == [{name:"v3.12.5",versionCodes:["3012005"],status:"draft"}]' "$track" >/dev/null ||
      mal "el pin ignora el tag solicitado y conserva otro versionCode"
  done

  reiniciar_play
  produccion_falsa "$PLAY_GUION" mismo completed || mal "completed no completa el mismo borrador"
  [ "$(grep -c '^POST .*:commit?changesInReviewBehavior=ERROR_IF_IN_REVIEW$' "$falso/peticiones")" -eq 15 ] ||
    mal "completed no usa el commit explícito para revisión"
  for track in "$falso"/tracks/*.json; do
    jq -e '.releases == [{name:"Conservar",versionCodes:["3012004"],status:"completed",releaseNotes:[{language:"es-ES",text:"Notas revisadas"}]}]' "$track" >/dev/null ||
      mal "completar el borrador pierde las notas o cambia el versionCode"
  done
  reiniciar_play
  produccion_falsa "$PLAY_GUION" rechazar-commit draft && mal "se oculta un rechazo del commit draft"
  [ "$(grep -c '^PUT ' "$falso/peticiones")" -eq 1 ] || mal "el rechazo draft provoca otro intento/estado o continúa escribiendo"
  ! grep -q 'status.*completed' "$falso"/pendientes/*.json 2>/dev/null || mal "un rechazo draft cae automáticamente a completed"

  # El borrador anterior sólo puede sustituirse con su tag explícito. Reproduce
  # v3.12.24 -> v3.14.0: el éxito HTTP por sí solo no demuestra que el payload haya
  # dejado de apuntar al AAB anterior, ni que el rerun no lo omita como idempotente.
  rechaza_sin_promover "$PLAY_GUION" pin-normal v3.14.0 ||
    mal "producción sustituye v3.12.24 sin --borrador-anterior"
  rechaza_sin_promover "$PLAY_GUION" pin-normal v3.14.0 v3.12.23 ||
    mal "producción sustituye un borrador distinto del pin explícito"
  for escenario in pin-otro pin-mezclado pin-futuro pin-completed pin-inProgress pin-halted pin-dos \
    pin-campo-desconocido pin-countryTargeting pin-userFraction pin-notas-invalidas pin-prioridad-invalida pin-interno-mal; do
    rechaza_sin_promover "$PLAY_GUION" "$escenario" v3.14.0 v3.12.24 ||
      mal "el pin de borrador anterior permite modificar releases con $escenario"
    [ "$(grep -c '^POST .*/edits$' "$falso/peticiones")" -eq 15 ] ||
      mal "$escenario no comprobó la última app antes de rechazar todas las escrituras"
  done
  # El estado de revisión no aparece en tracks.get. Incluso un draft editable
  # debe coincidir con exactamente un resumen DRAFT y su artefacto numérico.
  for escenario in pin-ciclo-in-review pin-ciclo-approved pin-ciclo-desconocido pin-ciclo-ausente pin-ciclo-vacio \
    pin-ciclo-doble pin-ciclo-track pin-ciclo-version pin-ciclo-mixto pin-ciclo-texto pin-ciclo-campo pin-ciclo-error; do
    rechaza_sin_promover "$PLAY_GUION" "$escenario" v3.14.0 v3.12.24 ||
      mal "producción modifica releases con revisión desconocida o bloqueada: $escenario"
    [ "$(grep -c '^GET .*/applications/eu\.sytue\.[a-z]*/tracks/production/releases$' "$falso/peticiones")" -eq 15 ] ||
      mal "$escenario no consultó los quince estados de revisión antes de rechazar"
  done

  # Las opciones se validan antes de firmar/solicitar OAuth, no después de abrir
  # una edición. Ni otra modalidad ni un pin igual/futuro admite esta excepción.
  for args in '--produccion --tag v3.14.0 --estado draft --borrador-anterior v3.14.0' \
    '--produccion --tag v3.14.0 --estado draft --borrador-anterior v3.14.1' \
    '--produccion --tag v3.14.0 --estado draft --borrador-anterior anterior' \
    '--produccion --tag v3.14.0 --estado draft --borrador-anterior' \
    '--interno --tag v3.14.0 --estado completed --borrador-anterior v3.12.24' \
    '--huellas --tag v3.14.0 --borrador-anterior v3.12.24' \
    '--sondear --borrador-anterior v3.12.24' '--seco --borrador-anterior v3.12.24'; do
    reiniciar_play
    # shellcheck disable=SC2086
    PATH="$falso/bin:$PATH" PLAY_CREDENCIALES="$falso/cuenta.json" RUNNER_TEMP="$falso" \
      PLAY_CONFIRMAR_PRODUCCION=PUBLICAR_15_APPS "$PLAY_GUION" $args >"$falso/produccion" 2>&1 &&
      mal "se acepta un pin inválido o fuera de producción: $args"
    if [ -s "$falso/peticiones" ] || [ -s "$falso/autenticaciones" ]; then
      mal "un pin inválido inicia OAuth o abre una edición: $args"
    fi
  done

  for estado_pin in draft completed; do
    reiniciar_play
    produccion_falsa "$PLAY_GUION" pin-normal "$estado_pin" v3.14.0 v3.12.24 ||
      mal "no se sustituye el borrador fijado v3.12.24 por v3.14.0/$estado_pin: $(tail -1 "$falso/produccion")"
    [ "$(grep -c '^PUT .*tracks/production$' "$falso/peticiones")" -eq 15 ] ||
      mal "sustituir el borrador anterior no escribe exactamente las quince apps"
    [ "$(grep -c '^POST .*:commit?' "$falso/peticiones")" -eq 15 ] ||
      mal "sustituir el borrador anterior no confirma exactamente las quince ediciones"
    awk '/^PUT / {exit} /^GET .*\/applications\/eu\.sytue\.[a-z]+\/tracks\/production\/releases$/ {n++} END {exit n != 15}' "$falso/peticiones" ||
      mal "se sustituye un borrador antes de conocer la revisión de las quince apps"
    for track in "$falso"/tracks/*.json; do
      payload_anterior_correcto "$estado_pin" "$track" ||
        mal "la sustitución $estado_pin conserva otro AAB/metadata o pierde notas/prioridad en ${track##*/}"
    done
    if [ "$estado_pin" = draft ]; then
      : >"$falso/peticiones"
      produccion_falsa "$PLAY_GUION" pin-normal draft v3.14.0 v3.12.24 ||
        mal "repetir el borrador sustituido deja de ser idempotente"
      ! grep -Eq '^PUT |^POST .*:commit' "$falso/peticiones" ||
        mal "repetir el mismo target con pin anterior vuelve a modificar producción"
      [ "$(grep -c '^DELETE ' "$falso/peticiones")" -eq 15 ] || mal "el rerun del borrador sustituido deja ediciones abiertas"
    else
      : >"$falso/peticiones"
      produccion_falsa "$PLAY_GUION" pin-normal completed v3.14.0 v3.12.24 ||
        mal "repetir completed del target ya publicado no permite reanudar el lote"
      ! grep -Eq '^PUT |^POST .*:commit' "$falso/peticiones" || mal "repetir completed del target vuelve a publicar"
      : >"$falso/peticiones"
      produccion_falsa "$PLAY_GUION" pin-normal draft v3.14.0 v3.12.24 &&
        mal "producción completed admite un downgrade a draft"
      ! grep -Eq '^PUT |^POST .*:commit' "$falso/peticiones" || mal "se modifica una release production ya completed"
    fi
  done
  # Reanudar no reenvía las apps ya entregadas a Google. Su lifecycle debe ser
  # inequívoco y corresponder al target; el resto conserva el pin del borrador.
  for escenario in pin-ya-completed-in-review pin-ya-completed-approved pin-ya-completed-published; do
    reiniciar_play
    produccion_falsa "$PLAY_GUION" "$escenario" completed v3.14.0 v3.12.24 ||
      mal "el rerun completed no reconoce el target ya entregado: $escenario"
    ! grep -Eq '^PUT |^POST .*:commit' "$falso/peticiones" || mal "$escenario vuelve a enviar una release ya completed"
    [ "$(grep -c '^DELETE ' "$falso/peticiones")" -eq 15 ] || mal "$escenario deja ediciones de consulta abiertas"
    [ "$(grep -c 'ya enviada; ciclo RELEASE_LIFECYCLE_STATE_.*; sin cambios' "$falso/produccion")" -eq 15 ] ||
      mal "$escenario no explica el estado de las quince apps ya enviadas"
  done
  for escenario in pin-ya-completed-draft pin-ya-completed-not-approved pin-ya-completed-not-sent pin-ya-completed-unknown pin-ya-completed-version; do
    reiniciar_play
    produccion_falsa "$PLAY_GUION" "$escenario" completed v3.14.0 v3.12.24 &&
      mal "el rerun da por entregada una release con lifecycle contradictorio: $escenario"
    ! grep -Eq '^PUT |^POST .*:commit' "$falso/peticiones" || mal "$escenario modifica una release antes de aclarar su estado"
  done
  reiniciar_play
  produccion_falsa "$PLAY_GUION" pin-parcial completed v3.14.0 v3.12.24 ||
    mal "no se reanuda un lote con seis apps ya completed y nueve borradores anteriores"
  if [ "$(grep -c '^PUT .*tracks/production$' "$falso/peticiones")" -ne 9 ] ||
    [ "$(grep -c '^POST .*:commit?' "$falso/peticiones")" -ne 9 ]; then
    mal "el rerun parcial no publica exactamente las nueve apps pendientes"
  fi
  [ "$(grep -c '^DELETE ' "$falso/peticiones")" -eq 6 ] || mal "el rerun parcial deja abiertas las seis ediciones ya completed"
  [ "$(grep -c 'ya enviada; ciclo RELEASE_LIFECYCLE_STATE_PUBLISHED; sin cambios' "$falso/produccion")" -eq 6 ] ||
    mal "el rerun parcial no distingue las seis apps ya entregadas"
  grep -Eq '^PUT .*applications/eu\.sytue\.(bases|calcus|calendar|cast|certipdf|contacts)/' "$falso/peticiones" &&
    mal "el rerun parcial vuelve a publicar una de las seis apps ya entregadas"
  for track in "$falso"/tracks/*.json; do
    payload_anterior_correcto completed "$track" || mal "el rerun parcial no fija el target o pierde metadata en ${track##*/}"
  done
  echo 'ok - production sustituye sólo el borrador anterior fijado y bloquea revisión/rollout antes de escribir'

  # Control negativo: al quitar el pin, el mismo guardián debe rechazar la mutación.
  mutacion="$falso/mutacion"
  mkdir -p "$mutacion/scripts"
  sed '/^preparar_produccion()/,/^}/s/^    \.track == "internal" and .*/    true/' \
    "$PLAY_GUION" >"$mutacion/scripts/play-publicar.sh"
  chmod +x "$mutacion/scripts/play-publicar.sh"
  ln -s "$AQUI/empaquetar.sh" "$mutacion/scripts/empaquetar.sh"
  ln -s "$FYNE/cmd" "$mutacion/cmd"
  if cmp -s "$PLAY_GUION" "$mutacion/scripts/play-publicar.sh"; then
    mal "el control negativo de Play no quitó el pin de internal"
  elif rechaza_sin_promover "$mutacion/scripts/play-publicar.sh" nuevo; then
    mal "el guardián de Play no ve en rojo la mutación sin pin"
  else
    [ "$(grep -c '^PUT ' "$falso/peticiones")" -eq 15 ] || mal "la mutación sin pin no llegó a escribir las quince releases"
    echo 'ok - el guardián de Play detecta la mutación sin pin del versionCode'
  fi
  python3 - "$PLAY_GUION" "$mutacion/scripts/internal-sin-pin.sh" <<'PY'
import sys
from pathlib import Path
source = Path(sys.argv[1]).read_text()
needle = 'any(.bundles[]?; (.versionCode | tostring) == $v)'
assert source.count(needle) == 1
Path(sys.argv[2]).write_text(source.replace(needle, 'true'))
PY
  chmod +x "$mutacion/scripts/internal-sin-pin.sh"
  if interno_falso interno-falta completed v3.13.4 "$mutacion/scripts/internal-sin-pin.sh"; then
    [ "$(grep -c '^PUT .*tracks/internal$' "$falso/peticiones")" = 2 ] || mal "la mutación internal no llegó a publicar el bundle ausente"
    echo 'ok - el guardián internal ve en rojo la mutación que acepta un bundle ausente'
  else
    mal "el control negativo internal no quitó la comprobación del bundle"
  fi
  if python3 - "$PLAY_GUION" "$mutacion/scripts" <<'PY'
import sys
from pathlib import Path
source = Path(sys.argv[1]).read_text()
mutations = {
    'production-sin-pin-anterior.sh': ('.versionCodes == [$anterior]', 'true'),
    'production-sin-ciclo.sh': ('.releaseLifecycleState == "RELEASE_LIFECYCLE_STATE_DRAFT"', 'true'),
    'production-sin-cambiar-version.sh': (
        'else .name = $t | .versionCodes = [$v] | .status = $s end] end)',
        'else .name = $t | .status = $s end] end)'),
}
for name, (needle, replacement) in mutations.items():
    assert source.count(needle) == 1, name
    Path(sys.argv[2], name).write_text(source.replace(needle, replacement))
PY
  then
    chmod +x "$mutacion/scripts/production-"*.sh
    if rechaza_sin_promover "$mutacion/scripts/production-sin-pin-anterior.sh" pin-otro v3.14.0 v3.12.24; then
      mal "el guardián no ve en rojo la mutación que ignora el borrador anterior autorizado"
    elif [ "$(grep -c '^PUT .*tracks/production$' "$falso/peticiones")" -eq 15 ]; then
      echo 'ok - el guardián production detecta la mutación que ignora el pin del borrador anterior'
    else
      mal "la mutación sin pin anterior no llegó a escribir el borrador incorrecto"
    fi
    if rechaza_sin_promover "$mutacion/scripts/production-sin-ciclo.sh" pin-ciclo-in-review v3.14.0 v3.12.24; then
      mal "el guardián no ve en rojo la mutación que acepta un borrador en revisión"
    elif [ "$(grep -c '^PUT .*tracks/production$' "$falso/peticiones")" -eq 15 ]; then
      echo 'ok - el guardián production detecta la mutación sin protección del ciclo de revisión'
    else
      mal "la mutación sin protección de revisión no llegó a escribir las quince releases"
    fi
    reiniciar_play
    if produccion_falsa "$mutacion/scripts/production-sin-cambiar-version.sh" pin-normal draft v3.14.0 v3.12.24; then
      [ "$(grep -c '^PUT .*tracks/production$' "$falso/peticiones")" -eq 15 ] || mal "la mutación del payload no llegó a escribir las quince releases"
      if payload_anterior_correcto draft "$falso/tracks/eu.sytue.mail.json"; then
        mal "el guardián no ve en rojo un payload que conserva el versionCode anterior"
      elif jq -e '.releases[0].versionCodes == ["3012024"]' "$falso/tracks/eu.sytue.mail.json" >/dev/null; then
        echo 'ok - el guardián production detecta la mutación que no sustituye el versionCode anterior'
      else
        mal "la mutación del payload no reprodujo la conservación del versionCode anterior"
      fi
    else
      mal "el control negativo no quitó la sustitución del versionCode del payload"
    fi
  else
    mal "no se pudieron generar las tres mutaciones del guardián production"
  fi
  if grep -qF -e "$clave" -e TOKEN-QUE-NO-SE-VE "$falso/argv" "$falso/produccion"; then
    mal "producción deja la clave privada o el token en argv o en el log"
  fi
fi
# Los workflows escriben la cuenta de servicio en $RUNNER_TEMP y la borran con un trap.
for wf in play-publicar release-mobile; do
  f="$RAIZ/.github/workflows/$wf.yml"
  { grep -q 'PLAY_CREDENCIALES="$RUNNER_TEMP/play.json"' "$f" && grep -q "trap 'rm -f \"\$PLAY_CREDENCIALES\"' EXIT" "$f"; } ||
    mal "$wf.yml no escribe la cuenta de servicio en \$RUNNER_TEMP con un trap que la borre"
done
{ grep -q 'confirmacion_produccion' "$RAIZ/.github/workflows/play-publicar.yml" &&
  grep -q 'PUBLICAR_15_APPS' "$RAIZ/.github/workflows/play-publicar.yml" &&
  grep -qF 'args=(--produccion --tag "$TAG" --estado "$ESTADO_PRODUCCION")' "$RAIZ/.github/workflows/play-publicar.yml"; } ||
  mal "play-publicar.yml no exige confirmar producción con tag y estado explícitos"
# Los permisos pedidos son los de <uses-permission>, no los que el proveedor de Drive
# exige a quien lo llama.
eval "$(sed -n '/^permisos_pedidos() {/,/^}/p' "$GUION")"
volcado='N: android=http://schemas.android.com/apk/res/android (line=2)
  E: manifest (line=2)
    E: uses-permission (line=7)
      A: http://schemas.android.com/apk/res/android:name(0x01010003)="android.permission.INTERNET" (Raw: "android.permission.INTERNET")
    E: uses-permission (line=8)
      A: http://schemas.android.com/apk/res/android:name(0x01010003)="android.permission.USE_BIOMETRIC" (Raw: "android.permission.USE_BIOMETRIC")
    E: application (line=10)
      E: provider (line=20)
        A: http://schemas.android.com/apk/res/android:permission(0x01010006)="android.permission.MANAGE_DOCUMENTS" (Raw: "android.permission.MANAGE_DOCUMENTS")'
[ "$(permisos_pedidos <<<"$volcado")" = "android.permission.INTERNET android.permission.USE_BIOMETRIC " ] ||
  mal "permisos_pedidos cuenta como pedido lo que el proveedor exige, o pierde un uses-permission: $(permisos_pedidos <<<"$volcado")"

grep -q 'no se suben a Google Play' "$RAIZ/.github/workflows/release-mobile.yml" ||
  mal "release-mobile.yml ya no se salta la subida a Play con un aviso cuando falta el secreto"

# BASH 3.2: lo que corre en el Mac (runner sytue-macos) lo lee el bash de macOS, y ese no
# entiende un `case` dentro de `$(…)` o `<(…)` cuyo patrón no abra con `(`: toma el `)`
# por el cierre y el guion ENTERO no se lee, con cualquier destino. Así cayó el primer
# `android-fyne` de verdad (v3.6.24). Aquí no hay bash 3.2, así que se mira la forma.
while IFS= read -r l; do
  mal "case dentro de \$(…)/<(…) que el bash 3.2 de macOS no sabe leer: $l"
done < <(grep -nE '[<$]\(case [^)]* in([[:space:]]*$|[[:space:]]+[^([:space:]])' \
  "$GUION" "$AQUI/apple-firma.sh" "$RAIZ"/.github/actions/toolchain/*.sh "$RAIZ/deploy/sytue-git/bin/publicar-asset.sh" "$RAIZ/.github/workflows/release-desktop.yml" 2>/dev/null)

# La CLI de Fyne codifica el manifiesto con una tabla de atributos anterior a la API 23:
# `android:autoVerify` (y cualquier otro que no esté en ella) tumba el AAB de TODAS las
# apps al llegar a la que lo declara (Rooms, v3.6.25). Se añade en el binario, no aquí.
if grep -n 'android:autoVerify\|android:foregroundServiceType' "$GUION" | grep -v '^[0-9]*:[[:space:]]*#'; then
  mal "empaquetar.sh escribe android:autoVerify o android:foregroundServiceType en el manifiesto: la CLI de Fyne no los sabe codificar"
fi

# La CLI de Fyne solo empaqueta UNA actividad, la GoNativeActivity: cualquier otra en el
# manifiesto tumba el AAB con «can only build an .apk for GoNativeActivity» (Rooms,
# v3.7.10, con la del permiso de compartir pantalla; todas las apps en v3.13.12 con
# SesionDesbloqueo inyectada por android-sesion.py). Lo que añade actividades o
# componentes adicionales solo puede ir en $ESCENARIO/android-$app.xml (el manifiesto de
# aapt2 link), nunca en $dir/AndroidManifest.xml (el que lee `fyne package`).
while IFS= read -r l; do
  mal "empaquetar.sh declara una actividad que la CLI de Fyne no empaqueta: $l"
done < <(grep -no '<activity android:name="[^"]*"' "$GUION" | grep -v '"org.golang.app.GoNativeActivity"')
grep -qF 'manifiesto_android "" "" >"$dir/AndroidManifest.xml"' "$GUION" ||
  mal "empaquetar.sh pasa componentes adicionales a \$dir/AndroidManifest.xml: fyne package rechaza SesionDesbloqueo"
grep -qF 'manifiesto_android "$sesion_queries" "$sesion_components" >"$ESCENARIO/android-$app.xml"' "$GUION" ||
  mal "empaquetar.sh no incluye sesion_queries y sesion_components en \$ESCENARIO/android-\$app.xml para el AAB final"

# El manifiesto común de TODAS las apps atiende el giro y los cambios de tamaño.
# No basta con orientation: desde API 13 también cambian screenSize/screenLayout.
# Antes se reservaban a las dos apps con PiP y las otras podían recrear la actividad.
rotacion_android_para_todas() {
  local cambios cambio
  cambios="$(sed -n 's/^[[:space:]]*android:configChanges="\([^"]*\)"[[:space:]]*$/\1/p' "$1")"
  case "$cambios" in '' | *'$'*) return 1 ;; esac
  for cambio in orientation keyboardHidden uiMode screenSize smallestScreenSize screenLayout; do
    case "|$cambios|" in *"|$cambio|"*) ;; *) return 1 ;; esac
  done
}
rotacion_android_para_todas "$GUION" ||
  mal "el manifiesto común de Android no conserva TODAS las apps al girar o redimensionar, incluidas las que no tienen PiP"
# Casos negativos: el guardián rechaza tanto perder los tamaños como volver a
# condicionar los flags a una capacidad de la app.
sed 's/|screenSize|smallestScreenSize|screenLayout//' "$GUION" >"$tmp/sin-rotacion-android.sh"
rotacion_android_para_todas "$tmp/sin-rotacion-android.sh" &&
  mal "el guardián acepta Android sin los cambios de tamaño del giro"
sed 's/android:configChanges="/android:configChanges="$pip|/' "$GUION" >"$tmp/rotacion-solo-pip.sh"
rotacion_android_para_todas "$tmp/rotacion-solo-pip.sh" &&
  mal "el guardián acepta que el giro dependa de PiP en vez de proteger todas las apps"

# --- 13. La Mac App Store: sandbox, derechos mínimos y sin actualizador propio ---------
mas_permisos="$tmp/Permisos.app"
mkdir -p "$mas_permisos/Contents"
chmod 755 "$mas_permisos" "$mas_permisos/Contents"
printf 'perfil público de distribución\n' >"$mas_permisos/Contents/embedded.provisionprofile"
chmod 600 "$mas_permisos/Contents/embedded.provisionprofile"
"$GUION" verificar-permisos-apple "$mas_permisos" >/dev/null 2>&1 &&
  mal "Apple acepta un perfil 0600 en un payload que pertenecerá a root"
chmod 644 "$mas_permisos/Contents/embedded.provisionprofile"
"$GUION" verificar-permisos-apple "$mas_permisos" >/dev/null 2>&1 ||
  mal "Apple rechaza un perfil 0644 públicamente legible"
chmod 700 "$mas_permisos/Contents"
"$GUION" verificar-permisos-apple "$mas_permisos" >/dev/null 2>&1 &&
  mal "Apple acepta un directorio privado aunque el perfil sea legible"
chmod 755 "$mas_permisos/Contents"
"$GUION" verificar-permisos-apple "$mas_permisos" >/dev/null 2>&1 ||
  mal "Apple rechaza el payload con directorios 0755 y perfil 0644"
echo 'ok - el payload de Apple rechaza perfiles 0600 y directorios privados'
# Visto en rojo, uno a uno: quitando `app-sandbox` de derechos_tienda, dándole a todas el
# derecho de ficheros, añadiendo `com.apple.security.print`, cambiando el grupo en
# base_tienda_darwin.go, firmando el .pkg con SYTUE_MAS_IDENTIDAD, quitando `--tags tienda`,
# convirtiendo el `elif` del Developer ID en `if`, quitando el `break` de los bundles,
# devolviendo actualizar_darwin.go a `darwin && !ios` y quitando DeTienda de Plataforma.
equipo=VHC5QW7BZ3
comunes='com.apple.application-identifier com.apple.developer.team-identifier com.apple.security.app-sandbox com.apple.security.application-groups com.apple.security.files.downloads.read-write com.apple.security.network.client com.apple.security.network.server keychain-access-groups'
ficheros='com.apple.security.files.user-selected.read-write'
llamadas='com.apple.security.device.audio-input com.apple.security.device.camera'
derechos_esperados() { # <app> -> las claves, ordenadas, en una línea
  local extra=""
  case "$1" in
  bases | calendar | hub) ;;
  cast | rooms) extra="$ficheros $llamadas" ;;
  calcus | certipdf | contacts | diapos | docus | drive | erp | mail | notes | photos) extra="$ficheros" ;;
  *) extra="app-sin-decidir" ;; # una app nueva tiene que decidirse AQUÍ, no heredar derechos
  esac
  printf '%s\n' $comunes $extra | sort | tr '\n' ' '
}
for toml in "$FYNE"/cmd/*/FyneApp.toml; do
  app="$(basename "$(dirname "$toml")")"
  [ "$app" != demo ] || continue
  plist="$("$GUION" derechos-tienda "$app" "$equipo" 2>&1)" || {
    mal "derechos-tienda $app falla: $plist"
    continue
  }
  tiene="$(grep -o '<key>[^<]*</key>' <<<"$plist" | sed 's/<[^>]*>//g' | sort | tr '\n' ' ')"
  [ "$tiene" = "$(derechos_esperados "$app")" ] ||
    mal "$app iría a la Mac App Store con «$tiene» y lo decidido es «$(derechos_esperados "$app")»"
  grep -q '<key>com.apple.security.app-sandbox</key><true/>' <<<"$plist" || mal "$app iría a la Mac App Store SIN App Sandbox"
  id="$(sed -n 's/^[[:space:]]*ID[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$toml" | sed -n 1p)"
  grep -qF "<string>$equipo.$id</string>" <<<"$plist" || mal "$app no se firma con su App ID ($equipo.$id)"
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import plistlib, sys; plistlib.loads(sys.stdin.buffer.read())' <<<"$plist" 2>/dev/null ||
      mal "los derechos de tienda de $app no son un plist válido"
  fi
done
"$GUION" derechos-tienda calcus equipo-raro >/dev/null 2>&1 && mal "derechos-tienda acepta un Team ID que no lo es"
# El grupo de la suite es el MISMO en la firma y en el código que busca la carpeta compartida.
grupo="$(sed -n 's/^const GrupoSuite = "\(.*\)"$/\1/p' "$RAIZ/apps/nucleo/ajustes/grupo_apple.go")"
plist="$("$GUION" derechos-tienda hub "$equipo")"
[ -n "$grupo" ] && [ "$(grep -c "<string>$grupo</string>" <<<"$plist")" = 2 ] ||
  mal "el grupo de la suite del código («$grupo») no es el de application-groups y keychain-access-groups de la firma"
python3 - "$GUION" "$FYNE" "$RAIZ/apps/nucleo/ajustes/grupo_apple.go" "$equipo" <<'PY' || mal "los hosts Apple no comparten la sesión con derechos mínimos, o piden el App Group con los widgets apagados"
import os, pathlib, plistlib, re, subprocess, sys, tomllib
guion, fyne, fuente, equipo = sys.argv[1:]
codigo = pathlib.Path(fuente).read_text()
llavero = re.findall(r'^const GrupoSuite = "([^"]+)"$', codigo, re.M)[0]
contenedor = re.findall(r'^const GrupoContenedorSuite = "([^"]+)"$', codigo, re.M)[0]
# Las dos posiciones del interruptor SYTUE_WIDGETS_APPLE: apagado, ni rastro del App Group
# (la firma de antes de los widgets); encendido, el contenedor común en todos los hosts.
for activo in ('0', '1'):
    entorno = dict(os.environ, SYTUE_WIDGETS_APPLE=activo)
    for toml in pathlib.Path(fyne, 'cmd').glob('*/FyneApp.toml'):
        if toml.parent.name == 'demo': continue
        datos = tomllib.loads(toml.read_text())
        appid = datos['Details']['ID']
        custom = datos.get('Development', {})
        for modo in ('ios', 'developerid'):
            p = plistlib.loads(subprocess.check_output(['bash', guion, 'derechos-sesion', str(toml.parent), equipo, modo], env=entorno))
            grupos = [contenedor] + (['group.' + appid] if custom.get('archivos') == 'si' else [])
            assert p['com.apple.security.application-groups'] == grupos, (appid, modo, p)
            claves = [equipo + '.' + appid, llavero] + ([equipo + '.' + appid + '.shared'] if custom.get('archivos') == 'si' else [])
            assert p['keychain-access-groups'] == claves, (appid, modo, p)
            assert ('com.apple.security.device.camera' in p) == (modo == 'developerid' and custom.get('llamadas') == 'si')
        tienda = plistlib.loads(subprocess.check_output(['bash', guion, 'derechos-tienda', str(toml.parent), equipo], env=entorno))
        assert tienda['com.apple.security.application-groups'] == [contenedor, llavero] + (['group.' + appid] if custom.get('archivos') == 'si' else []), (appid, activo)
assert subprocess.run(['bash', guion, 'widgets-apple'], env=dict(os.environ, SYTUE_WIDGETS_APPLE='1'), capture_output=True, text=True).stdout == 'si\n'
assert subprocess.run(['bash', guion, 'widgets-apple'], env=dict(os.environ, SYTUE_WIDGETS_APPLE='0'), capture_output=True, text=True).stdout == 'no\n'
assert subprocess.run(['bash', guion, 'widgets-apple'], env=dict(os.environ, SYTUE_WIDGETS_APPLE='quizá'), capture_output=True).returncode != 0
for modo in ('ios', 'macos'):
    extension = plistlib.loads(subprocess.check_output(['bash', guion, 'derechos-archivos', 'drive', equipo, modo]))
    assert llavero not in extension.get('keychain-access-groups', []), modo
PY
# Lo versionado manda: sin variable de entorno, el interruptor es el de Sytue.xcconfig, y
# un release no puede encenderlo sin que se vea en el diff del tag.
interruptor="$(sed -n 's/^SYTUE_WIDGETS_APPLE = \([A-Z]*\)$/\1/p' "$FYNE/xcode/Sytue.xcconfig")"
case "$interruptor:$(env -u SYTUE_WIDGETS_APPLE "$GUION" widgets-apple)" in
YES:si | NO:no) ;;
*) mal "empaquetar.sh widgets-apple no dice lo que fija Sytue.xcconfig («$interruptor»)" ;;
esac
grep -q 'SYTUE_WIDGETS_APPLE' "$RAIZ/.github/workflows/release-desktop.yml" &&
  ! grep -qE '^[[:space:]]+SYTUE_WIDGETS_APPLE:' "$RAIZ/.github/workflows/release-desktop.yml" ||
  mal "release-desktop.yml define SYTUE_WIDGETS_APPLE en el entorno o ya no lo consulta: el interruptor es el del tag"
"$GUION" derechos-sesion hub ABCDE12345 ios >/dev/null 2>&1 && mal "la sesión acepta otro equipo Apple"
"$GUION" derechos-sesion hub "$equipo" desconocido >/dev/null 2>&1 && mal "la sesión acepta una plataforma desconocida"
# LA EXTENSIÓN FILE PROVIDER (Drive en el Finder y en Archivos): solo la app que declara
# `archivos` lleva el segundo grupo de llavero, y ese grupo es el MISMO en la firma de la
# app, en la de la extensión y en el código que guarda la credencial. Visto en rojo dándole
# el grupo a todas, cambiando la constante de archivos.go y dándole a la extensión del Mac
# network.server.
grupo_archivos="$(sed -n 's/^[[:space:]]*GrupoLlaveroArchivos = "\(.*\)"$/\1/p' "$RAIZ/apps/nucleo/sistema/archivos.go")"
for toml in "$FYNE"/cmd/*/FyneApp.toml; do
  app="$(basename "$(dirname "$toml")")"
  [ "$app" != demo ] || continue
  plist="$("$GUION" derechos-tienda "$app" "$equipo" 2>&1)"
  if grep -qE '^[[:space:]]*archivos[[:space:]]*=[[:space:]]*"si"' "$toml"; then
    grep -qF "<string>$grupo_archivos</string>" <<<"$plist" || mal "$app lleva la extensión de archivos y su firma no comparte el llavero «$grupo_archivos»"
  else
    case "$plist" in *.shared\<*) mal "$app no lleva la extensión de archivos y su firma comparte un llavero con ella" ;; esac
  fi
done
[ -n "$grupo_archivos" ] || mal "apps/nucleo/sistema/archivos.go ya no declara GrupoLlaveroArchivos"
for p in macos ios app-ios; do
  plist="$("$GUION" derechos-archivos drive "$equipo" "$p" 2>&1)" || {
    mal "derechos-archivos drive $p falla: $plist"
    continue
  }
  grep -qF "<string>$grupo_archivos</string>" <<<"$plist" || mal "los derechos de archivos ($p) no llevan el llavero «$grupo_archivos»"
  tiene="$(grep -o '<key>[^<]*</key>' <<<"$plist" | sed 's/<[^>]*>//g' | sort | tr '\n' ' ')"
  case "$p" in
  macos) quiero='com.apple.application-identifier com.apple.developer.team-identifier com.apple.security.app-sandbox com.apple.security.application-groups com.apple.security.network.client keychain-access-groups ' ;;
  ios) quiero='application-identifier com.apple.developer.default-data-protection com.apple.developer.team-identifier com.apple.security.application-groups keychain-access-groups ' ;;
  app-ios) quiero='keychain-access-groups ' ;;
  esac
  [ "$tiene" = "$quiero" ] || mal "la extensión de archivos ($p) se firmaría con «$tiene» y lo decidido es «$quiero»"
done
# En iOS el PRIMER grupo de la app es su App ID: es donde guarda la sesión sin grupo.
plist="$("$GUION" derechos-archivos drive "$equipo" app-ios)"
grep -qF "<array><string>$equipo.eu.sytue.drive</string>" <<<"$plist" || mal "en iOS la sesión de Drive cambiaría de grupo de llavero"
"$GUION" derechos-archivos hub "$equipo" ios >/dev/null 2>&1 && mal "derechos-archivos firma una extensión para una app que no la declara"
# Un .pkg por app (no por bundle), y el .dmg de fuera de la tienda sigue siendo el suyo.
quiero="$(for a in bases calcus calendar cast certipdf contacts diapos docus drive erp hub mail notes photos rooms; do printf 'sytue-%s-macos-tienda.pkg\n' "$a"; done)"
[ "$("$GUION" publicables macappstore 2>&1)" = "$quiero" ] || mal "publicables macappstore no da un .pkg por cada una de las quince apps"
[ "$("$GUION" publicables macos 2>&1)" = "$(printf 'sytue-drive-macos-universal.dmg\nsytue-one-macos-universal.dmg')" ] ||
  mal "la variante de tienda ha cambiado lo que se publica como .dmg: $("$GUION" publicables macos 2>&1 | tr '\n' ' ')"
# Lo que solo se puede leer en el guion sin un Mac: quién firma qué, y con qué etiqueta.
grep -qF 'productbuild --component "$ESCENARIO/tienda/$app/$nombre.app" /Applications' "$GUION" &&
  grep -qF -- '--sign "$SYTUE_MAS_INSTALADOR" "$destino"' "$GUION" ||
  mal "el .pkg de la Mac App Store no se firma con el certificado de instalador (SYTUE_MAS_INSTALADOR)"
grep -qF 'grep -qF "$SYTUE_MAS_INSTALADOR" <<<"$firma_pkg"' "$GUION" || mal "no se comprueba quién firma el .pkg"
grep -qF 'codesign -f -s "$SYTUE_MAS_IDENTIDAD" --options runtime --timestamp --entitlements "$ent" "$app"' "$GUION" ||
  mal "el .app de la Mac App Store no se firma con el certificado de distribución y sus derechos"
grep -qF 'install -m 0644 "$perfil" "$app/Contents/embedded.provisionprofile"' "$GUION" || mal "el .app de la Mac App Store no lleva el perfil embebido públicamente legible"
grep -qF 'compilar_mac "$exe" ${etiquetas[@]+"${etiquetas[@]}"}' "$GUION" &&
  grep -qF '[ -z "$tienda" ] || etiquetas=(--tags "tienda,goolm")' "$GUION" &&
  grep -qF 'GOARCH=arm64 compilar_fyne "$salida.arm64" "$@"' "$GUION" &&
  grep -qF 'GOARCH=amd64 compilar_fyne "$salida.x86_64" "$@"' "$GUION" &&
  grep -qF 'compilar_mac "$salida" --tags tienda,goolm' "$GUION" ||
  mal "las dos mitades del universal de tienda (macappstore y la fase de Xcode Cloud) no se compilan con la etiqueta «tienda»"
# El .dmg no se ve afectado: Developer ID solo fuera de la tienda, y la tienda sin bundles.
grep -qF 'elif [ "$plataforma" = macos ] && [ -n "${SYTUE_MACOS_IDENTIDAD:-}" ]; then' "$GUION" ||
  mal "la firma de Developer ID y la de la tienda ya no se excluyen: una app llevaría las dos"
grep -qF '[ -z "$tienda" ] || break' "$GUION" || mal "la variante de tienda armaría además los .dmg de los bundles"
# La declaración autorizada vive en los Info.plist canónicos. La ruta manual la
# copia al artefacto, antes de firmarlo, sin cambiar sus otros metadatos o derechos.
python3 - "$GUION" "$FYNE/xcode" "$tmp" <<'PY' || mal "el plist de tienda no conserva la declaración canónica de cifrado"
import pathlib, plistlib, subprocess, sys
guion, xcode, tmp = map(pathlib.Path, sys.argv[1:])
uso, codigo = "ITSAppUsesNonExemptEncryption", "ITSEncryptionExportComplianceCode"
texto = guion.read_text()
llamada = 'copiar_cifrado_apple "$FYNE/xcode/Info-$plataforma.plist" "$plist"'
assert texto.index(llamada) < texto.index('[ -z "$ipa_abierto" ] || firmar_ipa')
assert texto.index(llamada) < texto.index('firmar_tienda "$destino" "$dir" "$appid"')
assert 'if [ "$plataforma" = ios ] || [ -n "$tienda" ]; then\n        ' + llamada in texto
for p in ("ios", "macos"):
    for formato in (plistlib.FMT_XML, plistlib.FMT_BINARY):
        fuente = xcode / ("Info-%s.plist" % p)
        destino = tmp / ("artefacto-%s.plist" % p)
        previo = {"CFBundleIdentifier": "eu.sytue.rooms", "CFBundleVersion": "42",
                  "CFBundleURLTypes": [{"CFBundleURLSchemes": ["sytue-rooms"]}],
                  uso: True, codigo: "obsoleto-no-enviar"}
        destino.write_bytes(plistlib.dumps(previo, fmt=formato))
        r = subprocess.run(["bash", str(guion), "cifrado-apple", str(fuente), str(destino)], capture_output=True)
        assert r.returncode == 0, r.stderr
        canonico = plistlib.loads(fuente.read_bytes())
        esperado = {k: v for k, v in previo.items() if k not in (uso, codigo)}
        esperado.update({k: canonico[k] for k in (uso, codigo) if k in canonico})
        assert plistlib.loads(destino.read_bytes()) == esperado
        assert destino.read_bytes().startswith(b"bplist00") == (formato == plistlib.FMT_BINARY)
    fuente = tmp / ("canonico-%s.plist" % p)
    # Un código autorizado futuro también se copia: no se vuelve a decidir el cifrado aquí.
    fuente.write_bytes(plistlib.dumps({uso: True, codigo: "codigo-de-prueba-no-enviar"}))
    r = subprocess.run(["bash", str(guion), "cifrado-apple", str(fuente), str(destino)], capture_output=True)
    assert r.returncode == 0, r.stderr
    info = plistlib.loads(destino.read_bytes())
    assert info[uso] is True and info[codigo] == "codigo-de-prueba-no-enviar"
    for roto in ({}, {uso: True}, {uso: "false"}, {uso: False, codigo: "obsoleto"}, {uso: True, codigo: "[]"}):
        fuente.write_bytes(plistlib.dumps(roto))
        original = destino.read_bytes()
        r = subprocess.run(["bash", str(guion), "cifrado-apple", str(fuente), str(destino)], capture_output=True)
        assert r.returncode != 0 and destino.read_bytes() == original, roto
print("ok - el cifrado canónico llega al plist de tienda sin alterar los demás metadatos")
PY
# SIN ACTUALIZADOR PROPIO en la variante de tienda (la Mac App Store lo prohíbe): el
# montaje del DMG ni se compila, y no hay plataforma que buscar en el catálogo.
tienda_go="$(cd "$RAIZ/apps/nucleo" && GOFLAGS=-mod=mod GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go list -tags tienda -f '{{.GoFiles}} {{.CgoFiles}}' ./sistema/escritorio ./auth 2>&1)"
case "$tienda_go" in *actualizar_darwin.go*) mal "la variante de tienda compila el actualizador de macOS (actualizar_darwin.go)" ;; esac
case "$tienda_go" in *actualizar_otros.go*tienda.go*store_apple.go*) ;; *) mal "la variante de tienda no compila lo que debe (sin actualizador, DeTienda, llavero de Apple): $tienda_go" ;; esac
case "$tienda_go" in *" store.go"* | *"[store.go"*) mal "la variante de tienda guardaría la sesión con /usr/bin/security (store.go), que el sandbox no deja compartir" ;; esac
normal_go="$(cd "$RAIZ/apps/nucleo" && GOFLAGS=-mod=mod GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go list -f '{{.GoFiles}} {{.CgoFiles}}' ./sistema/escritorio ./auth 2>&1)"
case "$normal_go" in *actualizar_darwin.go*tienda_no.go*store.go*) ;; *) mal "el macOS de fuera de la tienda ha perdido su actualizador o su llavero: $normal_go" ;; esac
# Sin tubería hacia `grep -q` (ci-sin-azar-test.sh).
trozo="$(grep -A3 '^func Plataforma() string {' "$RAIZ/apps/nucleo/sistema/escritorio/actualizar.go")"
grep -q 'if DeTienda {' <<<"$trozo" ||
  mal "escritorio.Plataforma ya no es vacía en la variante de tienda: el Hub buscaría actualizaciones propias"
trozo="$(grep -A3 'if !escritorio.DeTienda {' "$FYNE/cmd/hub/main.go")"
grep -q 'ElementoActualizar' <<<"$trozo" ||
  mal "el Hub de la Mac App Store enseñaría «Buscar actualizaciones» en su menú"

# --- 14. Xcode Cloud: el proyecto versionado es el que sale del árbol -------------------
# Xcode Cloud solo construye un proyecto de Xcode que esté en el repositorio, así que
# apps/fyne/xcode/Sytue.xcodeproj se genera (generar.py) y se versiona. Aquí: que sea el
# de las apps de cmd/ con sus derechos de tienda (una app nueva o un derecho cambiado sin
# regenerar es rojo), que ci_post_clone.sh sea ejecutable (si no, Xcode Cloud lo corre con
# zsh) y que los Info.plist omitan el par de exportación mientras no haya código aprobado. Visto en
# rojo editando calcus/tienda.entitlements, borrando la carpeta de una app, añadiendo una
# carpeta sobrante y quitando el bit de ejecución.
if command -v python3 >/dev/null 2>&1; then
  salida="$(python3 "$FYNE/xcode/generar.py" --comprobar 2>&1)" || mal "Sytue.xcodeproj no es el que genera generar.py: $salida"
fi
[ "$(git -C "$RAIZ" ls-files -s apps/fyne/xcode/ci_scripts/ci_post_clone.sh | cut -c1-6)" = 100755 ] ||
  mal "apps/fyne/xcode/ci_scripts/ci_post_clone.sh no está versionado como ejecutable: Xcode Cloud lo correría con zsh"
# Una rama de Xcode Cloud llega shallow y sin tags. La release alcanzable debe ser
# 3.12.2: hay también una RC en HEAD y una versión mayor en OTRA rama. No basta con
# escoger el tag más reciente de origin, ni con escribir 0.0.0 cuando describe falla.
version_fixture="$tmp/xcode-version"
mkdir -p "$version_fixture"
git init -q --bare "$version_fixture/remoto.git"
git init -q -b main "$version_fixture/fuente"
git -C "$version_fixture/fuente" config user.name 'Prueba Xcode Cloud'
git -C "$version_fixture/fuente" config user.email 'prueba@sytue.invalid'
printf 'release\n' >"$version_fixture/fuente/archivo"
git -C "$version_fixture/fuente" add archivo
git -C "$version_fixture/fuente" commit -qm release
git -C "$version_fixture/fuente" tag -a v3.12.2 -m release
for paso in 1 2 3 4 5; do
  printf '%s\n' "$paso" >>"$version_fixture/fuente/archivo"
  git -C "$version_fixture/fuente" commit -qam rama
done
git -C "$version_fixture/fuente" tag v3.13.0-rc.1
version_sha="$(git -C "$version_fixture/fuente" rev-parse HEAD)"
git -C "$version_fixture/fuente" checkout -qb ajena v3.12.2
printf 'ajena\n' >>"$version_fixture/fuente/archivo"
git -C "$version_fixture/fuente" commit -qam ajena
git -C "$version_fixture/fuente" tag v99.0.0
git -C "$version_fixture/fuente" remote add origin "$version_fixture/remoto.git"
git -C "$version_fixture/fuente" push -q origin main ajena --tags
git clone -q --depth=1 --no-tags --branch main "file://$version_fixture/remoto.git" "$version_fixture/rama"
mkdir -p "$version_fixture/rama/apps/fyne/xcode"
version_plist="$version_fixture/rama/apps/fyne/xcode/version.xcconfig"
version_probar() { # <tag> [versión explícita]
  CI_PRIMARY_REPOSITORY_PATH="$version_fixture/rama" CI_XCODE_SCHEME=calendar CI_BUILD_NUMBER=17 \
    CI_PRODUCT_PLATFORM=iOS CI_TAG="$1" SYTUE_APP_VERSION="${2:-}" \
    bash "$FYNE/xcode/ci_scripts/ci_post_clone.sh" --solo-version
}
version_probar '' >"$version_fixture/salida" 2>&1 ||
  mal "Xcode Cloud no resuelve la release de una rama shallow: $(cat "$version_fixture/salida")"
grep -qx 'MARKETING_VERSION = 3.12.2' "$version_plist" &&
  grep -qx 'CURRENT_PROJECT_VERSION = 17' "$version_plist" ||
  mal "la rama shallow no conserva la versión estable alcanzable y su número de build"
[ "$(git -C "$version_fixture/rama" rev-parse HEAD)" = "$version_sha" ] ||
  mal "resolver la versión de Xcode Cloud cambió el commit que se compila"
git -C "$version_fixture/rama" tag -d v3.12.2 >/dev/null
git -C "$version_fixture/rama" remote set-url origin "file://$version_fixture/no-existe.git"
version_probar v3.12.3 >"$version_fixture/salida" 2>&1 &&
  grep -qx 'MARKETING_VERSION = 3.12.3' "$version_plist" ||
  mal "un build de tag necesita fetch o no usa CI_TAG"
# Los flujos de Xcode Cloud arrancan con tienda-vX.Y.Z: la versión es la misma.
version_probar tienda-v3.12.4 >"$version_fixture/salida" 2>&1 &&
  grep -qx 'MARKETING_VERSION = 3.12.4' "$version_plist" ||
  mal "un build de tienda-vX.Y.Z no saca su versión del tag: $(cat "$version_fixture/salida")"
rm -f "$version_plist"
if version_probar '' >"$version_fixture/salida" 2>&1; then
  mal "Xcode Cloud acepta una rama sin versión ni acceso a los tags"
fi
[ ! -f "$version_plist" ] || mal "Xcode Cloud escribió una versión antes de resolverla"
version_probar '' 3.12.2 >"$version_fixture/salida" 2>&1 &&
  grep -qx 'MARKETING_VERSION = 3.12.2' "$version_plist" ||
  mal "Xcode Cloud no admite la versión explícita cuando Apple no permite fetch"
if version_probar v3.13.0-rc.1 >"$version_fixture/salida" 2>&1 ||
  version_probar tienda-v3.13.0-rc.1 >"$version_fixture/salida" 2>&1 ||
  version_probar '' 0.0.0 >"$version_fixture/salida" 2>&1; then
  mal "Xcode Cloud admite una RC o 0.0.0 como versión de App Store Connect"
fi
# Regresión real de Apple (90592): TRUE sin código archiva, pero no se puede subir.
# Se prueba EL post-clone contra plists temporales; ningún código de prueba llega a Apple.
if command -v python3 >/dev/null 2>&1; then
  python3 - "$FYNE/xcode" "$tmp" <<'PY' || mal "el post-clone no protege el par de exportación de Apple"
import os, pathlib, plistlib, runpy, shutil, subprocess, sys, tempfile
xcode = pathlib.Path(sys.argv[1])
guion = xcode / "ci_scripts/ci_post_clone.sh"
uso, codigo = "ITSAppUsesNonExemptEncryption", "ITSEncryptionExportComplianceCode"
fuentes = [xcode / ("Info-%s.plist" % p) for p in ("ios", "macos")]
# run_path no añade el directorio del guion para sus importaciones hermanas.
sys.path.insert(0, str(xcode.parent / "scripts"))
paises = runpy.run_path(str(xcode.parent / "scripts/appstore-publicar.py"))["PAISES"]
if "FRA" in paises:
    sys.exit("revisar la exención documental antes de habilitar Francia")
for ruta in fuentes:
    with ruta.open("rb") as fichero:
        info = plistlib.load(fichero)
    if info.get(uso) is not False or codigo in info:
        sys.exit("%s: cifrado estándar sin Francia debe declarar FALSE sin código" % ruta)

with tempfile.TemporaryDirectory(prefix="xcode-cifrado-", dir=sys.argv[2]) as carpeta:
    destino = pathlib.Path(carpeta) / "apps/fyne/xcode"
    destino.mkdir(parents=True)
    env = dict(os.environ, CI_PRIMARY_REPOSITORY_PATH=carpeta, CI_XCODE_SCHEME="calendar",
               SYTUE_APP_ENCRYPTION_EXPORT_CODE="")

    def restaurar():
        for fuente in fuentes:
            shutil.copyfile(fuente, destino / fuente.name)

    def ejecutar():
        return subprocess.run(["bash", str(guion), "--solo-cifrado"], env=env,
                              capture_output=True, text=True)

    restaurar()
    resultado = ejecutar()
    if resultado.returncode or any((destino / f.name).read_bytes() != f.read_bytes() for f in fuentes):
        sys.exit("sin código el post-clone no conserva la exención documental: " + resultado.stderr)

    # Verlo en rojo en LAS DOS plataformas: true sin código, tipos o pares inválidos.
    for fuente in fuentes:
        for cambios in ({uso: True}, {uso: True, codigo: ""},
                        {uso: True, codigo: "[]"}, {codigo: ""},
                        {codigo: "solo-prueba-no-enviar"}, {uso: "false"}, {uso: 0}):
            restaurar()
            ruta = destino / fuente.name
            info = plistlib.loads(ruta.read_bytes())
            info.update(cambios)
            ruta.write_bytes(plistlib.dumps(info))
            resultado = ejecutar()
            if resultado.returncode == 0 or "par TRUE + código no vacío" not in resultado.stderr:
                sys.exit("el post-clone no rechaza el par de exportación roto en " + fuente.name)
        restaurar()
        ruta = destino / fuente.name
        info = plistlib.loads(ruta.read_bytes())
        info.pop(uso)
        ruta.write_bytes(plistlib.dumps(info))
        resultado = ejecutar()
        if resultado.returncode == 0:
            sys.exit("el post-clone acepta metadatos de exportación ausentes en " + fuente.name)
    restaurar()
    env["SYTUE_APP_ENCRYPTION_EXPORT_CODE"] = " "
    resultado = ejecutar()
    if resultado.returncode == 0 or "SYTUE_APP_ENCRYPTION_EXPORT_CODE no contiene un código" not in resultado.stderr:
        sys.exit("el post-clone acepta un código vacío con espacios")
    # Solo formato y propagación en una carpeta temporal; no acredita aprobación.
    restaurar()
    env["SYTUE_APP_ENCRYPTION_EXPORT_CODE"] = "solo-prueba-no-enviar"
    resultado = ejecutar()
    if resultado.returncode:
        sys.exit("el post-clone no admite la ruta con código: " + resultado.stderr)
    for fuente in fuentes:
        original = plistlib.loads(fuente.read_bytes())
        generado = plistlib.loads((destino / fuente.name).read_bytes())
        esperado = dict(original, **{uso: True, codigo: env["SYTUE_APP_ENCRYPTION_EXPORT_CODE"]})
        if generado != esperado:
            sys.exit("el post-clone altera datos ajenos a la declaración en " + fuente.name)
print("ok - exportación: FALSE sin Francia, ruta con código y dieciséis pares inválidos rechazados")
PY
fi

# --- FYP-06: el ejecutable de Linux es PIE --------------------------------------
# Visto en rojo con el binario de prueba de siempre (sin -buildmode=pie).
pie="$tmp/bin-pie"
(cd "$tmp" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildmode=pie -o "$pie" .) >/dev/null 2>&1 ||
  mal "no se pudo compilar el binario PIE de prueba"
"$GUION" verificar-pie "$pie" >/dev/null 2>&1 || mal "el guardián rechaza un binario PIE"
"$GUION" verificar-pie "$amd64" >/dev/null 2>&1 && mal "el guardián da por PIE un binario que no lo es"

# --- FYP-05: UninstallString y QuietUninstallString entre comillas --------------
# La carpeta lleva espacio («Sytue One»): sin comillas, Windows prueba antes
# «…\Programs\Sytue.exe».
grep -qF "\"UninstallString\" '\"\\\$INSTDIR\\\\Desinstalar.exe\"'" "$GUION" ||
  mal "el UninstallString del instalador de Windows no va entre comillas"
grep -qF "\"QuietUninstallString\" '\"\\\$INSTDIR\\\\Desinstalar.exe\" /S'" "$GUION" ||
  mal "el QuietUninstallString del instalador de Windows no lleva la ruta entre comillas"

# --- FYP-07: lo que escribe `fyne build` al compilar no ensucia el árbol --------
git -C "$RAIZ" check-ignore -q apps/fyne/cmd/drive/fyne_metadata_init.go ||
  mal "fyne_metadata_init.go no está ignorado: todo binario saldría +dirty"

# --- FYP-04: los originales de los forks, a la vista de osv-scanner -------------
# Cada `replace X => github.com/thinkddr/…` tiene su X en apps/fyne/upstream/go.mod con la
# versión que pide apps/fyne/go.mod: si no, un aviso contra el original no se vería nunca.
# Visto en rojo quitando la línea de typesetting de upstream/go.mod.
while read -r original; do
  version="$(awk -v m="$original" '$1 == m && $2 ~ /^v/ {print $2; exit}' "$FYNE/go.mod")"
  grep -qE "^[[:space:]]*${original//./\\.}[[:space:]]+${version//./\\.}\$" "$FYNE/upstream/go.mod" ||
    mal "apps/fyne/upstream/go.mod no lleva $original $version, el original de un fork"
done < <(sed -n 's#^replace \([^ ]*\) => github.com/thinkddr/.*#\1#p' "$FYNE/go.mod")
grep -q 'apps/fyne/upstream/go.mod' "$RAIZ/.github/workflows/security.yml" ||
  mal "security.yml no le pasa a osv-scanner apps/fyne/upstream/go.mod"

[ "$fallos" -eq 0 ] && echo "ok - el guardián de arquitectura rechaza lo que debe y la cadena está fijada"
exit "$fallos"

