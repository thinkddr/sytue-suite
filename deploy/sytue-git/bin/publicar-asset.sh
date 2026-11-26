#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Publica ficheros como assets de una release del repositorio (GitHub).
#
#   publicar-asset.sh <tag> <fichero>...
#
# Existe porque cada job tenía que armar a mano el mismo baile de curl: tres
# copias de treinta líneas son tres copias que divergen, y la que diverge se
# descubre con la release ya cortada. Nació contra Sytue Git y se portó a GitHub
# el 2026-09-27: cambian la URL de subida (GitHub tiene un servidor aparte para
# los assets, `uploads.github.com`, y lo dice en `upload_url` de la release), la
# ruta para borrar un asset (`releases/assets/<id>`, sin el id de la release) y
# cómo se comprueba que el tag existe (`git/ref/tags/<tag>`).
#
# Necesita SYTUE_GIT_TOKEN (el GITHUB_TOKEN del job, con `contents: write`) y
# GITHUB_API_URL/GITHUB_REPOSITORY, que el runner define.
set -euo pipefail

comprobar=0
if [ "${1:-}" = --comprobar ]; then comprobar=1; shift; fi
TAG="${1:?falta el tag de la release}"
shift

: "${SYTUE_GIT_TOKEN:?falta SYTUE_GIT_TOKEN}"
api="${GITHUB_API_URL:?}/repos/${GITHUB_REPOSITORY:?}"
auth=(--header "Authorization: token $SYTUE_GIT_TOKEN" --header "Accept: application/vnd.github+json")
# Tope y reintentos en TODAS: sin --max-time, un servidor que no responde deja el
# job esperando hasta el timeout del runner sin decir a qué esperaba.
comun=(-fsS --max-time 300 --retry 3 --retry-connrefused)

# La release la crea normalmente `release.yml` con el mismo tag, pero los
# workflows corren a la vez y NO hay orden garantizado entre ellos: publicar
# nada más cortar el tag hacía que un job compilara entero y muriera con un 404
# porque el otro seguía en cola. Se crea como BORRADOR si falta, igual que
# `crear-release`, y es idempotente: quien llegue segundo se la encuentra y
# sigue. Que el orden no importe sale más barato que acordarse del orden.
#
# Imprime «id<TAB>upload_url» de la release, con la plantilla `{?name,label}`
# ya quitada de la URL de subida.
# GitHub NO devuelve los BORRADORES por `releases/tags/<tag>` (404); sí salen en el
# listado, para quien puede escribir en el repositorio. Por eso se busca en los dos.
buscar_release() {
  local json
  json="$(curl "${comun[@]}" "${auth[@]}" "$api/releases/tags/$TAG" || true)"
  if [ -z "$(jq -r '.id // empty' <<<"${json:-null}" 2>/dev/null)" ]; then
    json="$(curl "${comun[@]}" "${auth[@]}" "$api/releases?per_page=100" |
      jq -c --arg t "$TAG" '[.[] | select(.tag_name == $t)][0] // empty' || true)"
  fi
  [ -n "$json" ] && [ "$(jq -r '.id // empty' <<<"$json")" != "" ] || return 1
  jq -r '"\(.id)\t\(.upload_url | sub("\\{.*$"; ""))"' <<<"$json"
}

release() {
  buscar_release && return 0
  # Solo se crea sobre un TAG que exista. `release-desktop` despachado desde una
  # rama sin `inputs.ref` llega aquí con TAG=main: un borrador con tag_name «main»
  # se destaparía un día y crearía un tag llamado como la rama. El tag lo pone
  # quien corta la release; este guion no inventa ninguno.
  if ! curl "${comun[@]}" "${auth[@]}" "$api/git/ref/tags/$TAG" >/dev/null 2>&1; then
    echo "«${TAG}» no es un tag del repositorio: no se crea una release para él (¿despacho desde una rama sin inputs.ref?)" >&2
    return 1
  fi
  curl "${comun[@]}" -X POST "${auth[@]}" -H 'Content-Type: application/json' \
    -d "$(jq -n --arg t "$TAG" '{tag_name:$t, name:$t, draft:true, prerelease:false}')" \
    "$api/releases" >/dev/null || true
  buscar_release
}

# Capturar primero el resultado: `read` sobre una sustitución de proceso pierde
# el estado del productor; los errores de API deben propagarse y ser visibles.
if [ "$comprobar" = 1 ]; then
  # Solo lectura: diagnosticar el acceso a una release existente sin compilar,
  # crear borradores, borrar assets ni publicar nada.
  info="$(buscar_release)" || { echo "no se pudo leer la release $TAG" >&2; exit 1; }
else
  info="$(release)" || { echo "no se pudo crear ni leer la release $TAG" >&2; exit 1; }
fi
IFS=$'\t' read -r id subida <<<"$info"
if [ -z "${id:-}" ] || [ -z "${subida:-}" ]; then
  echo "no se pudo crear ni leer la release $TAG" >&2
  exit 1
fi
if [ "$comprobar" = 1 ]; then
  echo "· release $TAG accesible (id $id)"
  exit 0
fi

for fichero in "$@"; do
  [ -e "$fichero" ] || { echo "no existe: $fichero" >&2; exit 1; }
  nombre="$(basename "$fichero")"
  # Se borran TODOS los anteriores con ese nombre antes de subir: GitHub rechaza
  # un asset con un nombre que ya existe (422), así que reintentar un job moría
  # aquí; y borrar solo el primero dejaría las copias que ya hubiera.
  viejos="$(curl "${comun[@]}" "${auth[@]}" "$api/releases/$id/assets?per_page=100" |
    jq -r --arg n "$nombre" '.[] | select(.name == $n) | .id')"
  for viejo in $viejos; do
    curl "${comun[@]}" -X DELETE "${auth[@]}" "$api/releases/assets/$viejo" >/dev/null
  done
  # 600 y no 300: aquí viaja el binario entero, no un JSON. El nombre va en la
  # query CODIFICADO: un espacio o un `&` sin codificar la parten. El cuerpo es
  # el fichero tal cual (`--data-binary`), no un formulario: es lo que pide el
  # servidor de subidas.
  curl -fsS --max-time 600 --retry 3 --retry-connrefused -X POST "${auth[@]}" \
    -H 'Content-Type: application/octet-stream' \
    --data-binary "@$fichero" \
    "$subida?name=$(jq -rn --arg n "$nombre" '$n|@uri')" >/dev/null
  echo "· publicado $nombre"
done
