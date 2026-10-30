#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Gate: la marca vendorizada es la que producen los generadores.
#
# POR QUÉ. Los favicons, los iconos de @sytue/ui y los de Fyne y Sytue ID salen
# de `branding/`, y quien los escribe es un guion:
# `gen-marks.sh` produce las marcas y `sync-branding.sh` las reparte. Pero nadie
# COMPROBABA que se hubieran ejecutado.
#
# Lo que pasa cuando no se ejecutan no da error: sale un icono viejo. Pasó el
# 06/09: las marcas se rehicieron enteras esa noche y el conmutador de
# aplicaciones se quedó con la familia anterior, así que el mismo producto se veía
# de dos maneras según se mirara la pestaña o el conmutador. Se descubrió mirando
# la pantalla, que es la peor forma de descubrir una regresión.
#
# Se ejecutan los dos generadores y se exige que el árbol quede LIMPIO. Si algo
# cambia, es que lo versionado no es lo que los generadores producen.
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../../.." || exit 1

# El CI prepara el ImageMagick 7 fijado con RSVG mediante el instalador común.
# Si falta, «no se pudo mirar» no equivale a verde. Fuera del CI se deja el aviso.
command -v magick >/dev/null 2>&1 || {
  if [ -n "${SYTUE_MARCA_NO_SE_COMPRUEBA:-}" ]; then
    echo "::warning::lint:marca NO se ha comprobado: $SYTUE_MARCA_NO_SE_COMPRUEBA"
    exit 0
  fi
  # GITHUB_ACTIONS y no CI: el contenedor de los agentes del frontal también pone CI=true.
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "✗ marca: sin ImageMagick 7 (\`magick\`) no se puede comprobar, y en el CI eso no es verde." >&2
    exit 1
  fi
  echo "· sin ImageMagick 7: la marca NO se comprueba aquí (se comprueba en el piloto)"
  exit 0
}

tmp_img="$(mktemp)"
trap 'rm -f "$tmp_img" "$tmp_img".*' EXIT

imagenes_sistema=(
  'apps/fyne/cmd/*/Icon.png'
  services/id/internal/http/marca.svg
  services/id/internal/http/marca.ico
  services/id/internal/http/marca-apple.png
)
destinos=(
  branding
  apps/frontend/packages/ui/src/assets
  ':(glob)apps/frontend/apps/*/public/**'
  "${imagenes_sistema[@]}"
)
sucio_antes="$(git status --porcelain -- "${destinos[@]}" 2>/dev/null)"
if [ -n "$sucio_antes" ]; then
  # Con cambios sin confirmar no se puede distinguir «el generador produce otra
  # cosa» de «alguien está trabajando en ello»; decirlo es mejor que acusar.
  echo "· hay cambios sin confirmar en branding/ o en los destinos: no se comprueba"
  exit 0
fi

# LOS DESTINOS SE BORRAN ANTES DE GENERAR, y esto es la mitad del gate.
#
# Comparar lo generado con lo versionado solo caza lo que el generador PRODUCE:
# una app que no esté en su lista no cambia nada, `git status` sale limpio y el
# gate pasa tan contento con unos iconos copiados a mano. Ha pasado dos veces
# —SyERP y Mi cuenta, las dos con los favicons de Notepads dentro— y las dos se
# descubrieron mirando una pestaña, no aquí.
#
# Borrándolos primero, una app fuera del generador deja su fichero BORRADO y eso
# sí lo ve `git status`. Se recupera con `git checkout` si algo va mal, que es lo
# que dice el mensaje de más abajo.
rm -f apps/frontend/apps/*/public/favicon.ico \
      apps/frontend/apps/*/public/favicon-64.png 2>/dev/null || true
while IFS= read -r -d '' imagen; do
  rm -f -- "$imagen"
done < <(git ls-files -z -- "${imagenes_sistema[@]}")

# Los generadores explican sus fallos por stderr; ocultarlo deja el gate sin causa.
bash scripts/gen-marks.sh >/dev/null || { echo "✗ marca: gen-marks.sh falló" >&2; exit 1; }
bash scripts/sync-branding.sh >/dev/null || { echo "✗ marca: sync-branding.sh falló" >&2; exit 1; }

sucio="$(git status --porcelain -- "${destinos[@]}" 2>/dev/null)"

