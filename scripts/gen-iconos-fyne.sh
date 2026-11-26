#!/usr/bin/env bash
# Los `Icon.png` de las apps de Fyne (apps/fyne/cmd/<app>/Icon.png): el icono que
# enseña el SISTEMA —Dock, barra de tareas, lanzador, `.desktop`, APK, IPA, .app y
# .exe—, porque `fyne package` saca todos esos de este único fichero.
#
# Salen de la MISMA fuente que el selector de aplicaciones: la marca de
# `branding/marks`, en su variante maskable (color a sangre; iOS no admite
# transparencia y Android pone su propia máscara), y con el MISMO contorno que le
# pone el selector. Ese contorno es el `box-shadow: 0 0 0 1px
# var(--sytue-product-tile-ring)` de `ProductIcon.tsx` y el `conAnillo` de
# `apps/fyne/iconos/marcas.go`: un rectángulo de 3,5 a 36,5 con radio 9,5 sobre el
# viewBox de 40 de la marca y un trazo de 1. Se pinta aquí en ESE sistema de
# coordenadas, dentro de la transformación del glifo de la maskable, así que se ciñe
# al glifo igual que en el selector.
#
# El color es el del tema OSCURO del token, siempre: un icono del sistema no sabe de
# temas, y en claro el token es transparente —no habría borde—.
#
# CON EL PARÁMETRO `office` hace los de Sytue Office, la edición local
# (apps/fyne/cmd-local/<app>/Icon.png; PLAN-EDICION-LOCAL.md §5.2): el de su producto
# con la marca de la edición. Ver «Sytue Office», abajo.
#
# Uso:  scripts/gen-iconos-fyne.sh           los de la conectada (apps/fyne/cmd)
#       scripts/gen-iconos-fyne.sh office    los de Sytue Office (apps/fyne/cmd-local),
#                                            a partir de los de la conectada
#
# Lo llama `scripts/sync-branding.sh` (las dos veces, en ese orden); se puede lanzar
# suelto al tocar la marca. `sesion.TestElIconoDeCadaAppEsSuMarca` mira la tesela y el
# contorno de cada uno, y `local.TestElIconoDeLaEdicionLlevaSuMarca` la marca de la
# edición.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
edicion="${1:-conectada}"
case "$edicion" in
conectada | office) ;;
*)
  echo "uso: $0 [conectada|office]" >&2
  exit 2
  ;;
esac
# shellcheck disable=SC2054  # la coma es de ImageMagick («date,time»), no separa elementos
SIN_FECHA=(-strip -define png:exclude-chunk=date,time)

