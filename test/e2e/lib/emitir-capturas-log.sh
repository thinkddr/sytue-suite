#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Storage-quota fallback. Only synthetic fixture screenshots/reports enter the log.
set -euo pipefail
NOMBRE="${1:?wizard name required}"
case "$NOMBRE" in
  almacenamiento-externo|rooms-conexiones|conectar-app|correo-externo) ;;
  cinta-*) [[ "$NOMBRE" =~ ^cinta(-nativa)?-[0-9]+$ ]] || exit 1 ;;
  rotacion-nativa-*) [[ "$NOMBRE" =~ ^rotacion-nativa-[0-9]+$ ]] || exit 1 ;;
  *) printf 'Unknown fixture: %s\n' "$NOMBRE" >&2; exit 1 ;;
esac
DIRECTORIO="/tmp/$NOMBRE"
[[ -d "$DIRECTORIO" ]] || exit 0
TEMPORAL="$(mktemp -d)"
trap 'rm -rf "$TEMPORAL"' EXIT
# Deliberately exclude astro.log, environment/configuration files and symlinks.
if [[ "$NOMBRE" == rotacion-nativa-* ]]; then
  (cd "$DIRECTORIO" && find . -maxdepth 1 -type f -name 'native-rotacion-*.png' -print0) >"$TEMPORAL/files"
elif [[ "$NOMBRE" == cinta-* ]]; then
  # Native touch widths or Spanish portrait/landscape web states per editor.
  (cd "$DIRECTORIO" && find . -maxdepth 1 -regextype posix-extended -type f \( -name 'native-*.png' -o -regex './(document|spreadsheet|presentation)-es-(320|844|1024)(-derecha|-popover)?\.png' \) -print0) >"$TEMPORAL/files"
else
  (cd "$DIRECTORIO" && find . -maxdepth 1 -type f \( -name '*.png' -o -name report.json -o -name results.json \) -print0) >"$TEMPORAL/files"
fi
[[ -s "$TEMPORAL/files" ]] || exit 0
tar -C "$DIRECTORIO" --null -T "$TEMPORAL/files" -czf "$TEMPORAL/capturas.tar.gz"
printf '::group::Wizard screenshots fallback — %s\n' "$NOMBRE"
printf 'SYTUE_WIZARD_ARCHIVE_SHA256 %s %s\n' "$NOMBRE" "$(sha256sum "$TEMPORAL/capturas.tar.gz" | cut -d' ' -f1)"
printf 'SYTUE_WIZARD_ARCHIVE_BEGIN %s\n' "$NOMBRE"
base64 "$TEMPORAL/capturas.tar.gz" | tr -d '\n' | fold -w 100
printf '\nSYTUE_WIZARD_ARCHIVE_END %s\n' "$NOMBRE"
printf '::endgroup::\n'
