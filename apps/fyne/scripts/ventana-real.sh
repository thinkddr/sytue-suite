#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# ventana-real.sh — abre las apps de Fyne en una ventana DE VERDAD (GL por Mesa, no el
# pintor de software de los tests), las recorre y guarda capturas.
#
#   apps/fyne/scripts/ventana-real.sh [--x11|--wayland] [--sin-sesion] [--salida DIR]
#                                     [--bin DIR] [--color COLOR] [--forma FORMA]
#                                     [--escribe] [--alto N] [--menu-cuenta]
#                                     [--idioma-y Y] [--pasos-en MODO] [app…]
#   apps/fyne/scripts/ventana-real.sh --aros [--salida DIR]
#
# Sin apps, las quince, Mail primero: abre el login al arrancar sin sesión, y el Hub no.
# `--x11` (por omisión) usa Xvfb; `--wayland`, sway sin pantalla. El binario de Linux trae
# los dos motores de GLFW (las etiquetas `nowayland,novulkan` no llegan a GLFW), así que
# bajo sway la app habla Wayland nativo. Con `--bin` usa binarios ya compilados; sin él
# los compila con la CLI de Fyne, como la release (sin los metadatos de FyneApp.toml la
# app no guarda ajustes ni sesión: ver empaquetar.sh).
#
# POR APP: a 1280 hace el recorrido de `pasos` (lista, abrir un elemento, un diálogo, un
# menú; cada `foto` es una captura) y después la pantalla de inicio a 1280 y a 360, en
# claro, en oscuro y en traslúcido. Con `--sin-sesion` no entra: es lo que ve quien aún
# no ha iniciado sesión. Con `--color cobalto` las capturas salen en ese color de la
# suite, que es un eje aparte del tema (sin él, el de la casa); con `--forma carril`, en
# esa forma (sin él, Marco, que es la de la casa).
#
# NO ESCRIBE NADA. Las apps corren con `SYTUE_SOLO_LECTURA=1`: cortan antes de la red toda
# petición que no sea leer (`apps/fyne/sesion/conexion.go`). El recorrido va con la cuenta
# de auditoría CONTRA PRODUCCIÓN y pulsa en coordenadas fijas: el 2026-10-05, con la forma
# Marco recién puesta, una coordenada desplazada cayó en «Nueva nota», y los tres editores
# creaban un documento de plantilla en cada tirada, por diseño. Hubo que borrarlos a mano.
# Ahora un clic puede acabar en cualquier botón y no pasa nada. Los pasos que NECESITAN
# escribir —abrir un editor desde su plantilla— van marcados con `!` y solo corren con
# `--escribe`, que además quita el solo lectura: quien lo pasa deja un documento por
# editor en esa cuenta y le toca borrarlos (el guion lo recuerda al acabar). Lo que se
# corta queda en el registro de cada app (`<app>.log`, «escritura cortada»).
#
# LA EXCEPCIÓN, EL DESBLOQUEO DEL CIFRADO. Con `SYTUE_PERMITIR_DESBLOQUEO=1` (lo pone solo
# `ventana-real.yml`) las apps escriben además, y nada más, lo de una lista cerrada de
# rutas: dar de alta su dispositivo de Matrix, verificarlo con las firmas cruzadas de la
# cuenta, guardar el envoltorio de su clave y cerrar al final (`escriturasDelDesbloqueo`).
#
# Por eso mismo LA PRESENCIA NO ES DE FIAR en estas capturas: el latido que mantiene a la
# cuenta «en línea» es un `PUT /presence` y no sale. La app solo enseña lo que el Hub ya
# sepa de esa cuenta, y pasado su plazo sin latido (45 s por defecto) el Hub la da por
# desconectada, que la app no adopta. El menú de cuenta sale entonces sin marca en ningún
# estado y el avatar sin punto;
# si salen («En línea», punto verde) es que OTRA sesión de la misma cuenta está latiendo.
#
# `--alto N` cambia el alto de la ventana ancha (744 por omisión; 800 es el de las medidas
# de la web). Con `--menu-cuenta`, en cada tamaño y cada modo se fotografía además el menú
# de cuenta abierto (`…-cuenta.png`) y, si `--idioma-y Y` dice a qué altura cae su fila, el
# desplegable de idioma, que es el más largo de la suite (`…-idioma.png`). El recorrido de
# `pasos` va en claro; `--pasos-en oscuro` (o `traslucido`) lo hace en ese modo.
#
# `--aros` no abre ninguna app: abre el muestrario de aros de foco (`cmd/demo`), a ×1 y a
# ×2 y en claro y en oscuro, y MIDE cada aro sobre los píxeles de la ventana —grosor,
# opacidad, posición y contraste—. Sale con error si alguno no es el de la web (2 px del
# token, pegados a la caja). Existe porque el pintor de GL y el de software no trazan
# igual: el aro abierto salía bien en los tests y un píxel más adentro en la ventana, y
# nadie lo veía si no miraba. No pide sesión ni toca producción.
#
# EL LOGIN. La primera app sin sesión abre Sytue ID con `xdg-open`; aquí `xdg-open` es
# falso y deja la URL en `$SALIDA/login-url`. La contraseña NO está en esta máquina: la
# completa `test/e2e/ventana-real-login.mjs` donde viven las credenciales de las sondas (el
# piloto), y su salida —la URL de vuelta— se deja en `$SALIDA/login-vuelta`. Con
# `VENTANA_LOGIN="orden"` el guion la ejecuta él con la URL como argumento. La sesión se
# guarda en un gnome-keyring propio de esta tirada, así que las otras doce ya entran
# solas, y se borra al salir. Desde la ADR 0084 ese inicio de sesión trae también la clave
# de desbloqueo del cifrado: la página de Sytue ID la deriva en el navegador y el RELEVO
# la devuelve cifrada para la app en la vuelta (`sytue_unlock`), así que el guion de
# Playwright espera a que el relevo navegue a la vuelta y no corta antes.
#
# EN EL CI lo corre `.github/workflows/ventana-real.yml` (a mano, `workflow_dispatch`) en
# un runner del frontal con Xvfb y el GL de Mesa: siembra antes la cuenta con
# `test/e2e/sembrar-auditoria.mjs` —también los hilos pospuesto, silenciado y vuelto que
# Mail necesita enseñar, porque en solo lectura no se pueden crear desde aquí—, pasa
# `VENTANA_LOGIN` con `ventana-real-login.mjs` y sube las capturas como artefacto.
#
# EL CIFRADO. Cada tirada estrena dispositivo (llavero y carpeta de usar y tirar) y las
# apps de cliente cifrado —Mail, Hub y ERP, medido el 2026-11-17 en ventana-real
# 43985202886— arrancan en «Cifrado de extremo a extremo · Configurar este dispositivo».
# Con `VENTANA_LOGIN` y en X11, antes de las capturas de cada app de `VENTANA_CIFRADO`
# (por omisión «mail hub erp») el guion pulsa ese botón si lo ve (mira dos píxeles del
# botón del diálogo), y la app abre el SSO de Matrix en el navegador: `xdg-open` lo deja en
# `$SALIDA/matrix-url` y `ventana-real-login.mjs --matrix` lo completa con la sesión de
# Sytue ID que dejó el inicio de sesión (`VENTANA_NAVEGADOR`), sin teclear nada. Después
# la app desbloquea sola con la clave de la sesión. Lo que se ve queda en
# `<app>-cifradoN.png`, y lo que se paró en el navegador, en `login-<código>.png`. Al
# salir, `herramientas/dispositivos` cierra en el homeserver los dispositivos de Matrix
# que dio de alta la tirada (los lee de su llavero): la cuenta no los acumula.
# Medido el 2026-11-17 (ventana-real 44147994834): el inicio de sesión y el alta del
# dispositivo se completan en Mail, Hub y ERP (tres dispositivos, cerrados al acabar),
# pero producción (v3.20.4) aún no sirve el relevo de la ADR 0084: la vuelta llega sin
# la clave de desbloqueo y el cifrado no se abre hasta que se despliegue.
#
# LAS VUELTAS AL LOOPBACK. Cuando Sytue ID o Synapse devuelven con una redirección, el
# navegador del guion de Playwright la sigue y se la lleva él a la app (Playwright no
# enruta los saltos de una redirección); cuando la abre el guion del relevo, la ruta la
# recoge y la entrega este guion. Por eso `entregar` da por buena una conexión rechazada:
# la app ya cerró su escucha porque ya la tiene.
#
# Nada se queda encendido: el `trap` mata el servidor gráfico, el llavero y las apps.
set -euo pipefail

