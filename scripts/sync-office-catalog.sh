#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Fuentes únicas de apps/nucleo/office/ y su copia para el build web:
#   catalog.json   -> packages/config/src/office-catalog.json         (descargas)
#   comandos.json  -> packages/office/src/editor/comandos.json              (registro de comandos de la cinta)
#   comandos.json  -> services/docs/internal/domain/comandos.json        (qué comandos admite un script; el servicio lo embebe)
#   pestanas.json  -> packages/office/src/editor/pestanas.json              (taxonomía de pestañas de la cinta)
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
pares=(
  "apps/nucleo/office/catalog.json:apps/frontend/packages/config/src/office-catalog.json"
  "apps/nucleo/office/comandos.json:apps/frontend/packages/office/src/editor/comandos.json"
  "apps/nucleo/office/comandos.json:services/docs/internal/domain/comandos.json"
  "apps/nucleo/office/pestanas.json:apps/frontend/packages/office/src/editor/pestanas.json"
)
modo="${1:-}"
case "$modo" in ''|--check) ;; *) echo 'Usage: sync-office-catalog.sh [--check]' >&2; exit 2 ;; esac
for par in "${pares[@]}"; do
  source_file="$root/${par%%:*}"
  target_file="$root/${par##*:}"
  if [ "$modo" = --check ]; then
    if ! cmp -s "$source_file" "$target_file"; then
      echo "Office catalog is out of sync (${par##*:}); run bash scripts/sync-office-catalog.sh" >&2
      exit 1
    fi
  else
    cp "$source_file" "$target_file"
  fi
done