# SE COMPARA LA IMAGEN, NO LOS BYTES, y esto es la diferencia entre un gate que
# sirve y uno que solo sabe estar en rojo.
#
# Un PNG o un ICO no son reproducibles byte a byte entre versiones de ImageMagick:
# cambian la tabla de Huffman, el orden de los chunks o el relleno, y sale un
# fichero distinto con EXACTAMENTE la misma imagen dentro. Medido el 2026-09-13,
# comparando lo versionado con lo que produce el generador: de 39 ficheros que
# `git status` daba por cambiados, 37 tenían un RMSE por debajo de 0,001 —o sea,
# el mismo icono recomprimido— y solo 2 habían cambiado de verdad.
#
# Mientras el gate se autoexcusaba por no encontrar ImageMagick eso no se notaba.
# En cuanto empezó a correr de verdad en el CI (se instala en el job `quality`),
# puso `quality` en rojo en cada release por diferencias que nadie puede ver.
#
# El umbral son dos órdenes de magnitud de margen: la recompresión mide 0,0001-0,001
# y los cambios reales que se han visto —un icono cuadrado que pasa a redondeado, un
# color de producto, un glifo rediseñado— miden 0,05-0,25.
UMBRAL=0.01
if [ -n "$sucio" ]; then
  reales=""
  for f in $(printf '%s\n' "$sucio" | awk '{print $2}'); do
    case "$f" in
      *.png | *.ico) ;;
      # Lo que no es imagen (SVG, CSS, TS) se compara como siempre: ahí un byte
      # distinto es un cambio distinto, no un detalle del compresor.
      *) reales="$reales $f"; continue ;;
    esac
    # CON SU EXTENSIÓN: ImageMagick deduce el formato por el nombre, y un `.ico`
    # volcado a un temporal sin extensión da «no decode delegate for this image
    # format». Sin esto, los diecinueve favicons salían como «ilegible» y el gate
    # fallaba por no saber leerlos, que es peor que no comprobarlos.
    viejo_img="$tmp_img.${f##*.}"
    if ! git show "HEAD:$f" >"$viejo_img" 2>/dev/null; then
      reales="$reales $f(nuevo)"; continue
    fi
    # `[0]` en los dos lados: un .ico lleva varios tamaños dentro y hay que
    # comparar el mismo, que es el primero (64px).
    # `e-05` TAMBIÉN ES UN NÚMERO. ImageMagick imprime el RMSE normalizado entre
    # paréntesis y usa notación científica cuando es diminuto —«(6.6261e-05)»—, que
    # es justo el caso de la recompresión. Un patrón que solo aceptara dígitos y
    # punto no lo captura, deja `r` vacío y el fichero se cuenta como «ilegible»:
    # el gate acababa fallando por los ficheros que MENOS cambian.
    r="$(magick compare -metric RMSE "${viejo_img}[0]" "${f}[0]" null: 2>&1 |
      grep -oE '\([0-9.]+(e-?[0-9]+)?\)' | tr -d '()')"
    rm -f "$viejo_img"
    [ -n "$r" ] || { reales="$reales $f(ilegible)"; continue; }
    awk -v r="$r" -v u="$UMBRAL" 'BEGIN{exit !(r+0 > u+0)}' && reales="$reales $f($r)"
  done
  if [ -n "$reales" ]; then
    echo "✗ marca: lo versionado NO es lo que producen los generadores." >&2
    echo "  Ejecuta 'bash scripts/gen-marks.sh && bash scripts/sync-branding.sh' y confirma el resultado." >&2
    for f in $reales; do echo "    $f" >&2; done
    exit 1
  fi
  # Solo recompresión: el árbol se deja como estaba. Confirmar 57 ficheros que a la
  # vista son idénticos ensucia el historial y no arregla nada, porque el siguiente
  # que los regenere con otra versión volverá a verlos «cambiados».
  echo "· marca: $(printf '%s\n' "$sucio" | wc -l | tr -d ' ') ficheros recomprimidos (misma imagen); se restauran"
  if ! git checkout -- "${destinos[@]}"; then
    echo "✗ marca: no se pudieron restaurar los archivos recomprimidos." >&2
    exit 1
  fi
fi
# Y EL ESPEJO TIPADO. `packages/config/src/tokens.ts` copia a mano la paleta de
# producto que vive en `branding/tokens/colors.css` —lo dice sync-branding: «un
# espejo tipado a mano»—. Dos copias de la misma verdad se separan solas, y aquí
# separarse significa que el icono de un producto y su acento dejan de ser el
# mismo color sin que nada falle.
#
# SE COMPRUEBA DE tokens.ts HACIA LA PALETA, no al revés: la paleta tiene claves
# que el espejo NO declara a propósito —`pdf` es una superficie dentro de Docs, no
# un producto con clave propia— y exigirlas haría fallar al gate por algo que está
# bien. Lo que no puede pasar es que una clave declarada MIENTA.
malos=""
while IFS='|' read -r clave color; do
  [ -n "$clave" ] || continue
  esperado="$(sed -n "s/^  --sytue-product-$clave: \(#[0-9a-f]\{6\}\);.*/\1/p" branding/tokens/colors.css)"
  [ -n "$esperado" ] || continue
  [ "$color" = "$esperado" ] || malos="$malos $clave($color!=$esperado)"
done <<EOF2
$(sed -n "s/^  \([a-z]*\): '\(#[0-9a-f]\{6\}\)',.*/\1|\2/p" apps/frontend/packages/config/src/tokens.ts)
EOF2
if [ -n "$malos" ]; then
  echo "✗ marca: tokens.ts no refleja la paleta de branding/tokens/colors.css:$malos" >&2
  echo "  El icono y el acento del producto dejarian de ser el mismo color." >&2
  exit 1
fi
echo "✓ marca: favicons, iconos del paquete y marcas coinciden con sus generadores"
