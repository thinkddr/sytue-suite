#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Los gráficos de la ficha de Google Play de cada app (apps/fyne/play/README.md):
#
#   graficos.sh             icono (512, RGBA de 8 bits) y destacado (1024 × 500)
#   graficos.sh <dir>       y además copia las capturas elegidas de <dir>/<app>/, que es
#                           lo que deja SYTUE_CAPTURAS_PLAY=<dir>/<app> en el recorrido
#   graficos.sh --seleccion <app>  imprime solo la selección para el CI, sin ImageMagick
#
# Necesita `magick`, así que va en el Mac.
set -euo pipefail
AQUI="$(cd "$(dirname "$0")" && pwd)"
FYNE="$AQUI/.."
FUENTE="$FYNE/../nucleo/fuentes/Figtree-Bold.ttf"
PAPEL='#f8f5f3' TINTA='#1a3541' # --sytue-neutral-50 y --sytue-ink (branding/tokens/colors.css)

# Las capturas de cada app, por el número que les pone el recorrido (NNN-lo_pulsado.png),
# en el orden de la ficha. Se eligen mirando lo que deja el recorrido: la pantalla
# principal primero, y nada con un error o un «cargando» a la vista.
declare -A CAPTURAS=(
  [bases]="001 005 020 011"
  [calcus]="074 001"
  [calendar]="001 053 002"
  [cast]="001 015 062 037 034"
  [certipdf]="019 020 023"
  [contacts]="001 004 002 006"
  [diapos]="001 002 011"
  [docus]="008 013"
  [drive]="001 013 008"
  [erp]="001 002 013"
  [hub]="001 002"
  [mail]="001 014 002 018"
  [notes]="001 026 020"
  [photos]="003 004"
  [rooms]="001 005"
)

if [ "${1:-}" = --seleccion ]; then
  app="${2:?uso: $0 --seleccion <app>}"
  [ -n "${CAPTURAS[$app]:-}" ] || { echo "graficos.sh: app desconocida: $app" >&2; exit 1; }
  printf '%s\n' "${CAPTURAS[$app]}"
  exit 0
fi

command -v magick >/dev/null || { echo "graficos.sh: falta magick (brew install imagemagick)" >&2; exit 1; }
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
for app in "${!CAPTURAS[@]}"; do
  d="$AQUI/$app/graficos"
  mkdir -p "$d"
  nombre="$(sed -n 's/^[[:space:]]*Name[[:space:]]*=[[:space:]]*"\(.*\)".*/\1/p' "$FYNE/cmd/$app/FyneApp.toml" | head -1)"
  magick "$FYNE/cmd/$app/Icon.png" -resize 512x512 -depth 8 "PNG32:$d/icono.png"
  # La marca con las esquinas de la tesela (radio 9 sobre 40, como el selector) y el nombre.
  magick "$FYNE/cmd/$app/Icon.png" -alpha set -resize 240x240 \
    \( -size 240x240 xc:none -fill white -draw "roundrectangle 0,0,239,239,54,54" \) \
    -compose DstIn -composite "$tmp/marca.png"
  magick -size 1024x500 "xc:$PAPEL" "$tmp/marca.png" -geometry +120+130 -composite \
    -font "$FUENTE" -pointsize 76 -fill "$TINTA" -gravity West -annotate +410+0 "$nombre" \
    -depth 8 "PNG24:$d/destacado.png"
  if [ -n "${1:-}" ]; then
    rm -f "$d"/telefono-*.png
    i=0
    for n in ${CAPTURAS[$app]}; do
      i=$((i + 1))
      origen="$(ls "$1/$app/$n"-*.png | head -1)"
      magick "$origen" -depth 8 "PNG24:$d/telefono-$i.png"
    done
  fi
done
echo "graficos.sh: ${#CAPTURAS[@]} apps"
