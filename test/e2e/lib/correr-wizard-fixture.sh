#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Shared remote-CI runner for development fixtures of real application wizards.
set -euo pipefail
cd "$(dirname "$0")/.."
APLICACION="${1:?application required}"
RUTA="${2:?fixture route required}"
SALIDA="${3:?artifact directory required}"
PRUEBA="${4:?browser script required}"
PREFIJO="${5:?environment prefix required}"
BASE_RUTA="${6:-$APLICACION}"
PUERTO="${PUERTO:-4343}"
RAIZ="$(cd ../.. && pwd)"
mkdir -p "$SALIDA"
LOG="$SALIDA/astro.log"
export SYTUE_OIDC_ISSUER="${SYTUE_OIDC_ISSUER:-https://id.prueba.invalid}"
export SYTUE_OIDC_CLIENT_ID="${SYTUE_OIDC_CLIENT_ID:-prueba}"
export SYTUE_OIDC_REDIRECT_URI="${SYTUE_OIDC_REDIRECT_URI:-https://$BASE_RUTA.prueba.invalid/$BASE_RUTA/auth/callback}"
export SYTUE_OIDC_CLIENT_SECRET="${SYTUE_OIDC_CLIENT_SECRET:-prueba}"
export SYTUE_SESSION_SECRET="${SYTUE_SESSION_SECRET:-kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk}"

# The development toolbar is outside the product and can cover mobile footer controls.
# Match correr-accesibilidad.sh: change only the ephemeral project's Astro preference.
(cd "$RAIZ/apps/frontend/apps/$APLICACION" && bun --bun astro preferences disable devToolbar)

(cd "$RAIZ/apps/frontend/apps/$APLICACION" && exec bun --bun astro dev --port "$PUERTO" --host 127.0.0.1) >"$LOG" 2>&1 &
SERVIDOR=$!
trap 'kill "$SERVIDOR" 2>/dev/null || true; wait "$SERVIDOR" 2>/dev/null || true' EXIT
URL="http://127.0.0.1:$PUERTO/$BASE_RUTA/prueba/dev/$RUTA"
LISTO=0
for _ in $(seq 1 120); do
  if curl -fsS -o /dev/null "$URL"; then LISTO=1; break; fi
  kill -0 "$SERVIDOR" 2>/dev/null || { tail -60 "$LOG"; exit 1; }
  sleep 2
done
if [[ "$LISTO" != 1 ]]; then tail -60 "$LOG"; exit 1; fi
export "${PREFIJO}_URL=$URL" "${PREFIJO}_OUT=$SALIDA"
bun run "$PRUEBA"