# --- Sytue Office -----------------------------------------------------------------
#
# EL ICONO DE LA EDICIÓN ES EL DE SU PRODUCTO CON UNA INSIGNIA, no otro icono: la app se
# reconoce por su color y su glifo, y la insignia dice de qué edición es (las dos se
# instalan a la vez, R-CONVIVE, y en el Dock o en el lanzador están una al lado de la
# otra).
#
# LA INSIGNIA es un disco con los colores del símbolo de la casa
# (`branding/logo/sytue-mark.svg`: el negro #0a0a0a de su badge y el blanco de su
# monograma), con un aro blanco y, dentro, el glifo `device` del catálogo de la web
# (`iconos.Dispositivo`: una pantalla sobre su base) —la app y sus datos viven en ESTE
# equipo—, rellenado y no de trazo porque el rasterizador de la casa no dibuja trazos
# (`scripts/gen-marks.sh`). Va abajo a la derecha del glifo, que es donde los sistemas
# ponen la insignia de una variante (la del perfil de trabajo de Android): arriba a la
# derecha va el número de avisos (`escritorio.Insignia`) y abajo a la izquierda, la
# flecha de alias de macOS.
#
#  - ÁREA SEGURA: entera dentro del círculo seguro de la maskable (r = 40 % del lado,
#    409,6 en 1024), el mismo que respeta el glifo (`gen-marks.sh`): la máscara de los
#    iconos adaptativos de Android y el escudo de iOS y de macOS no la cortan. Su borde
#    más lejano queda a √2·184 + 140 = 400 del centro, y el contorno del selector, a 412.
#  - SE SEPARA DEL GLIFO por un hueco de 20 por el que se ve la tesela (y su cuña): ahí
#    se pinta el FONDO de la maskable —la tesela y la cuña al 22 %, `M512 179V512H179Z`
#    por dos—, que borra el glifo.
#  - CONTRASTE: el aro blanco da 4,4:1 o más sobre la tesela de los quince productos y
#    más sobre su cuña (es el blanco del glifo, que se eligió para eso); el negro solo no
#    llegaba a 3:1 en Rooms, Cast y Bases. No depende del tema del escritorio, claro u
#    oscuro: el icono es opaco y la insignia queda dentro de la tesela.
#  - A 16 px es un punto oscuro de 4 px en la esquina del glifo; a 32 se ve el disco con
#    su aro, y de 64 en adelante, la pantalla.
#
# Sale del `Icon.png` de `cmd/<app>` (lo que hace este guion sin parámetro), no de la
# marca: así, fuera de la insignia, es el mismo icono píxel a píxel, y basta con
# ImageMagick sin delegado de SVG (vale el `convert` de la versión 6).
#
# Las medidas, en píxeles del icono de 1024. `local.TestElIconoDeLaEdicionLlevaSuMarca`
# las lee de AQUÍ.
INSIGNIA_X=696      # el centro, en x y en y
INSIGNIA_RADIO=120  # el disco, con su aro
INSIGNIA_ARO=10     # el aro blanco
INSIGNIA_HUECO=20   # lo que se ve de tesela entre el glifo y el aro
INSIGNIA_ESCALA=6.9 # píxeles por unidad de la rejilla de 24 del glifo: mide 148 de ancho

