#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# CONSTRUYE UNA IMAGEN CON EL BUILDKIT DEL CI, con la misma forma que `docker build`.
#
#   construir-imagen.sh [--build-arg K=V]... [--target ETAPA] [--platform linux/ARQ] \
#     -f DOCKERFILE (--push REF | --tar REF FICHERO) CONTEXTO
#
#   --push REF          la sube al registro con la credencial de ~/.docker/config.json
#   --tar REF FICHERO   la deja en un tar con el formato de `docker save` (el que lee trivy)
#
# EL DEMONIO ES EL DEL FRONTAL (ADR 0066): un `buildkitd` sin root, de su propio
# usuario, compartido por los jobs a través de SU socket (`BUILDKIT_HOST`), no del
# de Docker. Por eso la imagen no pasa por ningún daemon: ni `docker push`, ni una
# etiqueta local que haya que borrar después, ni capas colgantes que la poda del host
# pueda quitarle a un build ajeno. Lo que un build está usando lo protege el propio
# BuildKit con sus leases.
#
# Medias Docker y no OCI (`oci-mediatypes=false`), como publicaba `docker push`: el
# resto de la tubería (`crane append/rebase/index`, el actualizador del piloto) no
# tiene que notar el cambio de constructor. Sin procedencia ni SBOM adjuntos, que es
# lo que hace `buildctl` si no se le piden: con ellos el registro recibiría un índice
# en vez de un manifiesto, y `crane append -b` o `crane rebase` encima dejarían de
# encajar.
set -euo pipefail

uso() { sed -n '7,11p' "$0" | sed 's/^# \{0,1\}//' >&2; exit 2; }
: "${BUILDKIT_HOST:?falta BUILDKIT_HOST: el socket del BuildKit del CI (unix:///run/sytue-ci/buildkitd.sock)}"

# Solo las recetas que declaran este ARG invalidan su capa apt diariamente.
# Un RUN apt-get update/upgrade en caché NO comprueba los parches publicados hoy.
declare -a opts=(--opt "build-arg:SYTUE_SECURITY_REFRESH=$(date -u +%Y-%m-%d)")
df="" salida=""
while [ $# -gt 1 ]; do
  case "$1" in
    --build-arg) opts+=(--opt "build-arg:$2"); shift 2 ;;
    --target) opts+=(--opt "target=$2"); shift 2 ;;
    --platform) opts+=(--opt "platform=$2"); shift 2 ;;
    -f) df="$2"; shift 2 ;;
    --push) salida="type=image,name=$2,push=true,oci-mediatypes=false"; shift 2 ;;
    --tar) salida="type=docker,name=$2,dest=$3"; shift 3 ;;
    *) echo "construir-imagen.sh: no conozco «$1»" >&2; uso ;;
  esac
done
if [ $# -ne 1 ] || [ -z "$df" ] || [ -z "$salida" ]; then uso; fi

exec buildctl build --progress plain --frontend dockerfile.v0 \
  --local "context=$1" --local "dockerfile=$(dirname "$df")" --opt "filename=$(basename "$df")" \
  ${opts[@]+"${opts[@]}"} --output "$salida"
