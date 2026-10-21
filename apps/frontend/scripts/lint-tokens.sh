#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# CLASES DE COLOR QUE SE ESCRIBEN Y NO EXISTEN.
#
# Tailwind v4 solo crea una utilidad si su token está declarado en el tema. Una
# clase con un nombre inventado —`text-text-danger` en vez de `text-danger-text`—
# no da error: no genera nada y el elemento se queda con el color heredado. Un
# mensaje de error deja de ser rojo y sigue pareciendo un mensaje normal.
#
# Pasó a lo grande: 58 clases inventadas en Sytue Fotos (`bg-bg-base`,
# `text-fg-base`, `text-fg-muted`, `border-border-base` — ninguno de esos cuatro
# tokens existe), y el visor de fotos se abría sin fondo ni color propio. Y el
# MISMO `text-text-danger` estaba en cinco apps: los avisos con `role="alert"` de
# Fotos, Drive, Docs, Calendar y Contacts no salían en rojo.
#
# SE COMPRUEBA CONTRA EL CSS CONSTRUIDO, no contra una lista de nombres. La lista
# se quedaría vieja en cuanto alguien añada un token; el CSS dice lo que de
# verdad se genera para esa app, con su tema y sus `@source`.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

# Qué se mira: TODO literal de cadena, no solo lo que va tras `class=`. Así era al
# principio y se le escapaba lo que vive en `cx(...)` repartido en varias líneas,
# en constantes (`const CLASE = '...'`) o en funciones que devuelven clases:
# `text-sidebar-fg-active` en el árbol de Notepads, `bg-bg-selected` en el tamaño
# de instancia de DevX, `bg-success-subtle` y `bg-primary-bg` en el PDF, y
# `text-body-fg`/`text-muted-fg` en el aviso sin organización del shell, que
# además está en un PAQUETE, fuera de `apps/*/src` (2026-09-22).
#
# Solo prefijos que llevan color, y fuera lo que parece clase y no lo es: los
# atributos de SVG (`stroke-width`, `fill-opacity`, `text-anchor`), las propiedades
# CSS escritas en un `style` (`border-top-color`) y `from-`/`to-`/
# `via-`, que aquí serían degradados —la casa no los usa— y en cambio casan con
# rutas y claves (`to-dos`).
clases_de_color() {
  grep -rhoE "['\"\`][^'\"\`]*['\"\`]" "$@" --include='*.tsx' --include='*.astro' --include='*.ts' \
    --exclude='*.test.*' --exclude-dir=node_modules --exclude-dir=dist 2>/dev/null |
    grep -ohE "(^|[ \"'\`:])(bg|text|border|ring|fill|stroke|decoration|divide|outline|accent|caret|placeholder)-[a-z][a-z0-9-]*" |
    sed -E "s/^[ \"'\`:]//" |
    grep -vE '^(border|divide|ring|outline)-(0|1|2|4|8|none|inset|x|y|t|r|b|l|s|e|solid|dashed|dotted|hidden|double|offset-[0-9]+)$' |
    grep -vE '^(text|bg)-(xs|2xs|sm|base|md|lg|[0-9]?xl|left|right|center|justify|start|end|wrap|nowrap|balance|pretty|clip|ellipsis|top|bottom|cover|contain|repeat|no-repeat|fixed|local|scroll|auto|transparent|current|inherit|white|black|anchor)$' |
    grep -vE '^(fill|stroke)-(opacity|width|linecap|linejoin|rule|dasharray|dashoffset|miterlimit|none|current)$' |
    grep -vE '^(accent|caret)-(auto|current|transparent|inherit)$' |
    grep -vE '^decoration-(solid|double|dotted|dashed|wavy|auto|from-font|clone|slice|[0-9])' |
    grep -vE '^(border-(top|right|bottom|left|collapse|color|width|style|radius|spacing|image)|text-(decoration|transform|indent|overflow|align|shadow|rendering)|outline-(color|style|width|offset)|accent-color|caret-color)(-|$)' |
    sort -u || true
}

# faltan <nombre> <css construido> <fuentes…>
faltan() {
  local nombre="$1" out="$2"; shift 2
  local lista=""
  while read -r c; do
    [ -n "$c" ] || continue
    grep -q -- "$c" "$out" || lista="$lista $c"
  done <<< "$(clases_de_color "$@")"
  if [ -n "$lista" ]; then
    echo "✗ $nombre: clases de color que NO se generan —el token no existe en el tema—:$lista" >&2
    fallos=$((fallos + 1))
  fi
}

fallos=0
# Y la ofimática común, que es código de app con su propia hoja (`packages/office`).
for dir in apps/*/ packages/office/; do
  app="$(basename "$dir")"
  css="$(ls "$dir"src/styles/*.css 2>/dev/null | head -1)" || continue
  [ -n "$css" ] || continue
  out="$(mktemp)"
  bunx @tailwindcss/cli -i "$css" -o "$out" >/dev/null 2>&1 || { rm -f "$out"; continue; }
  faltan "$app" "$out" "$dir"src
  # Los paquetes de interfaz se pintan dentro de cada app y el portal los escanea
  # todos (`@source`): se comprueban una vez, contra su hoja.
  [ "$app" = portal ] && faltan "paquetes (ui, shell)" "$out" packages/ui/src packages/shell/src
  rm -f "$out"
done

if [ "$fallos" -gt 0 ]; then
  echo "  Busca el nombre bueno en packages/config/src/theme.css (--color-*)." >&2
  exit 1
fi
echo "✓ tokens: ninguna app escribe clases de color que no existan"