FYNE="$(cd "$(dirname "$0")/.." && pwd)"
# Las opciones que no son listas viajan por el entorno: el guion se relanza a sí mismo
# dentro de su sesión de D-Bus (más abajo) y solo repite el modo, la salida y las apps.
modo=x11 sesion=si escribe="${VENTANA_ESCRIBE:-no}" ALTO="${VENTANA_ALTO:-744}" cuenta="${VENTANA_CUENTA:-no}" idiomaY="${VENTANA_IDIOMA_Y:-}"
pasosEn="${VENTANA_PASOS_EN:-claro}" aros=no
SALIDA="${TMPDIR:-/tmp}/ventana-real" BIN="" COLOR="${VENTANA_COLOR:-}" FORMA="${VENTANA_FORMA:-}"
apps=()
while [ $# -gt 0 ]; do
  case "$1" in
  --x11) modo=x11 ;;
  --wayland) modo=wayland ;;
  --sin-sesion) sesion=no ;;
  --salida) SALIDA="$2"; shift ;;
  --bin) BIN="$2"; shift ;;
  --color) COLOR="$2"; shift ;;
  --forma) FORMA="$2"; shift ;;
  --escribe) escribe=si ;;
  --alto) ALTO="$2"; shift ;;
  --menu-cuenta) cuenta=si ;;
  --idioma-y) idiomaY="$2"; shift ;;
  --pasos-en) pasosEn="$2"; shift ;;
  --aros) aros=si ;;
  -*) echo "opción desconocida: $1" >&2; exit 2 ;;
  *) apps+=("$1") ;;
  esac
  shift
