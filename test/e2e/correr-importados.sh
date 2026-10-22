#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Pinta los modelos importados (importados.mjs) con el banco de pruebas de Docus servido por
# `astro dev`, y lo apaga al acabar. Para el contenedor de los agentes del frontal:
#
#   cd test/e2e && IMPORTADOS_DIR=/ruta/modelos IMPORTADOS_OUT=/ruta/capturas ./correr-importados.sh
#
# Los modelos los deja TestCorpusTerceros con SYTUE_CORPUS_MODELOS (services/docs).
set -euo pipefail
cd "$(dirname "$0")"
PUERTO="${PUERTO:-4351}"
RAIZ="$(cd ../.. && pwd)"
LOG="${TMPDIR:-/tmp}/astro-dev-importados-$PUERTO.log"

# El middleware construye su cliente OIDC al arrancar y se niega sin estas variables, aunque el
# banco de pruebas no inicia sesión: valores de mentira, no se llama a ningún IdP.
export SYTUE_OIDC_ISSUER="${SYTUE_OIDC_ISSUER:-https://id.prueba.invalid}"
export SYTUE_OIDC_CLIENT_ID="${SYTUE_OIDC_CLIENT_ID:-prueba}"
export SYTUE_OIDC_REDIRECT_URI="${SYTUE_OIDC_REDIRECT_URI:-https://docus.prueba.invalid/auth/callback}"
export SYTUE_OIDC_CLIENT_SECRET="${SYTUE_OIDC_CLIENT_SECRET:-prueba}"
export SYTUE_SESSION_SECRET="${SYTUE_SESSION_SECRET:-$(head -c 40 /dev/zero | tr '\0' 'k')}"

(cd "$RAIZ/apps/frontend/apps/docus" && exec bun --bun astro dev --port "$PUERTO" --host 127.0.0.1) >"$LOG" 2>&1 &
SERVIDOR=$!
trap 'kill "$SERVIDOR" 2>/dev/null; wait "$SERVIDOR" 2>/dev/null' EXIT

for _ in $(seq 1 120); do
  curl -fsS -o /dev/null "http://127.0.0.1:$PUERTO/docus/prueba/dev/editor?demo=1" 2>/dev/null && break
  kill -0 "$SERVIDOR" 2>/dev/null || { echo "el servidor de Docus se cerró:"; tail -30 "$LOG"; exit 1; }
  sleep 2
done

IMPORTADOS_URL="http://127.0.0.1:$PUERTO/docus/prueba" bun run importados.mjs
