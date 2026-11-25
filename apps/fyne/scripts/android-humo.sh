#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# PRUEBA DE HUMO DE LAS APPS DE ANDROID EN UN EMULADOR: instala cada app, la abre, la
# deja correr y falla si se ha caído. Es lo que hace `android-emulador.yml`.
#
#   bash apps/fyne/scripts/android-humo.sh [--segundos 60] [--salida dir] <dir con los AAB>
#   bash apps/fyne/scripts/android-humo.sh revisar <logcat> <paquete>   # solo el juicio
#
# Los AAB son los de `empaquetar.sh android` (`sytue-<app>-android-sin-firmar.aab`), los
# mismos que firma release-mobile.yml: lo que se prueba es lo que se publica. Cada uno
# pasa a un APK universal con bundletool y una clave de depuración de usar y tirar, que
# solo vive en este job. Necesita el SDK con el emulador y su imagen
# (`accion.sh android-sdk android-emulador`, que deja ANDROID_HOME y
# ANDROID_EMULADOR_IMAGEN), bundletool y un JDK.
#
# CUÁNDO SE DA POR CAÍDA UNA APP, en su logcat (que se vacía antes de abrirla):
#   - `FATAL EXCEPTION` de su proceso (Java: la actividad o un hilo de la JNI);
#   - `panic:` o `fatal error:` (Go: un pánico o un error del runtime, que el driver
#     móvil de Fyne manda a logcat con la etiqueta «Fyne»);
#   - `Fatal signal` de su proceso (una caída nativa: SIGSEGV en el cgo, por ejemplo);
#   - o que pasados los segundos su proceso ya no esté, aunque no haya dejado rastro: el
#     último trozo de lo que escribe Go antes de morir puede no llegar a logcat.
# Un FATAL EXCEPTION de OTRO proceso (una app de Google de la imagen) se avisa y no
# cuenta: no es nuestro.
set -euo pipefail

# revisar <logcat> <paquete>: imprime las líneas que delatan una caída de <paquete> y
# sale con 1 si hay alguna; las de otros procesos van a stderr como aviso.
revisar() {
  local logcat="$1" paquete="$2"
  awk -v pq="$paquete" '
    function nuestra(l) { return index(l, pq) > 0 }
    # FATAL EXCEPTION: el proceso va en una de las líneas siguientes («Process: x, PID: n»).
    /FATAL EXCEPTION/ { pendiente = $0; quedan = 4; next }
    quedan > 0 {
      quedan--
      if ($0 ~ /Process: /) {
        if (nuestra($0)) { print pendiente; print; malas++ } else { print "aviso, de otro proceso: " $0 > "/dev/stderr" }
        quedan = 0; pendiente = ""
        next
      }
      if (quedan == 0 && pendiente != "") { print pendiente; malas++; pendiente = "" }
    }
    /(^|[^a-z])panic: |fatal error: / { print; malas++; next }
    # Una caída nativa: «Fatal signal 11 (SIGSEGV) … pid 1234 (eu.sytue.mail)».
    /Fatal signal [0-9]+/ {
      if (nuestra($0)) { print; malas++ } else { print "aviso, de otro proceso: " $0 > "/dev/stderr" }
    }
    END {
      if (pendiente != "") { print pendiente; malas++ }
      exit (malas > 0 ? 1 : 0)
    }
  ' "$logcat"
}

if [ "${1:-}" = revisar ]; then
  revisar "${2:?uso: $0 revisar <logcat> <paquete>}" "${3:?uso: $0 revisar <logcat> <paquete>}"
  exit $?
fi

segundos=60
salida="${RUNNER_TEMP:-/tmp}/android-humo"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --segundos) segundos="${2:?}"; shift 2 ;;
    --salida) salida="${2:?}"; shift 2 ;;
    -*) echo "::error::opción desconocida: $1" >&2; exit 2 ;;
    *) break ;;
  esac
done
dir="${1:?uso: $0 [--segundos N] [--salida dir] <dir con los AAB>}"
[[ "$segundos" =~ ^[0-9]+$ ]] && [ "$segundos" -ge 10 ] || { echo "::error::--segundos tiene que ser un número de 10 o más" >&2; exit 2; }
: "${ANDROID_HOME:?falta ANDROID_HOME: accion.sh android-sdk android-emulador}"
: "${ANDROID_EMULADOR_IMAGEN:?falta ANDROID_EMULADOR_IMAGEN: accion.sh android-emulador}"
for h in adb emulator bundletool keytool unzip; do
  command -v "$h" >/dev/null 2>&1 || { echo "::error::falta $h" >&2; exit 1; }
done
# Con `while read` y no mapfile: corre en el Mac del runner, con el bash 3.2 de macOS.
aabs=()
while IFS= read -r aab; do aabs+=("$aab"); done < <(find "$dir" -maxdepth 1 -name 'sytue-*-android-sin-firmar.aab' | sort)
[ "${#aabs[@]}" -gt 0 ] || { echo "::error::no hay ningún AAB en $dir" >&2; exit 1; }
mkdir -p "$salida"
trabajo="$(mktemp -d)"
avd=sytue-humo
export ANDROID_AVD_HOME="$trabajo/avd"
mkdir -p "$ANDROID_AVD_HOME"

