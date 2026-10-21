#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Texto de interfaz escrito en castellano dentro del código.
#
# Los cinco idiomas son condición de «terminado», no objetivo de una ola, pero
# el defecto reaparece constantemente porque escribir `label="Congelar paneles"`
# es más rápido que buscar la clave — y porque MUCHAS VECES la clave ya existe,
# traducida a los cinco, sin que nadie la use.
#
# Esto es un TRINQUETE, no una puerta cerrada: falla solo si el número SUBE
# respecto a la línea base de abajo. Queda deuda de sobra como para que exigir
# cero bloquease cualquier commit, y un gate que no se puede cumplir se acaba
# desactivando.
#
# Ojo con lo que NO ve: solo mira posiciones de interfaz (`label=`, `title:`,
# `placeholder=`…). El texto suelto en JSX y lo que sale de constantes
# compartidas se le escapa; para eso está `test/e2e/tests/i18n-sin-espanol.spec.ts`,
# que carga las apps pidiendo inglés y lee lo RENDERIZADO. Ninguno de los dos
# sustituye al otro: el spec no abre los popovers, y este no ve el texto suelto.
set -uo pipefail

AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$AQUI" || exit 1

# Línea base medida el 2026-08-07, bajada a 3 el 2026-08-15 al vaciar de castellano
# el catálogo de apps de `@sytue/ui`. Baja este número cuando limpies; nunca lo subas.
#
# Los 3 que quedan son falsos positivos comprobados: un comando de terminal de
# ejemplo (`ls -la /app`) y dos tamaños de instancia de la demo de Cloud, que
# imitan aposta lo que devuelve el backend sin traducir.
BASE=3

# shellcheck disable=SC2016  # es un programa perl: el `$` es suyo, no del shell.
CUENTA="$(find apps packages \( -name '*.tsx' -o -name '*.ts' -o -name '*.astro' \) \
  -not -path '*/node_modules/*' -not -path '*/dist/*' -not -path '*/.astro/*' \
  -not -path '*/i18n/messages/*' -not -path '*/test/*' \
  -not -name '*.test.ts' -not -name '*.test.tsx' -print0 2>/dev/null |
  xargs -0 perl -0777 -ne '
    # Fuera los comentarios: media docena de componentes de @sytue/ui documentan
    # su uso con un ejemplo en castellano dentro del JSDoc, y eso no lo lee nadie
    # más que quien edita el fichero.
    s{/\*.*?\*/}{}gs;
    s{^\s*//.*$}{}gm;
    while (m{(label|title|description|footnote|placeholder|hint|heading|intro|action|aria-label)[:=]\s*\n?\s*[\x27"]((?:[^\x27"\\]|\\.){5,}?)[\x27"]}gs) {
      my $v = $2;
      next if $v =~ m{^(https?|/|\.|\@|[a-z_]+\.[a-z])};
      print "1\n" if $v =~ /[\x{e1}\x{e9}\x{ed}\x{f3}\x{fa}\x{f1}\x{bf}\x{a1}]/
                  || $v =~ /\b(de|la|el|los|las|un|una|que|tu|se|no|con|para)\b/;
    }' 2>/dev/null | wc -l | tr -d ' ')"

if [ "$CUENTA" -gt "$BASE" ]; then
  echo "✗ i18n: $CUENTA rótulos en castellano en el código (la línea base es $BASE)."
  echo "  Antes de crear una clave, busca el literal en el \`es.ts\` de la app: lo"
  echo "  normal es que ya esté escrita en los cinco idiomas y solo falte usarla."
  exit 1
fi

if [ "$CUENTA" -lt "$BASE" ]; then
  echo "✓ i18n: $CUENTA rótulos en castellano ($((BASE - CUENTA)) menos que la línea base)."
  echo "  Baja BASE a $CUENTA en scripts/lint-i18n.sh para que no vuelvan a subir."
else
  echo "✓ i18n: $CUENTA rótulos en castellano, sin empeorar."
fi