done
if [ "$aros" = si ]; then
  command -v Xvfb >/dev/null || { echo "falta Xvfb" >&2; exit 1; }
  mkdir -p "$SALIDA/aros"
  bash "$FYNE/scripts/preparar-livekit.sh"
  (cd "$FYNE" && go build -tags goolm -o "$SALIDA/aros/demo" ./cmd/demo)
  n=90
  while [ -e "/tmp/.X$n-lock" ]; do n=$((n + 1)); done
  # 1200 de alto: a ×2 el muestrario mide 720 × 1120 y tiene que caber entero.
  Xvfb ":$n" -screen 0 1600x1200x24 +extension GLX -nolisten tcp >"$SALIDA/aros/servidor.log" 2>&1 &
  servidor=$!
  trap 'kill "$servidor" 2>/dev/null || true' EXIT
  export DISPLAY=":$n"
  for _ in $(seq 50); do xdpyinfo >/dev/null 2>&1 && break; sleep 0.2; done
  mal=0
  : >"$SALIDA/aros/medidas.txt"
  for escala in 1 2; do
    for t in claro oscuro; do
      echo "== aros de foco a ×$escala, en $t" | tee -a "$SALIDA/aros/medidas.txt"
      FYNE_SCALE="$escala" SYTUE_TEMA="$t" SYTUE_AROS="$SALIDA/aros/aros-x$escala-$t.png" \
        "$SALIDA/aros/demo" 2>>"$SALIDA/aros/demo.log" | tee -a "$SALIDA/aros/medidas.txt" || mal=1
    done
  done
  [ "$mal" = 0 ] || echo "aros: hay aros que no son los de la web (ver $SALIDA/aros/medidas.txt)" >&2
  exit "$mal"
