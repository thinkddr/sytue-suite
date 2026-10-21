#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Levanta el servidor de desarrollo de Sytue Bases y pasa la auditoría de accesibilidad de
# la rejilla (`accesibilidad-bases.mjs`: axe, teclado, zoom y táctil). Se lanza en el
# contenedor de los agentes, nunca en el portátil.
#
# Requisitos: `bun install` en apps/frontend y en test/e2e, y Chromium de Playwright.
# Variables: PUERTO (4337), ACC_OUT para las capturas. Sale con 1 si algo falla.
set -uo pipefail
cd "$(dirname "$0")"
PUERTO="${PUERTO:-4337}"
RAIZ="$(cd ../.. && pwd)"
LOG="${TMPDIR:-/tmp}/astro-dev-bases-$PUERTO.log"

# El middleware de Bases construye su cliente OIDC al arrancar y se niega sin estas
# variables, aunque el banco de pruebas (`/dev/rejilla`) no inicia sesión. Valores de
# mentira: no se llama a ningún IdP.
export SYTUE_OIDC_ISSUER="${SYTUE_OIDC_ISSUER:-https://id.prueba.invalid}"
export SYTUE_OIDC_CLIENT_ID="${SYTUE_OIDC_CLIENT_ID:-prueba}"
export SYTUE_OIDC_REDIRECT_URI="${SYTUE_OIDC_REDIRECT_URI:-https://bases.prueba.invalid/auth/callback}"
export SYTUE_OIDC_CLIENT_SECRET="${SYTUE_OIDC_CLIENT_SECRET:-prueba}"
export SYTUE_SESSION_SECRET="${SYTUE_SESSION_SECRET:-$(head -c 40 /dev/zero | tr '\0' 'k')}"

(cd "$RAIZ/apps/frontend/apps/bases" && exec bun --bun astro dev --port "$PUERTO" --host 127.0.0.1) >"$LOG" 2>&1 &
SERVIDOR=$!
trap 'kill "$SERVIDOR" 2>/dev/null; wait "$SERVIDOR" 2>/dev/null' EXIT

for _ in $(seq 1 120); do
  curl -fsS -o /dev/null "http://127.0.0.1:$PUERTO/bases/dev/rejilla" 2>/dev/null && break
  kill -0 "$SERVIDOR" 2>/dev/null || { echo "el servidor de Bases se cerró:"; tail -30 "$LOG"; exit 1; }
  sleep 2
done

export ACC_URL="http://127.0.0.1:$PUERTO/bases"
bun run accesibilidad-bases.mjs
