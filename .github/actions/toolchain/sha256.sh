#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
# Portable artifact verification, shared by toolchain and native source patches.
comprobar_sha256() {  # <sha256> <fichero>
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s  %s\n' "$1" "$2" | sha256sum -c - >/dev/null
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s  %s\n' "$1" "$2" | shasum -a 256 -c - >/dev/null
  else
    echo "✗ ni sha256sum ni shasum en el PATH: no hay con qué verificar la descarga." >&2
    return 1
  fi
}
