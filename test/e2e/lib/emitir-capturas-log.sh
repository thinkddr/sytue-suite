#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Storage-quota fallback. Only synthetic wizard screenshots/reports enter the log.
set -euo pipefail
NOMBRE="${1:?wizard name required}"
case "$NOMBRE" in
  almacenamiento-externo|rooms-conexiones|conectar-app) ;;
  *) printf 'Unknown fixture: %s\n' "$NOMBRE" >&2; exit 1 ;;
esac
DIRECTORIO="/tmp/$NOMBRE"
[[ -d "$DIRECTORIO" ]] || exit 0
TEMPORAL="$(mktemp -d)"
trap 'rm -rf "$TEMPORAL"' EXIT
# Deliberately exclude astro.log, environment/configuration files and symlinks.
(cd "$DIRECTORIO" && find . -maxdepth 1 -type f \( -name '*.png' -o -name report.json -o -name results.json \) -print0) >"$TEMPORAL/files"
[[ -s "$TEMPORAL/files" ]] || exit 0
tar -C "$DIRECTORIO" --null -T "$TEMPORAL/files" -czf "$TEMPORAL/capturas.tar.gz"
printf '::group::Wizard screenshots fallback — %s\n' "$NOMBRE"
printf 'SYTUE_WIZARD_ARCHIVE_SHA256 %s %s\n' "$NOMBRE" "$(sha256sum "$TEMPORAL/capturas.tar.gz" | cut -d' ' -f1)"
printf 'SYTUE_WIZARD_ARCHIVE_BEGIN %s\n' "$NOMBRE"
base64 "$TEMPORAL/capturas.tar.gz" | tr -d '\n' | fold -w 100
printf '\nSYTUE_WIZARD_ARCHIVE_END %s\n' "$NOMBRE"
printf '::endgroup::\n'
