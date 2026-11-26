#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Baja una herramienta del CI PRIMERO del espejo de Sytue Git y, solo si allí no
# está, de su origen. Con el sha256 comprobado en los dos casos, que es lo que de
# verdad garantiza qué se ejecuta.
#
#   bajar-herramienta.sh <nombre> <versión> <fichero> <sha256> <url-origen> <destino>
#
# Por qué con respaldo y no solo el espejo: el espejo se puebla cuando cambia
# `runner/Containerfile`, así que entre subir una versión y que el espejo corra
# hay una ventana en la que el fichero aún no está. Cortar ahí el CI sería
# cambiar «sale a github.com» por «no compila», que es peor.
#
# El aviso cuando cae al origen no es decorativo: es lo que permite saber que el
# espejo se quedó atrás en vez de descubrirlo el 2026-10-01, cuando caduque la
# excepción de github.com en `compliance/supply-chain/providers.tsv`.
set -euo pipefail

nombre="${1:?falta el nombre}"
version="${2:?falta la versión}"
fichero="${3:?falta el fichero}"
sha="${4:?falta el sha256}"
origen="${5:?falta la URL de origen}"
destino="${6:?falta el destino}"

# El espejo era el registro genérico de Sytue Git (y después de git.gay). Con el CI
# en GitHub no hay espejo por defecto: solo se usa si se da su raíz. Sin ella se
# baja del origen directamente y sin aviso, porque no es que el espejo se haya
# quedado atrás: es que no hay. El sha256 se comprueba igual.
FORJA="${SYTUE_TOOLS_MIRROR:-}"
ORG="${SYTUE_GIT_PACKAGE_OWNER:-sytue}"
espejo="$FORJA/api/packages/$ORG/generic/$nombre/$version/$fichero"

# La descarga —reintentos, suelo de velocidad y reanudación de un corte por
# tiempo— es la de `accion.sh`: una sola política para todo lo que baja el CI.
# shellcheck source=/dev/null
. "$(dirname "${BASH_SOURCE[0]}")/descarga-reanudable.sh"
comun=(--proto '=https' --tlsv1.2)

bajado=""
if [ -n "$FORJA" ] && [ -n "${SYTUE_GIT_PACKAGES_TOKEN:-}" ]; then
  # `--retry 3`, como hasta ahora, y no los 6 de la política: que el fichero aún
  # no esté en el espejo es lo normal entre subir una versión y repoblarlo, y
  # detrás está el origen. Con 6, ese 404 son 71 s de espera (medidos) por
  # herramienta antes de bajar de donde sí está; con 3, siete.
  if descargar_reanudable "$destino" "$espejo" "${comun[@]}" --retry 3 \
    -H "Authorization: token $SYTUE_GIT_PACKAGES_TOKEN" 2>/dev/null; then
    bajado="espejo"
  fi
fi

if [ -z "$bajado" ]; then
  if [ -n "$FORJA" ]; then
    echo "! $nombre $version no está en el espejo $FORJA: se baja del origen." >&2
    echo "  Repóblalo con el workflow «espejo de herramientas en Sytue Git»." >&2
  fi
  descargar_reanudable "$destino" "$origen" "${comun[@]}"
  bajado="origen"
fi

# La verificación es la misma venga de donde venga: el espejo no se cree por ser
# nuestro, se comprueba igual que lo de fuera.
. "$(dirname "${BASH_SOURCE[0]}")/../../../.github/actions/toolchain/sha256.sh"
comprobar_sha256 "$sha" "$destino"
echo "· $nombre $version desde el $bajado"