fi
[ ${#apps[@]} -gt 0 ] || apps=(mail hub drive calendar contacts notes photos erp docus calcus diapos certipdf rooms cast bases)
# Solo Mail abre el login al arrancar sin sesión: si no va la primera, las demás corren sin
# entrar y todas las capturas son la pantalla de «No has iniciado sesión».
if [ "$sesion" = si ] && [ "${apps[0]}" != mail ]; then
  resto=()
  for a in "${apps[@]}"; do [ "$a" = mail ] || resto+=("$a"); done
  apps=(mail "${resto[@]}")
fi
mkdir -p "$SALIDA/$modo"
for h in xdotool xwd convert dbus-run-session gnome-keyring-daemon; do
  command -v "$h" >/dev/null || { echo "falta $h (x11-apps, xdotool, imagemagick, dbus, gnome-keyring)" >&2; exit 1; }
done

# --- Compilar ------------------------------------------------------------------
if [ -z "$BIN" ]; then
  BIN="$SALIDA/bin"
  mkdir -p "$BIN"
  CLI="$(command -v fyne || echo "$HOME/go/bin/fyne")"
  for a in "${apps[@]}"; do
    (cd "$FYNE/cmd/$a" && GOPRIVATE='github.com/thinkddr/*' "$CLI" build --tags goolm --target linux -o "$BIN/$a")
  done
fi

# El cierre de los dispositivos de Matrix de la tirada, compilado AQUÍ: más abajo HOME es
# un directorio vacío, sin la caché de Go.
if [ "$sesion" = si ] && [ ! -x "$BIN/dispositivos" ]; then
  (cd "$FYNE" && GOPRIVATE='github.com/thinkddr/*' go build -o "$BIN/dispositivos" ./herramientas/dispositivos) ||
    echo "OJO: sin herramientas/dispositivos, los dispositivos de Matrix de la tirada se quedan abiertos" >&2
fi

# El llavero: Secret Service en una sesión de D-Bus propia, desbloqueado con una clave
# de usar y tirar. Es donde `apps/nucleo/auth` guarda la sesión que comparten las quince.
if [ -z "${VENTANA_DBUS:-}" ]; then
  export VENTANA_DBUS=1 VENTANA_COLOR="$COLOR" VENTANA_FORMA="$FORMA" \
    VENTANA_ESCRIBE="$escribe" VENTANA_ALTO="$ALTO" VENTANA_CUENTA="$cuenta" VENTANA_IDIOMA_Y="$idiomaY" \
    VENTANA_PASOS_EN="$pasosEn"
  sin=()
  [ "$sesion" = si ] || sin=(--sin-sesion)
  exec dbus-run-session -- "$0" "--$modo" "${sin[@]}" --salida "$SALIDA" --bin "$BIN" "${apps[@]}"
fi

# --- Un entorno propio: config, llavero y navegador falsos -------------------------
# Todo en un directorio de la tirada: ni la sesión ni el tema tocan los de quien lanza.
ENTORNO="$(mktemp -d)"
export HOME="$ENTORNO" XDG_CONFIG_HOME="$ENTORNO/config" XDG_DATA_HOME="$ENTORNO/data" \
  XDG_RUNTIME_DIR="$ENTORNO/run" PATH="$ENTORNO/bin:$PATH"
mkdir -p "$XDG_CONFIG_HOME/sytue" "$XDG_DATA_HOME" "$ENTORNO/bin"
chmod 700 "$XDG_RUNTIME_DIR" 2>/dev/null || mkdir -m 700 "$XDG_RUNTIME_DIR"
rm -f "$SALIDA/login-url" "$SALIDA/login-vuelta" "$SALIDA/matrix-url" "$SALIDA/abiertas.txt"
# Lo que una app abre en el navegador. Los inicios de sesión se completan aparte y en
# `abiertas.txt` solo se nombran: su URL lleva el state y la cuenta de Matrix.
cat >"$ENTORNO/bin/xdg-open" <<EOF
#!/bin/sh
case "\$1" in
*id.*authorize*|*/authorize\?*) printf '%s\n' "\$1" >"$SALIDA/login-url"; echo "(inicio de sesión en Sytue ID)" >>"$SALIDA/abiertas.txt" ;;
*/_matrix/client/*/login/sso/redirect*) printf '%s\n' "\$1" >"$SALIDA/matrix-url"; echo "(SSO de Matrix)" >>"$SALIDA/abiertas.txt" ;;
*) printf '%s\n' "\$1" >>"$SALIDA/abiertas.txt" ;;
esac
EOF
chmod +x "$ENTORNO/bin/xdg-open"
# Para ventana-real-login.mjs: dónde deja la sesión del navegador el inicio de sesión
# (la reutiliza el SSO de Matrix) y dónde fotografía lo que se pare en el navegador.
export VENTANA_NAVEGADOR="$ENTORNO/navegador.json" VENTANA_DIAGNOSTICO="$SALIDA/$modo"

pids=()
limpiar() {
  for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  # Con las apps ya paradas y el llavero aún vivo: lo que la tirada dio de alta en Matrix
  # se cierra con su propio token, pase lo que pase antes (también si algo falló).
  if [ "$sesion" = si ] && [ -x "$BIN/dispositivos" ]; then
    "$BIN/dispositivos" 2>&1 | tee -a "$SALIDA/$modo/resumen.txt" || true
  fi
  rm -rf "$ENTORNO"
}
trap limpiar EXIT
trap 'exit 143' INT TERM # que también limpie si lo matan

printf x | gnome-keyring-daemon --unlock --components=secrets >/dev/null

# --- El servidor gráfico ---------------------------------------------------------
if [ "$modo" = x11 ]; then
  n=90
  while [ -e "/tmp/.X$n-lock" ]; do n=$((n + 1)); done
  Xvfb ":$n" -screen 0 1600x1000x24 +extension GLX -nolisten tcp >"$SALIDA/$modo/servidor.log" 2>&1 &
  pids+=($!)
  export DISPLAY=":$n"
  for _ in $(seq 50); do xdpyinfo >/dev/null 2>&1 && break; sleep 0.2; done
else
  for h in sway grim wtype; do command -v "$h" >/dev/null || { echo "falta $h" >&2; exit 1; }; done
  # Sin GPU: el pintor de pixman del compositor, y Mesa (llvmpipe) para el GL de la app.
  cat >"$ENTORNO/sway.conf" <<'EOF'
output HEADLESS-1 resolution 1600x1000 bg #808080 solid_color
xwayland disable
default_border none
for_window [app_id=".*"] floating enable, move position 0 0
EOF
  # sway se niega a correr como root («Unable to drop root»): en un contenedor corre como
  # `nobody`, dueño del XDG_RUNTIME_DIR, y root sigue pudiendo hablar con sus enchufes.
  como=()
  if [ "$(id -u)" = 0 ]; then
    chmod o+x "$ENTORNO" && chown -R nobody "$XDG_RUNTIME_DIR" "$ENTORNO/sway.conf"
    como=(setpriv --reuid=nobody --regid=nogroup --clear-groups --reset-env
      env XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" HOME=/tmp)
  fi
  "${como[@]}" env WLR_BACKENDS=headless WLR_RENDERER=pixman WLR_LIBINPUT_NO_DEVICES=1 \
    sway -c "$ENTORNO/sway.conf" >"$SALIDA/$modo/servidor.log" 2>&1 &
  pids+=($!)
  for _ in $(seq 50); do ls "$XDG_RUNTIME_DIR"/wayland-* "$XDG_RUNTIME_DIR"/sway-ipc.* >/dev/null 2>&1 && break; sleep 0.2; done
  export WAYLAND_DISPLAY SWAYSOCK
  for f in "$XDG_RUNTIME_DIR"/wayland-*; do
    case "$f" in *.lock) ;; *) WAYLAND_DISPLAY="$(basename "$f")"; break ;; esac
  done
  for f in "$XDG_RUNTIME_DIR"/sway-ipc.*; do SWAYSOCK="$f"; break; done
fi
DISPLAY=${DISPLAY:-} glxinfo -B 2>/dev/null | grep -E 'renderer string|version string' >"$SALIDA/$modo/gl.txt" || true

# --- Una app ---------------------------------------------------------------------------
tema() { printf '{"tema":"%s","color":"%s","forma":"%s"}\n' "$1" "$COLOR" "$FORMA" >"$XDG_CONFIG_HOME/sytue/ajustes.json"; sleep 3; } # la vigila cada 2 s
# Una ventana es su id de X11 o, en sway, el pid de la app: sway la coloca en 0,0, así que
# las coordenadas de los pasos valen igual en los dos.
ventana() { # <pid> → id
  if [ "$modo" = x11 ]; then
    xdotool search --onlyvisible --pid "$1" 2>/dev/null | head -1 || true
  else
    grep -q "\"pid\": $1," <<<"$(swaymsg -t get_tree)" && echo "$1" || true
  fi
}
capturar() { # <ventana> <fichero>
  if [ "$modo" = x11 ]; then
    xwd -silent -id "$1" | convert xwd:- "$2"
  else
    local g
    g="$(swaymsg -t get_tree | python3 -c '
import json, sys
def nodos(n):
    yield n
    for h in n.get("nodes", []) + n.get("floating_nodes", []):
        yield from nodos(h)
for n in nodos(json.load(sys.stdin)):
    if n.get("pid") == int(sys.argv[1]):
        r = n["rect"]; print("%d,%d %dx%d" % (r["x"], r["y"], r["width"], r["height"]))' "$1")"
    grim -g "$g" "$2"
  fi
}
medir() { # <ventana> <ancho> <alto>
  if [ "$modo" = x11 ]; then
    xdotool windowsize "$1" "$2" "$3"
    # Sin gestor de ventanas el cambio a veces no llega (el Hub se quedó en 360 al volver
    # a 1280 y su recorrido entero salió estrecho): se mira y se repite una vez.
    sleep 1
    grep -q "Geometry: ${2}x${3}" <<<"$(xdotool getwindowgeometry "$1")" || xdotool windowsize "$1" "$2" "$3"
  else
    swaymsg "[pid=$1] resize set $2 $3" >/dev/null
  fi
  sleep 3
}
# En sway sin pantalla no hay puntero: el backend `headless` no crea dispositivos y
# `swaymsg seat … cursor` no llega a la app (medido: las cinco fotos de Mail salían
# iguales). El teclado sí, con `wtype`. Así que en Wayland los pasos de ratón no se
# hacen y se dice, y lo que se prueba es teclado, tema y tamaños; el recorrido con ratón
# es el de X11.
mover() { # <ventana> <x> <y>
  if [ "$modo" = x11 ]; then
    xdotool mousemove --window "$1" "$2" "$3"
  fi
}
clic() { # <ventana> <x> <y>
  if [ "$modo" = x11 ]; then
    mover "$@"
    xdotool click 1
  else
    echo "sway: sin puntero, el clic en $2,$3 no se hace" >>"$SALIDA/$modo/resumen.txt"
  fi
}
# En sway, los ATAJOS por wtype no son fiables: wtype crea un teclado virtual con su propio
# mapa de códigos y GLFW los traduce con la tabla fija de evdev (_glfw.wl.keycodes). El texto
# llega bien (va por keysym), pero ctrl+k puede llegar como otra tecla —en Mail, lo que se
# escribe después cae en el atajo «c» y abre el redactor—. Es de la herramienta, no de la
# app: con un teclado de verdad no pasa. Los atajos se comprueban en X11.
tecla() { # <ventana> <tecla al estilo de xdotool: ctrl+k, Escape, Return>
  if [ "$modo" = x11 ]; then
    xdotool key --window "$1" "$2"
  else
    local mods=() k="$2"
    while [[ "$k" == *+* ]]; do mods+=(-M "${k%%+*}"); k="${k#*+}"; done
    local sueltas=()
    for ((i = ${#mods[@]} - 1; i > 0; i -= 2)); do sueltas+=(-m "${mods[i]}"); done
    wtype "${mods[@]}" -k "$k" "${sueltas[@]}"
  fi
}
escribir() { # <ventana> <texto>
  if [ "$modo" = x11 ]; then xdotool type --window "$1" "$2"; else wtype "$2"; fi
}
# pasos <app> — el recorrido, a 1280 × 744, en líneas `orden argumentos`. Las coordenadas
# son de la ventana y salen de las capturas de la tirada anterior; si una pantalla se
# mueve, se ve en la foto siguiente, que sale mal. `app@forma` es un paso de esa forma
# nada más: en la otra, lo que hay en esas coordenadas es otra cosa.
#
# Lo de Mail tras abrir el primer correo (que es el hilo VUELTO de Pospuestos, arriba de
# Recibidos con su reloj): el menú del reloj, la carpeta «Pospuestos» y su hilo con la
# franja «Pospuesto hasta…», y el hilo silenciado —buscado, porque silenciar lo archiva—
# con la suya; en los dos, «Más acciones». Esos tres hilos los deja la siembra. Lo de la
# paleta (⌘K): en el Hub, la búsqueda en Sytue One desde dos letras, las flechas e Intro
# (lo que abre queda en `abiertas.txt`); en el ERP, sus capas `erp:vistas`, `erp` y
# `erp:registros`. Coordenadas medidas con el driver de Fyne en Marco a 1280 × 744 (la
# fila 450,233; el buscador 373,128; «Pospuestos» y «Recibidos» del lateral 130,293 y
# 130,253; el reloj y «Más acciones» del lector 904,177 y 976,177).
pasos() {
  awk -v app="$1" -v escribe="$escribe" -v forma="${FORMA:-marco}" '$1 == app || $1 == "*" || $1 == app "@" forma || (escribe == "si" && $1 == app "!") { $1 = ""; print substr($0, 2) }' <<'FIN'
* foto inicio
mail clic 450 233
mail espera 3
mail foto abrir-correo
mail clic 904 177
mail espera 1
mail foto posponer-menu
mail tecla Escape
mail clic 130 293
mail espera 4
mail foto pospuestos
mail clic 450 233
mail espera 4
mail foto pospuesto-franja
mail clic 976 177
mail espera 1
mail foto pospuesto-mas-acciones
mail tecla Escape
mail clic 373 128
mail escribir silenciado
mail espera 4
mail foto silenciado-buscado
mail clic 450 233
mail espera 4
mail foto silenciado-franja
mail clic 976 177
mail espera 1
mail foto silenciado-mas-acciones
mail tecla Escape
mail clic 130 253
mail espera 3
mail! clic 130 72
mail! foto redactar
mail! tecla Escape
calendar clic 920 266
calendar foto evento
calendar tecla Escape
calendar clic 134 74
calendar foto crear-evento
calendar tecla Escape
contacts clic 380 222
contacts foto ficha
contacts clic 134 74
contacts foto nuevo-contacto
contacts tecla Escape
drive clic 1230 403
drive tecla Down
drive foto menu-fila
drive tecla Escape
drive clic 1229 79
drive foto cuadricula
drive clic 1200 79
drive doble 390 152
drive espera 3
drive foto carpeta
drive clic 1040 79
drive foto menu-nuevo
drive tecla Escape
erp clic 232 468
erp espera 3
erp foto tickets
erp clic 1120 92
erp foto ficha
erp tecla Escape
erp clic 36 98
erp foto cajon
erp tecla Escape
erp clic 36 206
erp espera 3
erp foto tareas
erp mover 36 152
erp espera 1
erp foto titulo
notes clic 340 114
notes foto nota
notes clic 1134 76
notes foto ajustes
notes tecla Escape
photos clic 230 84
photos foto albumes
rooms clic 74 304
rooms espera 3
rooms foto canal
rooms mover 600 586
rooms foto encima-de-un-mensaje
rooms clic 100 468
rooms foto explorar-canales
rooms tecla Escape
calcus! clic 120 406
calcus! espera 6
calcus! foto hoja
docus! clic 120 400
docus! espera 6
docus! foto documento
diapos! clic 120 400
diapos! espera 6
diapos! foto presentacion
certipdf clic 120 506
certipdf espera 6
certipdf foto pdf
bases clic 640 252
bases espera 3
bases foto base-abierta
hub@marco clic 605 172
hub@marco foto mas
hub@marco tecla Down
hub@marco foto mas-flecha
hub@marco tecla Escape
hub@marco foto mas-escape
hub rueda 640 400
hub foto accesos
mail clic 1243 22
mail clic 1221 499
mail espera 4
mail foto forma-carril
mail clic 1179 499
mail espera 4
mail foto forma-marco
mail clic 1111 499
mail espera 4
mail tecla Escape
* clic 1243 22
* foto menu-cuenta
* tecla Down
* foto menu-cuenta-flecha
* tecla Escape
* tecla ctrl+k
* escribir acta
* espera 3
* foto buscar
* tecla Escape
hub tecla ctrl+k
hub espera 2
hub foto paleta
hub escribir au
hub espera 1
hub foto paleta-buscando
hub espera 4
hub foto paleta-dos-letras
hub tecla Down
hub tecla Down
hub foto paleta-flechas
hub tecla Return
hub espera 2
hub foto paleta-saltar
hub tecla Escape
erp tecla ctrl+k
erp espera 2
erp foto paleta-capas
erp tecla Down
erp tecla Down
erp foto paleta-capas-flechas
erp escribir fa
erp espera 1
erp foto paleta-buscando
erp espera 4
erp foto paleta-registros
erp tecla Up
erp foto paleta-registros-flecha
erp tecla Escape
FIN
}
recorrer() { # <app> <ventana>
  local orden x y n=0
  while read -r orden x y; do
    case "$orden" in
    clic) clic "$2" "$x" "$y"; sleep 2 ;;
    doble) clic "$2" "$x" "$y"; clic "$2" "$x" "$y"; sleep 2 ;;
    mover) mover "$2" "$x" "$y"; sleep 1 ;;
    # rueda <x> <y>: cuarenta muescas hacia abajo con el puntero ahí (solo X11).
    rueda) mover "$2" "$x" "$y"; [ "$modo" = x11 ] && xdotool click --repeat 40 --delay 30 5; sleep 2 ;;
    tecla) tecla "$2" "$x"; sleep 2 ;;
    escribir) escribir "$2" "$x${y:+ $y}"; sleep 1 ;;
    espera) sleep "$x" ;;
    foto) n=$((n + 1)); capturar "$2" "$SALIDA/$modo/$1-paso$n-$x.png" ;;
    esac
  done < <(pasos "$1")
}
entrar() { # espera la URL de login, la completa y vuelve a la app
  for _ in $(seq 20); do [ -s "$SALIDA/login-url" ] && break; sleep 1; done
  [ "$sesion" = si ] && [ -s "$SALIDA/login-url" ] || return 0 # ya había sesión, o no se quiere
  if [ -n "${VENTANA_LOGIN:-}" ] && ! $VENTANA_LOGIN "$(cat "$SALIDA/login-url")" >"$SALIDA/login-vuelta"; then
    # Sin sesión todas las capturas serían la pantalla de entrar: se para aquí, y dicho.
    echo "login: VENTANA_LOGIN no completó el acceso (su motivo, arriba)" >&2
    return 1
  fi
  echo "login: esperando $SALIDA/login-vuelta" >&2
  for _ in $(seq 300); do [ -s "$SALIDA/login-vuelta" ] && break; sleep 1; done
  entregar "$(cat "$SALIDA/login-vuelta")" login
  rm -f "$SALIDA/login-url" "$SALIDA/login-vuelta"
}
# entregar <vuelta> <quién> — lleva a la app la vuelta que recogió el guion de Playwright.
# Conexión rechazada (curl 7) es que el navegador ya se la llevó al seguir la redirección
# y la app cerró su escucha (ver LAS VUELTAS AL LOOPBACK); otro fallo sí es un problema.
entregar() {
  local rc=0
  curl -fsS -o /dev/null --max-time 20 "$1" 2>/dev/null || rc=$?
  case "$rc" in
  0) ;;
  7) echo "$2: la vuelta ya la llevó el navegador a la app" >&2 ;;
  *) echo "$2: la vuelta no entró en la app (curl $rc)" >&2 ;;
  esac
}
# boton <captura> <x1> <x2> <y> — ¿hay un botón principal (el teal de la marca) en esos dos
# puntos de la captura? Dos puntos y no uno: el botón del diálogo de cifrado mide 204 px y
# a su izquierda puede haber otro botón teal (el de la vista sin diálogo, en 538–742).
boton() {
  local x
  for x in "$2" "$3"; do
    convert "$1" -format "%[pixel:p{$x,$4}]" info: 2>/dev/null |
      awk -F'[(),]' '{ exit !($2 < 90 && $3 > $2 + 50 && $4 > $2 + 50) }' || return 1
  done
}
# atender_navegador <segundos> — completa lo que la app abra en el navegador mientras
# configura su cifrado: el SSO de Matrix del alta del dispositivo (con la sesión de Sytue
# ID de la tirada) y, si pide volver a entrar, el inicio de sesión. Tras cada uno espera
# un poco más por si viene otro. Devuelve 1 si en ese plazo no abrió nada.
atender_navegador() {
  local hecho=1 fin=$((SECONDS + $1)) vuelta
  while [ "$SECONDS" -lt "$fin" ]; do
    if [ -s "$SALIDA/matrix-url" ]; then
      echo "cifrado: el SSO de Matrix del alta del dispositivo" >&2
      if vuelta="$($VENTANA_LOGIN --matrix "$(cat "$SALIDA/matrix-url")")"; then
        entregar "$vuelta" "cifrado (SSO de Matrix)"
      fi
      rm -f "$SALIDA/matrix-url"
      hecho=0 fin=$((SECONDS + 15))
    elif [ -s "$SALIDA/login-url" ]; then
      echo "cifrado: la app vuelve a pedir el inicio de sesión en Sytue ID" >&2
      if vuelta="$($VENTANA_LOGIN "$(cat "$SALIDA/login-url")")"; then
        entregar "$vuelta" "cifrado (inicio de sesión)"
      fi
      rm -f "$SALIDA/login-url"
      hecho=0 fin=$((SECONDS + 15))
    fi
    sleep 1
  done
  return "$hecho"
}
# desbloquear <app> <ventana> — «Configurar este dispositivo» (ver EL CIFRADO arriba), en
# rondas: una app puede tener dos cuentas de Matrix que configurar (la de Rooms y la de
# su producto), y cada una vuelve a sacar el diálogo. Las coordenadas son las del botón
# del diálogo a 1280 de ancho (medido en ventana-real 43985202886); el diálogo va
# centrado, así que con otro alto se mueve la mitad.
desbloquear() {
  [ -n "${VENTANA_LOGIN:-}" ] && [ "$sesion" = si ] && [ "$modo" = x11 ] || return 0
  case " ${VENTANA_CIFRADO:-mail hub erp} " in *" $1 "*) ;; *) return 0 ;; esac
  local ronda f altas=0 y=$((427 + (ALTO - 744) / 2))
  medir "$2" 1280 "$ALTO"
  for ronda in 1 2 3 4; do
    f="$SALIDA/$modo/$1-cifrado$ronda.png"
    capturar "$2" "$f"
    if boton "$f" 697 880 "$y"; then
      clic "$2" 792 "$y"
      altas=$((altas + 1))
      if ! atender_navegador 30; then
        echo "$1: «Configurar este dispositivo» no abrió el navegador (ronda $ronda)" | tee -a "$SALIDA/$modo/resumen.txt"
        break
      fi
      # El alta, la recuperación de la clave y la autoverificación del dispositivo.
      sleep 25
    elif boton "$f" 545 735 $((y + 18)); then
      # El diálogo cerrado: el botón de la vista lo vuelve a abrir.
      clic "$2" 640 $((y + 18))
      sleep 3
    else
      break
    fi
  done
  capturar "$2" "$SALIDA/$modo/$1-cifrado-fin.png"
  echo "$1: «Configurar este dispositivo» pulsado $altas vez/veces, ver $1-cifrado*.png" | tee -a "$SALIDA/$modo/resumen.txt"
}

# Solo lectura salvo que se pida escribir (ver la cabecera).
[ "$escribe" = si ] || export SYTUE_SOLO_LECTURA=1
for a in "${apps[@]}"; do
  tema claro
  "$BIN/$a" >"$SALIDA/$modo/$a.log" 2>&1 &
  app=$!
  pids+=("$app")
  w=""
  for _ in $(seq 60); do
    w="$(ventana "$app")"
    [ -n "$w" ] && break
    sleep 0.5
  done
  if [ -z "$w" ]; then
    echo "$a: sin ventana" | tee -a "$SALIDA/$modo/resumen.txt"
    kill "$app" 2>/dev/null || true
    continue
  fi
  entrar
  sleep 8 # los datos de producción
  desbloquear "$a" "$w"
  mover "$w" 1279 743 # el puntero, en una esquina: en medio deja una fila «encima»
  for t in claro oscuro traslucido; do
    tema "$t"
    for ancho in 1280 360; do
      alto=$ALTO
      [ "$ancho" = 360 ] && alto=740
      medir "$w" "$ancho" "$alto"
      capturar "$w" "$SALIDA/$modo/$a-$ancho-$t.png"
      if [ "$cuenta" = si ] && [ "$sesion" = si ]; then
        clic "$w" $((ancho - 37)) 22; sleep 2
        capturar "$w" "$SALIDA/$modo/$a-$ancho-$t-cuenta.png"
        if [ -n "$idiomaY" ]; then
          clic "$w" $((ancho - 115)) "$idiomaY"; sleep 2
          capturar "$w" "$SALIDA/$modo/$a-$ancho-$t-idioma.png"
          tecla "$w" Escape; sleep 1
        fi
        tecla "$w" Escape; sleep 1
        mover "$w" $((ancho - 1)) $((alto - 1))
      fi
    done
  done
  tema "$pasosEn"
  medir "$w" 1280 "$ALTO"
  [ "$sesion" = no ] || recorrer "$a" "$w"
  kill "$app" 2>/dev/null || true
  wait "$app" 2>/dev/null || true
  echo "$a: capturas en $SALIDA/$modo/$a-*.png" | tee -a "$SALIDA/$modo/resumen.txt"
done
if [ "$escribe" = si ] && [ "$sesion" = si ]; then
  for a in "${apps[@]}"; do
    case "$a" in
    docus | calcus | diapos)
      echo "OJO: --escribe ha dejado un documento de plantilla de $a en la cuenta de la tirada: bórralo." | tee -a "$SALIDA/$modo/resumen.txt"
      ;;
    esac
  done
fi
