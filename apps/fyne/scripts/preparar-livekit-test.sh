#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
# Run only in remote CI/ephemeral development, like the native build itself.
set -euo pipefail
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RAIZ="$(cd "$AQUI/../../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/apps/fyne/scripts" "$tmp/apps/fyne/patches" "$tmp/deploy/sytue-git/bin" "$tmp/.github/actions/toolchain"
cp "$AQUI/preparar-livekit.sh" "$tmp/apps/fyne/scripts/"
cp "$AQUI/../patches/"*.patch "$tmp/apps/fyne/patches/"
cp "$RAIZ/deploy/sytue-git/versions.env" "$tmp/deploy/sytue-git/"
cp "$RAIZ/deploy/sytue-git/bin/"{bajar-herramienta,descarga-reanudable}.sh "$tmp/deploy/sytue-git/bin/"
cp "$RAIZ/.github/actions/toolchain/sha256.sh" "$tmp/.github/actions/toolchain/"
cp "$AQUI/../go.mod" "$tmp/apps/fyne/go.mod"
preparar="$tmp/apps/fyne/scripts/preparar-livekit.sh"
bash "$preparar" > "$tmp/output" 2>&1
bash "$preparar" >> "$tmp/output" 2>&1
grep -q CodecVP8 "$tmp/.deps/livekit-server-sdk-go/e2ee.go"
# A source edit must fail instead of silently blessing a changed cache.
printf '\n// modified fixture source\n' >> "$tmp/.deps/livekit-server-sdk-go/e2ee.go"
if bash "$preparar" >> "$tmp/output" 2>&1; then echo 'accepted edited SDK cache' >&2; exit 1; fi
rm -rf "$tmp/.deps"
# A different module version must fail before downloading or patching.
sed 's#server-sdk-go/v2 v[0-9.]*#server-sdk-go/v2 v0.0.0#' "$AQUI/../go.mod" > "$tmp/apps/fyne/go.mod"
if bash "$preparar" >> "$tmp/output" 2>&1; then echo 'accepted a different module version' >&2; exit 1; fi
cp "$AQUI/../go.mod" "$tmp/apps/fyne/go.mod"
# Corrupt removed upstream text: zero-fuzz application must reject it.
parche="$(find "$tmp/apps/fyne/patches" -name '*.patch' -print -quit)"
sed 's/unencrypted_audio_bytes/missing_fixture_header/g' "$parche" > "$tmp/broken.patch"
mv "$tmp/broken.patch" "$parche"
if bash "$preparar" >> "$tmp/output" 2>&1; then echo 'accepted a patch with wrong upstream contents' >&2; exit 1; fi
# Keep origin checks active even when the patch is correct.
cp "$AQUI/../patches/"*.patch "$tmp/apps/fyne/patches/"
sed 's/^LIVEKIT_GO_SOURCE_SHA256=.*/LIVEKIT_GO_SOURCE_SHA256=0000000000000000000000000000000000000000000000000000000000000000/' "$RAIZ/deploy/sytue-git/versions.env" > "$tmp/deploy/sytue-git/versions.env"
if bash "$preparar" >> "$tmp/output" 2>&1; then echo 'accepted incorrect upstream SHA' >&2; exit 1; fi
echo 'ok - verified SDK patch, repeat setup, edited cache, version, patch contents and SHA guards'
