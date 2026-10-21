#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Rutas absolutas escritas a mano en apps que se sirven BAJO un prefijo.
#
# Las nueve apps se construyen con `base` (`/admin`, `/one`, `/drive`…), porque
# las dos direcciones son válidas: `admin.sytue.eu/admin` y también
# `<organizacion>.sytue.eu/admin`. Astro prefija sus assets y sus páginas, pero
# NO lo que escribimos a mano: un `href="/users"` o un `location.assign('/apps')`
# apuntan a la raíz del host, donde el ingress no enruta nada, y el navegador
# recibe el 404 de Traefik.
#
# Así estuvo TODA la navegación en producción, y el login con ella: el guard
# redirigía a `/auth/login` en vez de `/admin/auth/login`, de modo que no se
# podía entrar en ninguna de las nueve apps. Nada fallaba en los logs: son 404
# del ingreso, no errores de la aplicación.
#
# Lo que SÍ está cubierto en un punto único y este gate no cuenta:
#   · `fetch('/api/…')` y `fetch('/auth/…')` — los prefija `basePathScript`
#     (packages/ui/src/basePath.ts), inyectado por el layout de cada app.
#   · el login del guard — `basePath` del BFF (@sytue/auth) prefija esa URL.
#
# Y lo que NO estaba cubierto aunque esta lista dijera que sí: el valor del
# `returnTo`. `bff.basePath` prefija la URL DEL LOGIN, no el destino que se le
# pasa dentro. Siete sitios escribían ahí una ruta literal —`'/bienvenida'`,
# `'/dispositivos'`, `` `/files/${id}` ``— y el resultado era que te
# autenticabas y aterrizabas en un 404: medido en producción, `/one/bienvenida`
# redirigía con `returnTo=%2Fbienvenida` y `/bienvenida` es 404. La línea de esta
# lista es la que hizo que nadie lo mirara durante siete ficheros.
#
# Es un TRINQUETE, no una puerta cerrada: falla solo si el número SUBE respecto a
# la línea base. Queda deuda en las apps que aún no se han barrido, y un gate que
# no se puede cumplir se acaba desactivando.
set -uo pipefail

AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$AQUI" || exit 1

# Apps con `base` en su astro.config.mjs. Las que se sirven en la raíz (www,
# platform) no tienen este problema y no se cuentan.
APPS="$(grep -l "base: '/" apps/*/astro.config.mjs 2>/dev/null | cut -d/ -f2 | sort -u)"
[ -n "$APPS" ] || { echo "✓ rutas: ninguna app usa base"; exit 0; }

