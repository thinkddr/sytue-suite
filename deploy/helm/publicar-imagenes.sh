#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)
#
# Construye las imágenes de Sytue Suite y las sube a TU registro, con los nombres
# que espera el chart de Helm (global.imageRegistry/<repositorio>:<tag>).
#
#   bash deploy/helm/publicar-imagenes.sh <registro> <dominio> [tag]
#   bash deploy/helm/publicar-imagenes.sh ghcr.io/mi-org suite.miempresa.eu
#
# Antes: `docker login <registro>`. El tag por defecto es el del chart
# (image.tag de los subcharts); si usas otro, pásalo también a Helm con
# --set global.image.tag=<tag>. Las apps web hornean su dominio público al
# compilarse, por eso se pide aquí.
set -euo pipefail

REGISTRO="${1:?uso: $0 <registro> <dominio> [tag]}"
DOMINIO="${2:?uso: $0 <registro> <dominio> [tag]}"
RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TAG="${3:-$(sed -n 's/^  tag: "\(.*\)"$/\1/p' "$RAIZ/deploy/helm/charts/id/values.yaml" | head -1)}"
[ -n "$TAG" ] || { echo "No encuentro el tag por defecto del chart; pásalo como tercer argumento." >&2; exit 2; }
PLATAFORMA="${PLATAFORMA:-linux/amd64}"
COMMIT="$(git -C "$RAIZ" rev-parse --short HEAD 2>/dev/null || echo dev)"

construir() { # construir <repositorio> <dockerfile> <contexto> [args…]
  local repo="$1" df="$2" ctx="$3"; shift 3
  echo "▸ ${REGISTRO}/${repo}:${TAG}"
  docker buildx build --platform "$PLATAFORMA" --push \
    --build-arg "VERSION=${TAG}" --build-arg "COMMIT=${COMMIT}" "$@" \
    -t "${REGISTRO}/${repo}:${TAG}" -f "$RAIZ/$df" "$RAIZ/$ctx"
}

# Servicios Go: services/<svc>/Dockerfile → sytue-<svc>
for svc in ai analytics bases calendar cast contacts docs drive hub id integrations mail notes ocr pages rooms syerp syphotos; do
  construir "sytue-${svc}" "services/${svc}/Dockerfile" .
done
construir sytue-cast-transcoder services/cast/Dockerfile.transcoder .
for c in api builder controller; do
  construir "sytue-cloud-${c}" "services/cloud/Dockerfile.${c}" .
done

# Apps web (Astro): una imagen por app, compilada para el dominio público.
for app in account bases calcus calendar cast certipdf cloud-dashboard console contacts diapos docus drive erp mail notes pages photos portal rooms www; do
  construir "sytue-frontend-${app}" apps/frontend/Dockerfile apps/frontend --target runtime \
    --build-arg "APP=${app}" \
    --build-arg "PUBLIC_OIDC_ISSUER=https://id.${DOMINIO}" \
    --build-arg "PUBLIC_HUB_URL=https://hub.${DOMINIO}" \
    --build-arg "PUBLIC_SYTUE_HUB_BASE_URL=https://hub.${DOMINIO}" \
    --build-arg "PUBLIC_SYTUE_DRIVE_BASE_URL=https://drive.${DOMINIO}" \
    --build-arg "PUBLIC_SYTUE_DOCS_BASE_URL=https://docs.${DOMINIO}" \
    --build-arg "PUBLIC_SYTUE_CLOUD_API_URL=https://cloud.${DOMINIO}/api"
done
echo "✓ Imágenes publicadas en ${REGISTRO} con el tag ${TAG}."
