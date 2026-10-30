#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Empaqueta los bundles de los marketplaces de los vendors
# (integraciones-marketplace/) en dist-marketplace/<vendor>.zip, listos para
# subir a cada vendor y para publicar como assets de una release de GitHub
# (release-marketplace.yml). Idempotente: rehace los zip desde cero.
#
# Los iconos del paquete de Teams se renderizan con ImageMagick desde la marca
# (branding/marks), como los de las apps de escritorio: no se fabrican PNG a
# mano para que diverjan del logo.
set -euo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ORIGEN="$RAIZ/integraciones-marketplace"
SALIDA="$RAIZ/dist-marketplace"

command -v zip >/dev/null 2>&1 || { echo "::error::falta zip" >&2; exit 1; }

rm -rf "$SALIDA"; mkdir -p "$SALIDA"
trabajo="$(mktemp -d)"; trap 'rm -rf "$trabajo"' EXIT

# Teams: el paquete exige manifest + color.png + outline.png en la raíz del zip.
if command -v magick >/dev/null 2>&1; then
  magick -background none "$RAIZ/branding/marks/one.svg" -resize 192x192 "$trabajo/color.png"
  magick -background none "$RAIZ/branding/marks/one.svg" -resize 32x32 "$trabajo/outline.png"
else
  echo "::error::falta ImageMagick 7 (magick): Teams exige ambos iconos PNG." >&2
  exit 1
fi
cp "$ORIGEN/microsoft-teams/manifest.json" "$trabajo/"
(cd "$trabajo" && zip -q -r "$SALIDA/microsoft-teams.zip" .)
bash "$RAIZ/.github/actions/toolchain/preparar-teams.sh" "$SALIDA/microsoft-teams.zip"
echo "✓ dist-marketplace/microsoft-teams.zip"

# El resto: un zip por directorio, con el contenido en la raíz.
for dir in slack google-drive onedrive zoom google-meet google-chat; do
  (cd "$ORIGEN/$dir" && zip -q -r "$SALIDA/$dir.zip" .)
  echo "✓ dist-marketplace/$dir.zip"
done