total=0
detalle=""
# La ofimática común (`packages/office`) es código de las apps que la montan
# —Docus, Calcus, Diapos y CertiPDF, todas con `base`—, así que se cuenta igual.
for app in $APPS office; do
  src="apps/$app/src"
  [ "$app" = office ] && src="packages/office/src"
  # `href="/x"`, `href: '/x'` y los saltos de navegación por script. Se excluyen
  # los tests y los ficheros de rutas que existen justo para prefijar.
  #
  # Y la TABLA de navegación de la consola (los `href: '/x'` sueltos), que era la
  # línea base entera: sus 17 rutas ya se pintan con `r(n.href)`, así que estaban
  # bien desde el barrido y el gate las contaba igual —lo que dejaba una deuda
  # falsa e impedía añadir una entrada al menú sin subir el número—. Se excluye la
  # forma de DATO en ese fichero; un `href="/x"` suyo se sigue contando.
  # `href={\`/…\`}` y `window.open('/…')` TAMBIÉN. No estaban, y ahí vivían seis
  # rutas reales —abrir un PDF desde Drive, descargar una grabación de Rooms,
  # exportar variables y logs de la consola de nube— que en producción daban 404
  # bajo el host de organización. El parche de `basePathScript` envuelve solo
  # `window.fetch`, así que una URL que se abre o se enlaza a mano no pasa por él:
  # son justo las que este gate tiene que cazar, y eran las que se le escapaban.
  # `encodeURIComponent('/…')` cubre el destino del `returnTo`: todos los usos
  # legítimos le pasan una VARIABLE (`ctx.url.pathname + ctx.url.search`), así que
  # una ruta literal ahí dentro es siempre el fallo.
  # `src={\`/api/…\`}` y `return \`/api/…\`` TAMPOCO estaban, y ahí vivía el fallo
  # del 2026-09-10: «descargar original (.eml)» en Mail bajaba un fichero con
  # «404 page not found» dentro. Eran QUINCE sitios en seis apps —el .eml y los
  # adjuntos de un correo, la foto de un contacto, los adjuntos y emojis de
  # Rooms, las miniaturas de Drive, sus descargas de versiones y ZIP, y cuatro
  # exportaciones de la consola—, todos rotos bajo el host de organización.
  #
  # LA FORMA QUE SE LES ESCAPABA ES LA FUNCIÓN QUE COMPONE LA URL: el `href`
  # recibe una variable, así que mirar solo el atributo no la ve. Se cuenta el
  # `return` de una ruta `/api/` literal, que es donde nace.
  #
  # Prefijarla es seguro aunque acabe en un `fetch`: `withBase` es idempotente y
  # el shim solo toca lo que empieza por `/api/`, así que una ruta ya prefijada
  # pasa de largo. No hay doble prefijo posible.
  n="$( { grep -rEn "href=\"/[a-z]|href: '/[a-z]|href=\{\`/[a-z]|src=\{\`/api/|src=\"/api/|return \`/api/|return '/api/|window\.open\(\`?'?/[a-z]|location\.(assign|replace)\('/[a-z]|location\.href = '/[a-z]|encodeURIComponent\((\`|')/" \
        "$src" --include='*.tsx' --include='*.ts' --include='*.astro' 2>/dev/null \
        | grep -v '\.test\.' | grep -v '/lib/rutas\.ts' | grep -v '/pages/api/' \
        | grep -vE "ConsoleShell\.tsx:[0-9]+:.*href: '/" || true; } | wc -l | tr -d ' ')"
  total=$((total + n))
  [ "$n" -gt 0 ] && detalle="$detalle  $app: $n\n"
done

# Línea base del 2026-08-15, tras barrer console y los iconos de los layouts.
# BAJARLA al arreglar una app; nunca subirla.
#
# 32 → 26: barridas las navegaciones de docs (abrir un documento), portal (el
# lanzador, el onboarding y su cierre de sesión) y cloud-dashboard (entrar en un
# proyecto, plantillas, factura), con el mismo helper `r()`.
# 17 → 1: las 17 eran la tabla del menú de la consola, que ya iba por `r()` (ver
# la exclusión de arriba). La que queda es de contacts y es real.
# 1 → 0: barridas las SIETE que el patrón ampliado destapó. Seis eran `href={`/…`}`
# o `window.open('/…')`, formas que el gate no miraba y donde vivía el fallo real:
# abrir un PDF desde Drive daba 404 bajo el host de organización, y lo mismo la
# descarga de una grabación de Rooms y las exportaciones de la consola de nube.
# La séptima era la de contacts que arrastraba la línea base.
#
# EN CERO YA NO ES UNA DEUDA, es una invariante: cualquier ruta absoluta nueva
# rompe el gate. Que siga así.
BASE_LINE=0

if [ "$total" -gt "$BASE_LINE" ]; then
  echo "✗ rutas: $total rutas absolutas sin prefijo (línea base $BASE_LINE)." >&2
  printf "%b" "$detalle" >&2
  echo "  En una app con \`base\`, '/x' es la raíz del HOST y en producción devuelve 404." >&2
  echo "  Usa el helper de la app (\`r('/x')\`, ver apps/console/src/lib/rutas.ts)." >&2
  exit 1
fi
if [ "$total" -lt "$BASE_LINE" ]; then
  echo "✓ rutas: $total sin prefijo (mejor que la línea base $BASE_LINE — bájala en el script)."
  exit 0
fi
echo "✓ rutas: $total absolutas sin prefijo, sin empeorar."