office() {
  local im n=0 cx=$INSIGNIA_X r=$INSIGNIA_RADIO negro='#0a0a0a'
  im=$(command -v magick || command -v convert) || {
    echo "Falta ImageMagick (magick o convert): brew install imagemagick" >&2
    exit 1
  }
  # Sin NINGÚN trozo auxiliar, como los de la conectada: el ImageMagick 6 escribe además
  # `date:timestamp` (que `exclude-chunk=date,time` no quita, y cada ejecución daba otros
  # bytes), `cHRM` y `bKGD`.
  local SIN_TROZOS=(-strip -define png:exclude-chunk=all)
  # x lleva una coordenada de la rejilla de 24 del glifo a píxeles del icono: el centro
  # del glifo, (12, 12), cae en el de la insignia. caja es un rectángulo redondeado de
  # esa rejilla (x0 y0 x1 y1 radio).
  x() { awk -v c="$cx" -v k="$INSIGNIA_ESCALA" -v u="$1" 'BEGIN { printf "%.2f", c + (u - 12) * k }'; }
  caja() {
    local rr
    rr=$(awk -v k="$INSIGNIA_ESCALA" -v u="$5" 'BEGIN { printf "%.2f", u * k }')
    printf 'roundrectangle %s,%s %s,%s %s,%s' "$(x "$1")" "$(x "$2")" "$(x "$3")" "$(x "$4")" "$rr" "$rr"
  }
  # El glifo `device` (`M5.5 5.5h13a1 1 0 0 1 1 1v8.5h-15V6.5a1 1 0 0 1 1-1ZM2.5 18.5h19`)
  # con un trazo de 2,5: la pantalla es su caja por fuera menos su caja por dentro, y la
  # base, la línea con sus remates redondos.
  local pantalla hueco base
  pantalla=$(caja 3.25 4.25 20.75 16.25 2)
  hueco=$(caja 5.75 6.75 18.25 13.75 0.01)
  base=$(caja 1.25 17.25 22.75 19.75 1.25)
  for toml in "$root"/apps/fyne/cmd-local/*/FyneApp.toml; do
    local app origen tesela cuna
    app=$(basename "$(dirname "$toml")")
    origen="$root/apps/fyne/cmd/$app/Icon.png"
    [ -f "$origen" ] || {
      echo "gen-iconos-fyne: cmd-local/$app no tiene en cmd/$app el icono de su producto" >&2
      exit 1
    }
    # La tesela, a un octavo de la esquina, y la cuña, en la esquina de abajo (fuera del
    # glifo y del contorno).
    tesela=$("$im" "$origen" -format '%[pixel:p{128,128}]' info:)
    cuna=$("$im" "$origen" -format '%[pixel:p{1000,1000}]' info:)
    "$im" "$origen" \
      \( -size 1024x1024 "xc:$tesela" -fill "$cuna" -draw 'polygon 1024,358 1024,1024 358,1024' \) \
      \( -size 1024x1024 xc:black -fill white -draw "circle $cx,$cx $((cx + r + INSIGNIA_HUECO)),$cx" \) \
      -composite \
      \( -size 1024x1024 xc:none -fill white -draw "circle $cx,$cx $((cx + r)),$cx" \
      -fill "$negro" -draw "circle $cx,$cx $((cx + r - INSIGNIA_ARO)),$cx" \
      -fill white -draw "$pantalla" -fill "$negro" -draw "$hueco" -fill white -draw "$base" \) \
      -composite -alpha off "${SIN_TROZOS[@]}" "$root/apps/fyne/cmd-local/$app/Icon.png"
    n=$((n + 1))
  done
  echo "gen-iconos-fyne: $n iconos de Sytue Office"
}

if [ "$edicion" = office ]; then
  office
  exit 0
fi

command -v magick >/dev/null || { echo "Falta ImageMagick (magick): brew install imagemagick" >&2; exit 1; }
css="$root/branding/tokens/colors.css"
# La segunda `--sytue-ink-rgb` y el segundo `--sytue-product-tile-ring` son los del
# bloque `[data-theme='dark']`; la primera pareja es la del claro.
tinta=$(grep -o -- '--sytue-ink-rgb: [0-9, ]*' "$css" | sed -n 2p | sed 's/.*: //; s/ //g')
alfa=$(grep -o -- '--sytue-product-tile-ring: rgba([^)]*), [0-9.]*' "$css" | sed -n 1p | sed 's/.*, //')
[ -n "$tinta" ] && [ -n "$alfa" ] || { echo "gen-iconos-fyne: no encuentro el token del anillo en $css" >&2; exit 1; }
n=0
# <directorio de cmd>:<clave de la marca>. El mismo reparto que `productoDe` en
# `apps/fyne/sesion/esquemas_test.go`, que falla si aquí se cruza un color.
for par in calcus:sheets calendar:calendar certipdf:pdf contacts:contacts diapos:slides \
           docus:docs drive:drive erp:erp hub:one mail:mail notes:notes photos:photos rooms:rooms \
           cast:cast bases:bases; do
  dir="$root/apps/fyne/cmd/${par%%:*}"; marca="$root/branding/marks/${par##*:}-maskable.svg"
  [ -d "$dir" ] || continue
  glifo=$(grep -o 'transform="translate(256 256)[^"]*"' "$marca")
  anillo="<g $glifo><rect x=\"3.5\" y=\"3.5\" width=\"33\" height=\"33\" rx=\"9.5\" ry=\"9.5\" fill=\"none\" stroke=\"rgb($tinta)\" stroke-opacity=\"$alfa\" stroke-width=\"1\"/></g>"
  sed "s|</svg>|$anillo</svg>|" "$marca" |
    magick -density 576 svg:- -resize 1024x1024 -alpha off "${SIN_FECHA[@]}" "$dir/Icon.png"
  n=$((n + 1))
done
echo "gen-iconos-fyne: $n iconos de apps de Fyne"
