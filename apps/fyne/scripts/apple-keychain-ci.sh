#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
# Existing signing helpers own certificates, profiles and Keychain cleanup. This
# CI-only orchestration never creates Apple resources or publishes an app.
set -euo pipefail
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RAIZ="$(cd "$AQUI/../../.." && pwd)"
cd "$RAIZ"
: "${RUNNER_TEMP:?CI runner required}"
: "${GITHUB_ENV:?CI environment required}"
: "${SYTUE_OPENSSL:?verified OpenSSL 3 required}"
[ "$(uname -s)" = Darwin ] || exit 1
cleanup() {
  if [ -n "${SYTUE_APPLE_SIGNING_STATE:-}" ]; then
    python3 "$AQUI/apple_llavero.py" limpiar
  fi
}
trap cleanup EXIT
creacion="$(python3 "$AQUI/apple_llavero.py" crear)"
SYTUE_APPLE_JOB_DIR="$(printf '%s' "$creacion" | python3 -c 'import json,sys; print(json.load(sys.stdin)["jobDir"])')"
export SYTUE_APPLE_JOB_DIR
export SYTUE_APPLE_SIGNING_STATE="$SYTUE_APPLE_JOB_DIR/estado.json"
export SYTUE_APPLE_KEYCHAIN_HELPER="$AQUI/apple_llavero.py"
export SYTUE_PERFILES_DIR="$SYTUE_APPLE_JOB_DIR/perfiles-apple"
CODEMAGIC_CLI_TOOLS_VERSION="$(sed -n 's/^CODEMAGIC_CLI_TOOLS_VERSION=//p' deploy/sytue-git/versions.env)"
export CODEMAGIC_CLI_TOOLS_VERSION
wrapper="$SYTUE_APPLE_JOB_DIR/wrappers"
mkdir -m 700 "$wrapper"
install -m 700 .github/actions/apple/xcrun-existing-only.sh "$wrapper/xcrun"
for herramienta in codesign productsign productbuild; do
  install -m 700 .github/actions/apple/sign-existing-only.sh "$wrapper/$herramienta"
done
install -m 700 .github/actions/apple/security-existing-only.sh "$wrapper/security"
export SYTUE_EXISTING_SIGNING_WRAPPER="$wrapper"
bash .github/actions/apple/instalar-cli.sh "$SYTUE_APPLE_JOB_DIR/cm"
export PATH="$wrapper:$SYTUE_APPLE_JOB_DIR/cm/bin:$PATH"
SYTUE_KEYCHAIN_QA_IDENTITY="$(bash "$AQUI/apple-firma.sh" preparar macos)"
export SYTUE_KEYCHAIN_QA_IDENTITY
python3 "$AQUI/apple-perfiles.py" --plataforma developerid --destinos hosts \
  --solo-existentes --preparar --salida "$SYTUE_PERFILES_DIR" hub rooms
bash "$AQUI/apple-keychain-test.sh"
