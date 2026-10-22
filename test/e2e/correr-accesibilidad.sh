#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Levanta el servidor de desarrollo de Sytue Docus (que aloja los bancos de pruebas de la
# ofimática, también el de CertiPDF) y pasa las auditorías de la cinta y del
# editor: `cinta.mjs` (maquetación y teclado de la cinta), `paridad-cinta-web.mjs` (qué comandos
# del registro común no tienen control) y `accesibilidad-office.mjs` (axe,
# teclado, zoom y táctil de Docus, Calcus, Diapos y CertiPDF) y `dibujo.mjs` (el bloque «Dibujo»
# de Docus en un navegador real) y `superposiciones.mjs` (Modal/Popover móviles,
# sin recortes y con sus acciones alcanzables, también el compositor de Rooms). La usa el CI (ci-frontend.yml,
# job `accesibilidad-office`) y se puede lanzar igual en el contenedor de los agentes.
#
# Requisitos: `bun install` en apps/frontend y en test/e2e, y Chromium de Playwright.
# Variables: PUERTO (4338), ACC_SOLO=docus,calcus,... para acotar, ACC_OUT y CINTA_OUT para
# las capturas. Sale con 1 si cualquiera falla.
set -uo pipefail
cd "$(dirname "$0")"
PUERTO="${PUERTO:-4338}"
RAIZ="$(cd ../.. && pwd)"
LOG="${TMPDIR:-/tmp}/astro-dev-docus-$PUERTO.log"

# El middleware de la ofimática construye su cliente OIDC al arrancar y se niega sin estas variables,
# aunque los bancos de pruebas (`/dev/*`) no inician sesión. Valores de mentira: no se llama
# a ningún IdP. El secreto de sesión se arma aquí, no se escribe.
export SYTUE_OIDC_ISSUER="${SYTUE_OIDC_ISSUER:-https://id.prueba.invalid}"
export SYTUE_OIDC_CLIENT_ID="${SYTUE_OIDC_CLIENT_ID:-prueba}"
export SYTUE_OIDC_REDIRECT_URI="${SYTUE_OIDC_REDIRECT_URI:-https://docus.prueba.invalid/auth/callback}"
export SYTUE_OIDC_CLIENT_SECRET="${SYTUE_OIDC_CLIENT_SECRET:-prueba}"
export SYTUE_SESSION_SECRET="${SYTUE_SESSION_SECRET:-$(head -c 40 /dev/zero | tr '\0' 'k')}"

# La barra de desarrollo no pertenece al producto ni a sus auditorías. Astro
# puede dejar obsoleto su entrypoint durante la optimización de dependencias.
# La preferencia solo afecta al proyecto efímero servido por esta prueba.
(cd "$RAIZ/apps/frontend/apps/docus" && bun --bun astro preferences disable devToolbar) || exit 1

(cd "$RAIZ/apps/frontend/apps/docus" && exec bun --bun astro dev --port "$PUERTO" --host 127.0.0.1) >"$LOG" 2>&1 &
SERVIDOR=$!
trap 'kill "$SERVIDOR" 2>/dev/null; wait "$SERVIDOR" 2>/dev/null' EXIT

for _ in $(seq 1 120); do
  curl -fsS -o /dev/null "http://127.0.0.1:$PUERTO/docus/prueba/dev/cinta" 2>/dev/null && break
  kill -0 "$SERVIDOR" 2>/dev/null || { echo "el servidor de Docus se cerró:"; tail -30 "$LOG"; exit 1; }
  sleep 2
done

export CINTA_URL="http://127.0.0.1:$PUERTO/docus/prueba"
export ACC_URL="$CINTA_URL"
estado=0
echo "=== cinta.mjs ==="
bun run cinta.mjs || estado=1
echo "=== paridad-cinta-web.mjs ==="
bun run paridad-cinta-web.mjs || estado=1
echo "=== accesibilidad-office.mjs ==="
bun run accesibilidad-office.mjs || estado=1
echo "=== dibujo.mjs ==="
bun run dibujo.mjs || estado=1
echo "=== superposiciones.mjs ==="
bun run superposiciones.mjs || estado=1
if [ "$estado" -ne 0 ]; then
  echo "=== diagnóstico del servidor de Docus ==="
  tail -100 "$LOG"
fi
exit "$estado"