emulador_pid=""
terminar() {
  if [ -n "$emulador_pid" ]; then
    adb emu kill >/dev/null 2>&1 || true
    sleep 5
    kill "$emulador_pid" 2>/dev/null || true
    wait "$emulador_pid" 2>/dev/null || true
  fi
  rm -rf "$trabajo"
}
trap terminar EXIT

# La clave de depuración: de este job y de nadie más. Un APK sin firmar no se instala.
keytool -genkeypair -keystore "$trabajo/humo.p12" -storetype PKCS12 -storepass android \
  -alias humo -keypass android -keyalg RSA -keysize 2048 -validity 1 \
  -dname "CN=Sytue prueba de humo" >/dev/null 2>&1

# El emulador, sin ventana ni sonido y sin instantáneas: cada ronda empieza de cero.
echo "no" | "$ANDROID_HOME/cmdline-tools/latest/bin/avdmanager" create avd -n "$avd" \
  -k "$ANDROID_EMULADOR_IMAGEN" --device pixel_7 --force >/dev/null
emulator -avd "$avd" -no-window -no-audio -no-boot-anim -no-snapshot -wipe-data \
  -gpu swiftshader_indirect -memory 4096 >"$salida/emulador.log" 2>&1 &
emulador_pid=$!
adb wait-for-device
arranque=0
until [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ]; do
  arranque=$((arranque + 5))
  [ "$arranque" -lt 600 ] || { echo "::error::el emulador no arrancó en 10 minutos (ver emulador.log)" >&2; exit 1; }
  kill -0 "$emulador_pid" 2>/dev/null || { echo "::error::el emulador se cerró al arrancar (ver emulador.log)" >&2; exit 1; }
  sleep 5
done
echo "emulador listo en ${arranque} s ($ANDROID_EMULADOR_IMAGEN)"
# Sin animaciones y con la pantalla despierta: lo que se mide es la app.
for a in window_animation_scale transition_animation_scale animator_duration_scale; do
  adb shell settings put global "$a" 0 || true
done
adb shell svc power stayon true || true
adb shell input keyevent 82 || true

fallidas=()
resumen=()
for aab in "${aabs[@]}"; do
  app="$(basename "$aab" -android-sin-firmar.aab)"
  app="${app#sytue-}"
  paquete="$(bundletool dump manifest --bundle "$aab" | sed -n 's/.*package="\([^"]*\)".*/\1/p' | head -1)"
  [ -n "$paquete" ] || { echo "::error::$aab no declara paquete" >&2; fallidas+=("$app"); continue; }
  bundletool build-apks --bundle "$aab" --output "$trabajo/$app.apks" --mode=universal --overwrite \
    --ks "$trabajo/humo.p12" --ks-pass pass:android --ks-key-alias humo --key-pass pass:android >/dev/null
  unzip -o -q "$trabajo/$app.apks" universal.apk -d "$trabajo/$app"
  echo "── $app ($paquete)"
  if ! adb install -r -g "$trabajo/$app/universal.apk" >"$salida/$app-instalar.txt" 2>&1; then
    echo "::error::$app no se instala: $(tail -1 "$salida/$app-instalar.txt")"
    fallidas+=("$app"); resumen+=("| $app | no se instala |"); continue
  fi
  adb logcat -c || true
  adb shell monkey -p "$paquete" -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1 || true
  sleep "$segundos"
  vivo="$(adb shell pidof "$paquete" 2>/dev/null | tr -d '\r' || true)"
  adb logcat -d -v threadtime >"$salida/$app-logcat.txt" 2>&1 || true
  motivo=""
  if ! revisar "$salida/$app-logcat.txt" "$paquete" >"$salida/$app-caidas.txt"; then
    motivo="se cayó: $(head -1 "$salida/$app-caidas.txt")"
  elif [ -z "$vivo" ]; then
    motivo="a los $segundos s su proceso ya no estaba"
  fi
  adb shell am force-stop "$paquete" >/dev/null 2>&1 || true
  adb uninstall "$paquete" >/dev/null 2>&1 || true
  if [ -n "$motivo" ]; then
    echo "::error::$app $motivo (logcat en $app-logcat.txt)"
    fallidas+=("$app"); resumen+=("| $app | $motivo |")
  else
    echo "  ✓ abierta $segundos s sin caerse"
    resumen+=("| $app | bien |")
  fi
done

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### Prueba de humo en el emulador ($ANDROID_EMULADOR_IMAGEN, $segundos s por app)"
    echo
    echo "| App | Resultado |"
    echo "| --- | --- |"
    printf '%s\n' ${resumen[@]+"${resumen[@]}"}
  } >>"$GITHUB_STEP_SUMMARY"
fi
if [ "${#fallidas[@]}" -gt 0 ]; then
  echo "::error::se caen en el emulador: ${fallidas[*]}" >&2
  exit 1
fi
echo "las ${#aabs[@]} apps abren y siguen vivas a los $segundos s"
