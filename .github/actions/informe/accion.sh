#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Publica un fichero de log como incidencia del repositorio.
#
#   .github/actions/informe/accion.sh <fichero> "<título>"
#
# POR QUÉ EXISTE. Nació en git.gay, donde NO había forma de leer el log de una
# ejecución desde fuera (la API devolvía 404 en `runs/<n>` y la web exigía
# sesión): un job en rojo solo se podía diagnosticar en el navegador. En GitHub
# el log sí se lee, y esto se conserva porque una incidencia con la cola del
# fallo sigue siendo lo primero que se abre, se puede consultar por API y se
# cierra cuando se resuelve. Habla la API de GitHub: la lista de incidencias
# abiertas trae también las pull requests, que se descartan.
#
# EL LOG VA EN CLARO, NO EN BASE64. Antes se mandaba codificado «para no depender
# de jq ni escapar JSON a mano», y el resultado eran incidencias con un churro
# ilegible por cuerpo: había que copiarlo y decodificarlo para saber qué había
# fallado, que es justo lo contrario de para lo que se abren. `jq` está en el
# runner —lo usa el propio ci-frontend dos pasos antes—, así que escapa él, y si
# algún día no estuviera, se cae a base64 DICIÉNDOLO y con el comando para leerlo.
#
# Y SE LIMPIAN LOS COLORES. El log trae secuencias ANSI de bun y de oxlint; en un
# cuerpo de incidencia son ruido que tapa el error.
#
# NUNCA hace fallar al job que lo llama: se invoca desde un paso `if: failure()`
# y el fallo que importa es el de antes, no el de no poder contarlo.
set -uo pipefail

fichero="${1:?falta el fichero de log}"
titulo="${2:-informe $(date -u +%Y-%m-%dT%H:%M:%SZ)}"

[ -s "$fichero" ] || { echo "· $fichero vacío o inexistente: no hay nada que informar"; exit 0; }
[ -n "${GITHUB_TOKEN:-}" ] || { echo "· sin GITHUB_TOKEN: no se puede publicar el informe"; exit 0; }

# Solo la cola: una incidencia con 20 000 líneas no la lee nadie, y el error está
# al final. Si hace falta más, se sube el número aquí.
LIMITE=60000
# `tail -c` corta por bytes y puede partir un carácter UTF-8 por la mitad; `iconv -c`
# lo quita para que el cuerpo sea texto válido.
cola="$(tail -c "$LIMITE" "$fichero" | { iconv -c -f UTF-8 -t UTF-8 2>/dev/null || cat; } | sed -E 's/\x1b\[[0-9;]*[A-Za-z]//g')"

enlace="${GITHUB_SERVER_URL:-}/${GITHUB_REPOSITORY:-}/actions/runs/${GITHUB_RUN_ID:-}"
cuerpo="$(
  printf '%s\n' \
    "**${GITHUB_WORKFLOW:-CI}** ha fallado en \`${GITHUB_REF_NAME:-?}\` (commit \`${GITHUB_SHA:0:7}\`)." \
    "" \
    "Ejecución: ${enlace}" \
    "" \
    "<details><summary>Cola del log</summary>" \
    "" \
    '```text' \
    "$cola" \
    '```' \
    "" \
    "</details>"
)"

publicar() {  # publicar <url> <json>; si falla, dice lo que contestó GitHub
  local codigo
  codigo="$(curl -sS --max-time 30 -X POST -o /tmp/informe-respuesta.json -w '%{http_code}' \
    -H "Authorization: token $GITHUB_TOKEN" \
    -H 'Accept: application/vnd.github+json' \
    -H 'Content-Type: application/json' \
    --data @"$2" "$1" 2>/dev/null)" || codigo=000
  case "$codigo" in 2*) return 0 ;; esac
  echo "· GitHub contestó $codigo: $(head -c 300 /tmp/informe-respuesta.json 2>/dev/null)" >&2
  return 1
}

api="${GITHUB_API_URL:-}/repos/${GITHUB_REPOSITORY:-}"

if command -v jq >/dev/null 2>&1; then
  jq -n --arg t "$titulo" --arg b "$cuerpo" '{title:$t, body:$b}' > /tmp/informe.json

  # UNA INCIDENCIA POR TÍTULO. El gate corre en cada empujón, así que sin esto se
  # abren cuatro del mismo commit y la lista deja de servir para nada. Si ya hay
  # una abierta con este título, el log va como comentario suyo.
  abierta="$(curl -fsS --max-time 20 -H "Authorization: token $GITHUB_TOKEN" \
    -H 'Accept: application/vnd.github+json' \
    "$api/issues?state=open&per_page=50" 2>/dev/null |
    jq -r --arg t "$titulo" 'map(select(.pull_request == null and .title == $t)) | .[0].number // empty' 2>/dev/null)"

  if [ -n "$abierta" ]; then
    jq -n --arg b "$cuerpo" '{body:$b}' > /tmp/informe.json
    if publicar "$api/issues/$abierta/comments" /tmp/informe.json; then
      echo "· informe añadido a la incidencia abierta #$abierta"
      exit 0
    fi
  elif publicar "$api/issues" /tmp/informe.json; then
    echo "· informe publicado como incidencia: $titulo"
    exit 0
  fi
  echo "· no se pudo publicar el informe (se sigue: el fallo que importa es el anterior)"
  exit 0
fi

# Sin jq: base64, pero avisando en el propio cuerpo de que hay que decodificarlo.
# Antes esto era el camino normal y nadie sabía que lo era.
b64="$(printf '%s' "$cola" | base64 -w0 2>/dev/null || printf '%s' "$cola" | base64 | tr -d '\n')"
printf '{"title":"%s","body":"Sin `jq` en el runner, el log va en base64. Para leerlo:\\n\\n    echo <esto> | base64 -d\\n\\n%s"}' \
  "$titulo" "$b64" > /tmp/informe.json
if publicar "$api/issues" /tmp/informe.json; then
  echo "· informe publicado (en base64: el runner no tiene jq): $titulo"
else
  echo "· no se pudo publicar el informe (se sigue: el fallo que importa es el anterior)"
fi
exit 0
