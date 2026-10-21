#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# PUBLICA LA IMAGEN DE UN FRONTEND PARA LAS ARQUITECTURAS QUE SE LE PIDAN.
#
# QUIÉN DECIDE CUÁLES: `scripts/arquitecturas-imagen.sh`, que es de donde saca
# `ARQUITECTURAS` la release. Para los frontends son amd64 y solo amd64 —los
# trece los despliega el piloto y sytue.com se sirve como assets estáticos—,
# así que en la práctica aquí se construye UNA y es la cruzada. El defecto de este guion
# —las dos, la nativa primero— se queda para las invocaciones a mano y porque
# es lo que prueba el test: la maquinaria de cruzar tiene que seguir sana, que
# es la que consume el frontal público.
#
# EL MISMO PROBLEMA QUE EN LOS SERVICIOS (ver scripts/publicar-imagen-go.sh):
# los runners son ARM64, el servidor de la suite es x86_64, y una imagen ajena
# no falla al descargarse sino al ARRANCAR. Pero la solución NO puede ser la
# misma, y esto se midió antes de escribir nada:
#
#   $ bun install --frozen-lockfile --production   # en la imagen de runtime
#   $ find node_modules -name '*.node' -o -name '*.so*' | wc -l
#   16
#
# Es decir, los `node_modules` de producción NO son solo JavaScript: traen
# `@img/sharp-linuxmusl-arm64` —el que el SSR carga para servir imágenes—,
# `lightningcss`, el compilador de Astro, `rolldown` y `satteri`. Montar el
# mismo contenido sobre la base amd64 daría una imagen que arranca y que falla
# al pedir la primera imagen, que es la peor forma de fallar.
#
# LO QUE SÍ FUNCIONA, y es barato porque no emula nada:
#
#   1. `bun install --cpu x64 --os linux` instala el árbol del destino desde el
#      runner ARM (bun 1.4.2 lo acepta; `--cpu` afecta a las dependencias
#      opcionales, que es donde viven esos binarios). Lo hace la etapa
#      `prod-deps` del Dockerfile, con `--build-arg CPU_DESTINO`.
#   2. El `dist` de Astro es JavaScript y no depende de la arquitectura: se
#      construye una sola vez por imagen, nativo.
#   3. Queda un solo ejecutable de la arquitectura equivocada: el propio `bun`,
#      que viene de la imagen base. Se cambia con `crane rebase`, que sustituye
#      las capas de la base y deja intactas las de encima. La base
#      `oven/bun:1.4.3-alpine` está fijada por el digest de su ÍNDICE, que trae
#      amd64 y arm64, así que las dos variantes salen de ese mismo digest sin
#      tocar el pinning.
#
# USO:
#   DESTINO=ghcr.io/thinkddr TAG=v3.3.0 COMMIT=$(git rev-parse HEAD) \
#     DOMINIO=sytue.eu scripts/publicar-imagen-frontend.sh portal
#
#   # el segundo juego, el de sytue.com (frontal, ARM64):
#   DESTINO=… TAG=… COMMIT=… DOMINIO=sytue.com SUFIJO=-com ARQUITECTURAS=arm64 \
#     scripts/publicar-imagen-frontend.sh portal
set -euo pipefail

# shellcheck source=scripts/lib-portabilidad.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib-portabilidad.sh"
construir="$(dirname "${BASH_SOURCE[0]}")/construir-imagen.sh"

app="${1:?uso: publicar-imagen-frontend.sh <app>}"
: "${DESTINO:?falta DESTINO (p. ej. ghcr.io/thinkddr)}"
: "${TAG:?falta TAG}"
: "${DOMINIO:?falta DOMINIO (el DOMAIN del .env del servidor)}"
COMMIT="${COMMIT:-}"

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$raiz"
df=apps/frontend/Dockerfile
# `SUFIJO` separa los DOS JUEGOS de imágenes: el de `sytue.eu` (sin sufijo, al
# piloto) y el de `sytue.com` (`-com`, al frontal). Es la misma fuente con otro
# `DOMINIO` horneado, y tiene que ser otro nombre de imagen porque la etiqueta
# es la misma versión: publicar las dos como `sytue-frontend-mail:vX.Y.Z` haría
# que la segunda pisara a la primera y el piloto acabara sirviendo la suite de
# `.com`. Ver ADR 0049 y `scripts/arquitecturas-imagen.sh`.
nombre="sytue-frontend-$app${SUFIJO:-}"

