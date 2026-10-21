#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
# Materialize the maintained upstream SDK plus a reviewed codec framing patch.
# Never modify shared GOMODCACHE or vendor crypto into the product source tree.
set -euo pipefail

AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RAIZ="$(cd "$AQUI/../../.." && pwd)"
# shellcheck source=/dev/null
. "$RAIZ/deploy/sytue-git/versions.env"
version="${LIVEKIT_GO_VERSION:?falta LIVEKIT_GO_VERSION}"
sha="${LIVEKIT_GO_SOURCE_SHA256:?falta LIVEKIT_GO_SOURCE_SHA256}"
parche="$AQUI/../patches/livekit-server-sdk-go-v${version}-vp8.patch"
destino="$RAIZ/.deps/livekit-server-sdk-go"
[ -f "$parche" ] || { echo "::error::falta el parche VP8 de LiveKit v${version}" >&2; exit 1; }
grep -q "github.com/livekit/server-sdk-go/v2 v${version}$" "$AQUI/../go.mod" || {
  echo "::error::LiveKit go.mod y versions.env no coinciden; no se aplica otro parche." >&2
  exit 1
}
command -v patch >/dev/null || { echo '::error::falta patch para preparar LiveKit' >&2; exit 1; }
command -v unzip >/dev/null || { echo '::error::falta unzip para preparar LiveKit' >&2; exit 1; }
# A source fingerprint makes repeated setup cheap while rejecting edited caches.
if command -v sha256sum >/dev/null; then hash=(sha256sum); else hash=(shasum -a 256); fi
huella="$("${hash[@]}" "$parche" | cut -d ' ' -f1)"
if [ -f "$destino/.sytue-source" ] && [ "$(cat "$destino/.sytue-source")" = "$sha $huella" ]; then
  (cd "$destino" && "${hash[@]}" -c .sytue-manifest >/dev/null) && exit 0
  echo '::error::la fuente LiveKit preparada ha cambiado; elimina solo .deps/livekit-server-sdk-go y repite.' >&2
  exit 1
fi
mkdir -p "$RAIZ/.deps"
tmp="$(mktemp -d "$RAIZ/.deps/livekit-source.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
bash "$RAIZ/deploy/sytue-git/bin/bajar-herramienta.sh" livekit-server-sdk-go "$version" "v${version}.zip" "$sha" \
  "https://proxy.golang.org/github.com/livekit/server-sdk-go/v2/@v/v${version}.zip" "$tmp/source.zip"
unzip -q "$tmp/source.zip" -d "$tmp/source"
fuente="$tmp/source/github.com/livekit/server-sdk-go/v2@v${version}"
[ -f "$fuente/go.mod" ] || { echo '::error::el archivo LiveKit no contiene el módulo fijado' >&2; exit 1; }
(cd "$fuente" && patch --batch --forward --fuzz=0 -p1 < "$parche")
sed -i.bak \
  -e 's|github.com/google/cel-go v0\.[0-9.]*|github.com/google/cel-go v0.31.0|' \
  -e 's|golang.org/x/net v0\.[0-9.]*|golang.org/x/net v0.61.0|' \
  "$fuente/go.mod"
rm -f "$fuente/go.mod.bak"
printf '%s %s\n' "$sha" "$huella" > "$fuente/.sytue-source"
(cd "$fuente" && find . -type f ! -name .sytue-manifest -print | LC_ALL=C sort | while IFS= read -r archivo; do "${hash[@]}" "$archivo"; done > .sytue-manifest)
# Replace only this generated dependency, never a tracked checkout or module cache.
if [ -e "$destino" ] && [ ! -f "$destino/.sytue-source" ]; then
  echo '::error::.deps/livekit-server-sdk-go no es una fuente generada de Sytue' >&2
  exit 1
fi
rm -rf "$destino"
mv "$fuente" "$destino"
echo "LiveKit Go v${version}: fuente verificada y parche VP8 aplicado."
