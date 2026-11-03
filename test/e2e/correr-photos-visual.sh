#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Ejecutar sólo en el runner remoto, con Bun y Chromium de Playwright instalados.
set -euo pipefail
exec "$(dirname "$0")/lib/correr-wizard-fixture.sh" \
  photos app "${PHOTOS_VISUAL_OUT:-/tmp/photos-visual-${GITHUB_RUN_ID:-manual}}" \
  photos-visual.mjs PHOTOS_VISUAL