# La base sale del Dockerfile —es la del primer FROM— para no tener el digest
# escrito en dos sitios: el día que Renovate lo suba, `crane rebase` seguiría
# quitando las capas de la base VIEJA de una imagen construida con la nueva, y
# eso no encaja: falla en vez de publicar algo raro, pero falla la release.
base="$(grep -m1 '^FROM ' "$df" | awk '{print $2}')"

case "$(uname -m)" in
  aarch64 | arm64) nativa=arm64 ;;
  *) nativa=amd64 ;;
esac
ARQUITECTURAS="${ARQUITECTURAS:-$nativa $([ "$nativa" = amd64 ] && echo arm64 || echo amd64)}"

declare -a publicadas=()
read -ra arquitecturas <<<"$ARQUITECTURAS"
for arq in "${arquitecturas[@]}"; do
  ref="$DESTINO/$nombre:$TAG-$arq"
  echo "── frontend $app ($arq)"

  # `x64` y no `amd64`: es como llama bun a esa arquitectura (la de Node).
  declare -a cruzado=()
  [ "$arq" = "$nativa" ] || cruzado=(--build-arg "CPU_DESTINO=$([ "$arq" = amd64 ] && echo x64 || echo arm64)")

  # LOS MISMOS BUILD-ARGS PARA LAS DOS, escritos una vez. Si aquí faltara uno,
  # la imagen se construye igual y el fallo aparece en el navegador del usuario:
  # los frontends Astro HORNEAN las URL públicas en tiempo de build, y VERSION y
  # COMMIT son los que contesta su /healthz y pinta el panel de la consola.
  #
  # Construir y subir es UNA orden contra el BuildKit del CI (ADR 0066): la imagen
  # no pasa por ningún daemon, así que no queda etiqueta local que borrar después.
  # El registro ha devuelto `error from registry: unknown` después de aceptar capas
  # (v3.6.4, turnos 1 y 3): el reintento repite la orden entera, y lo construido ya
  # está en la caché, así que la repetición solo vuelve a subir.
  reintentar 3 15 timeout 3000 "$construir" --target runtime ${cruzado[@]+"${cruzado[@]}"} \
    --build-arg "VERSION=$TAG" \
    --build-arg "COMMIT=$COMMIT" \
    --build-arg "APP=$app" \
    --build-arg "PUBLIC_OIDC_ISSUER=https://id.$DOMINIO" \
    --build-arg "PUBLIC_HUB_URL=https://hub.$DOMINIO" \
    --build-arg "PUBLIC_SYTUE_HUB_BASE_URL=https://hub.$DOMINIO" \
    --build-arg "PUBLIC_SYTUE_DRIVE_BASE_URL=https://drive.$DOMINIO" \
    --build-arg "PUBLIC_SYTUE_DOCS_BASE_URL=https://docs.$DOMINIO" \
    --build-arg "PUBLIC_SYTUE_CLOUD_API_URL=https://cloud.$DOMINIO/api" \
    -f "$df" --push "$ref" apps/frontend

  if [ "$arq" != "$nativa" ]; then
    # EL ÚNICO EJECUTABLE QUE QUEDA MAL ES EL `bun` DE LA BASE. Se cambia la base
    # entera por la del destino; lo de encima —node_modules del destino y el
    # dist, que es JS— no se toca. Y después se dice que la imagen es de esa
    # arquitectura: `rebase` cambia las capas, no la configuración.
    viejo="${base%@*}@$(timeout 300 crane digest --platform "linux/$nativa" "$base")"
    nuevo="${base%@*}@$(timeout 300 crane digest --platform "linux/$arq" "$base")"
    timeout 900 crane rebase "$ref" --old_base "$viejo" --new_base "$nuevo" -t "$ref"
    timeout 300 crane mutate "$ref" -t "$ref" --set-platform "linux/$arq"
  fi
  publicadas+=("$ref")
done

# Un solo `:$TAG` que sirve a los dos destinos: el servidor (x86_64) y sytue.com
# (ARM64) piden lo mismo y cada uno se lleva lo suyo.
declare -a manifiestos=()
for ref in "${publicadas[@]}"; do manifiestos+=(-m "$ref"); done
timeout 300 crane index append "${manifiestos[@]}" -t "$DESTINO/$nombre:$TAG"
echo "   ✓ $DESTINO/$nombre:$TAG → ${publicadas[*]}"
