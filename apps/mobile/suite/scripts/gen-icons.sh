#!/usr/bin/env bash
# Genera las FUENTES de icono/splash del PRODUCTO ACTIVO desde su marca. A partir de
# estas, `npm run assets` (@capacitor/assets) produce los assets nativos iOS/Android.
#
#   SYTUE_APP=mail bash scripts/gen-icons.sh   # icono de Sytue Mail
#   bash scripts/gen-icons.sh                  # sin variable: la suite
#
# Salen de branding/marks/ —el logotipo «sy:» con el color de cada producto, generado
# por scripts/gen-marks.sh—: no se inventan logos. NO de branding/icons/, que son las
# teselas del conmutador: un pictograma a tamaño de icono de movil no se distingue de
# los otros diez, y lo que identifica una app en la pantalla de inicio es su color.
#
# Requiere ImageMagick (`magick`).
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../../.." && pwd)"       # raiz del repo (apps/mobile/suite/scripts -> repo)
RES="$HERE/../resources"
MARKS="$ROOT/branding/marks"
IVORY="#faf9f6"                               # marfil (bg claro de la suite)
INK="#0c0e10"                                 # grafito profundo (bg oscuro)

APP="${SYTUE_APP:-one}"
# El SVG se llama como la clave, salvo Sytue Admin: su icono conserva el nombre historico
# `console`, que sigue siendo la clave del producto en el codigo.
case "$APP" in
  admin) MARK="$MARKS/console.svg" ;;
  *)     MARK="$MARKS/$APP.svg" ;;
esac
MASKABLE="${MARK%.svg}-maskable.svg"

command -v magick >/dev/null || { echo "Falta ImageMagick (magick)"; exit 1; }
[ -f "$MARK" ] || { echo "No existe la marca del producto: $MARK"; exit 1; }
[ -f "$MASKABLE" ] || { echo "Falta $MASKABLE (ejecuta scripts/gen-marks.sh)"; exit 1; }
mkdir -p "$RES"

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# icon.png (1024): la variante MASKABLE tal cual —color a sangre y el glifo en la zona
# segura—. Ya viene resuelta del generador de marcas, asi que aqui no se compone nada.
# `-alpha off` porque iOS RECHAZA iconos con transparencia; `-strip` porque ImageMagick
# escribe la fecha dentro del PNG y sin quitarla dos ejecuciones dan bytes distintos.
magick -background none -density 2048 "$MASKABLE" -resize 1024x1024 -alpha off \
  -strip -define png:exclude-chunk=date,time "$RES/icon.png"

# splash.png / splash-dark.png (2732): la marca ENTERA —con su badge y sus esquinas—
# centrada sobre el fondo de la suite. Aqui no recorta nadie, asi que se ve el icono tal cual.
magick -background none -density 2048 "$MARK" -resize 720x720 "$tmp/mark-splash.png"
magick -size 2732x2732 "xc:$IVORY" "$tmp/mark-splash.png" -gravity center -composite -alpha off \
  -strip -define png:exclude-chunk=date,time "$RES/splash.png"
magick -size 2732x2732 "xc:$INK" "$tmp/mark-splash.png" -gravity center -composite -alpha off \
  -strip -define png:exclude-chunk=date,time "$RES/splash-dark.png"

echo "OK ($APP) -> $RES/{icon,splash,splash-dark}.png"
echo "Siguiente: SYTUE_APP=$APP npm run assets   (genera assets nativos)"
