#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
# Real signed macOS Keychain IO. Only random synthetic account names; no app
# installation, LaunchServices registration, distribution or production reads.
set -euo pipefail
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RAIZ="$(cd "$AQUI/../../.." && pwd)"
: "${RUNNER_TEMP:?CI runner required}"
: "${SYTUE_KEYCHAIN_QA_IDENTITY:?prepared Developer ID identity required}"
: "${SYTUE_PERFILES_DIR:?prepared existing profiles required}"
[ "$(uname -s)" = Darwin ] || { echo '::error::signed Keychain QA requires macOS'; exit 1; }
qa="$(mktemp -d "$RUNNER_TEMP/sytue-keychain.XXXXXXXX")"
nonce="$(openssl rand -hex 32)"
llavero="$(bash "$AQUI/apple-firma.sh" llavero-efimero)"
cleanup(){
  for app in "$qa/a.app" "$qa/b.app"; do
    [ ! -x "$app/Contents/MacOS/probe" ] || "$app/Contents/MacOS/probe" clean "$nonce" >/dev/null 2>&1 || true
  done
  rm -rf "$qa"
}
trap cleanup EXIT
cd "$RAIZ/apps/nucleo"
go build -tags tienda,apple_keychain_qa,goolm -trimpath -o "$qa/probe" ./auth/testdata/apple-keychain
for pair in a:hub b:rooms; do
 slot="${pair%%:*}";product="${pair#*:}";bundle="$qa/$slot.app"
 mkdir -p "$bundle/Contents/MacOS"
 cp "$qa/probe" "$bundle/Contents/MacOS/probe"
 python3 - "$RAIZ/apps/fyne/cmd/$product/FyneApp.toml" "$bundle/Contents/Info.plist" <<'PY'
import pathlib,plistlib,sys,tomllib
metadata=tomllib.loads(pathlib.Path(sys.argv[1]).read_text())
with open(sys.argv[2],'wb') as out:
 plistlib.dump({'CFBundleIdentifier':metadata['Details']['ID'],'CFBundleExecutable':'probe',
  'CFBundleName':'Sytue Keychain QA','CFBundlePackageType':'APPL',
  'CFBundleURLTypes':[{'CFBundleURLSchemes':[metadata['Development']['esquema']]}]},out)
PY
 appid="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$bundle/Contents/Info.plist")"
 team="$(sed -n 's/^const GrupoSuite = "\([^.]*\)\..*/\1/p' "$RAIZ/apps/nucleo/ajustes/grupo_apple.go")"
 bash "$AQUI/empaquetar.sh" derechos-sesion "$product" "$team" developerid >"$qa/$slot.plist"
 perfil="$(bash "$AQUI/apple-firma.sh" perfil-ruta "$appid" developerid)"
 cp "$perfil" "$bundle/Contents/embedded.provisionprofile"
 codesign --force --options runtime --timestamp=none --keychain "$llavero" \
  --sign "$SYTUE_KEYCHAIN_QA_IDENTITY" --entitlements "$qa/$slot.plist" "$bundle"
 codesign --verify --strict "$bundle"
done
"$qa/a.app/Contents/MacOS/probe" seed-a "$nonce"
"$qa/b.app/Contents/MacOS/probe" check-b "$nonce"
"$qa/a.app/Contents/MacOS/probe" migrate-a "$nonce"
"$qa/b.app/Contents/MacOS/probe" after-b "$nonce"
echo 'KEYCHAIN_QA_REAL_SIGNED_TWO_APPS_PASS'
