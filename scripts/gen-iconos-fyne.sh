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
# Lo llama `scripts/sync-branding.sh`; se puede lanzar suelto al tocar la marca.
# `sesion.TestElIconoDeCadaAppEsSuMarca` mira la tesela y el contorno de cada uno.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
command -v magick >/dev/null || { echo "Falta ImageMagick (magick): brew install imagemagick" >&2; exit 1; }
css="$root/branding/tokens/colors.css"
# La segunda `--sytue-ink-rgb` y el segundo `--sytue-product-tile-ring` son los del
# bloque `[data-theme='dark']`; la primera pareja es la del claro.
tinta=$(grep -o -- '--sytue-ink-rgb: [0-9, ]*' "$css" | sed -n 2p | sed 's/.*: //; s/ //g')
alfa=$(grep -o -- '--sytue-product-tile-ring: rgba([^)]*), [0-9.]*' "$css" | sed -n 1p | sed 's/.*, //')
[ -n "$tinta" ] && [ -n "$alfa" ] || { echo "gen-iconos-fyne: no encuentro el token del anillo en $css" >&2; exit 1; }
# shellcheck disable=SC2054  # la coma es de ImageMagick («date,time»), no separa elementos
SIN_FECHA=(-strip -define png:exclude-chunk=date,time)
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
