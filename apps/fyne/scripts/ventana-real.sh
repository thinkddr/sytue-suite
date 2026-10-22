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
# editor en esa cuenta y le toca borrarlos (el guion lo recuerda al acabar).
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
# solas, y se borra al salir.
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
rm -f "$SALIDA/login-url" "$SALIDA/login-vuelta" "$SALIDA/abiertas.txt"
cat >"$ENTORNO/bin/xdg-open" <<EOF
#!/bin/sh
printf '%s\n' "\$1" >>"$SALIDA/abiertas.txt"
case "\$1" in *id.*authorize*|*/authorize\?*) printf '%s\n' "\$1" >"$SALIDA/login-url" ;; esac
EOF
chmod +x "$ENTORNO/bin/xdg-open"

pids=()
limpiar() {
  for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
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
pasos() {
  awk -v app="$1" -v escribe="$escribe" -v forma="${FORMA:-marco}" '$1 == app || $1 == "*" || $1 == app "@" forma || (escribe == "si" && $1 == app "!") { $1 = ""; print substr($0, 2) }' <<'FIN'
* foto inicio
mail clic 420 175
mail espera 3
mail foto abrir-correo
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
  if [ -n "${VENTANA_LOGIN:-}" ]; then
    $VENTANA_LOGIN "$(cat "$SALIDA/login-url")" >"$SALIDA/login-vuelta"
  fi
  echo "login: esperando $SALIDA/login-vuelta" >&2
  for _ in $(seq 300); do [ -s "$SALIDA/login-vuelta" ] && break; sleep 1; done
  curl -fsS -o /dev/null "$(cat "$SALIDA/login-vuelta")" || echo "login: la vuelta no entró" >&2
  rm -f "$SALIDA/login-url" "$SALIDA/login-vuelta"
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
