#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Lanza el barrido de desbordamiento EN SCALEWAY y espera su resultado.
#
# POR QUÉ EXISTE: la regla de la casa es que toda prueba que ejecute algo corre
# en la nube, y Playwright con nombre y apellidos —lo que decide es dónde arranca
# el Chromium, no contra qué URL apunta—. La vía correcta ya existía (el workflow
# `mobile.yml` corre en runners de Scaleway y admite disparo manual), pero usarla
# eran cinco pasos —buscar el token, componer el POST, localizar el run, seguirlo,
# leer el resultado— mientras que saltársela era uno: `./auditar-prod.sh docs`.
#
# Una regla que se cumple con cinco pasos y se incumple con uno se incumple. Así
# que esto iguala el coste: un comando, la misma interfaz que el guion local, y
# el resultado por pantalla.
#
#   ./auditar-en-cloud.sh                # todas las apps, perfiles de móvil
#   ./auditar-en-cloud.sh docs mail      # solo esas
#   ./auditar-en-cloud.sh --escritorio   # las VENTANAS de las apps de escritorio
#   ./auditar-en-cloud.sh --no-esperar   # dispara y devuelve la URL
#
# `--escritorio` dispara escritorio.yml en vez de mobile.yml: mismas apps, pero
# medidas a 880×600 —el mínimo al que se puede encoger la ventana de Wails— y con
# la comprobación vertical encendida. Está aquí y no en un guion aparte por lo
# mismo que dice el párrafo de arriba: si lanzarlo cuesta más que lanzar el otro,
# no se lanza.
#
# La forja es GitHub desde el 2026-09-27 (antes, git.gay): el disparo y el
# seguimiento van con `gh`, con la sesión que ya tiene quien lo lanza.
set -euo pipefail

REPO="${SYTUE_GIT_REPO:-thinkddr/sytue-one}"
RAMA="${SYTUE_MOBILE_REF:-main}"
ESPERAR=1
FLUJO=mobile.yml
QUE="Barrido móvil"
APPS=()

for arg in "$@"; do
  case "$arg" in
    --no-esperar) ESPERAR=0 ;;
    --escritorio) FLUJO=escritorio.yml; QUE="Barrido de ventana de escritorio" ;;
    -h|--help) sed -n '5,30p' "$0"; exit 0 ;;
    *) APPS+=("$arg") ;;
  esac
done
command -v gh >/dev/null || { echo "✗ falta gh (brew install gh) con sesión en GitHub" >&2; exit 1; }

LISTA="$(IFS=' '; echo "${APPS[*]:-}")"
echo "$QUE en el CI — rama $RAMA${LISTA:+, apps: $LISTA}"

antes="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
if [ -n "$LISTA" ]; then
  gh workflow run "$FLUJO" -R "$REPO" --ref "$RAMA" -f apps="$LISTA"
else
  gh workflow run "$FLUJO" -R "$REPO" --ref "$RAMA"
fi
echo "  ✓ lanzado (el Chromium corre en el runner, no aquí)"

if [ "$ESPERAR" -eq 0 ]; then
  echo "  runs: https://github.com/$REPO/actions/workflows/$FLUJO"
  exit 0
fi

# El disparo no devuelve el id del run: se busca el primero de ese flujo creado
# después de lanzarlo (preguntar demasiado pronto devolvería el ANTERIOR).
RUN_ID=""
for _ in $(seq 1 15); do
  RUN_ID="$(gh run list -R "$REPO" --workflow "$FLUJO" --event workflow_dispatch -L 5 \
    --json databaseId,createdAt --jq "[.[] | select(.createdAt >= \"$antes\")][0].databaseId // empty")"
  [ -n "$RUN_ID" ] && break
  sleep 4
done
if [ -z "$RUN_ID" ]; then
  echo "  · no encontré el run todavía; míralo en https://github.com/$REPO/actions" >&2
  exit 0
fi
echo "  run $RUN_ID — https://github.com/$REPO/actions/runs/$RUN_ID"
if gh run watch "$RUN_ID" -R "$REPO" --exit-status --interval 20 >/dev/null; then
  echo "  ✓ sin desbordamientos"
else
  echo "  ✗ el barrido encontró desbordamientos (o falló): https://github.com/$REPO/actions/runs/$RUN_ID" >&2
  exit 1
fi
