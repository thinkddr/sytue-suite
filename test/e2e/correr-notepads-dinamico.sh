#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# La pantalla real con BFF sintético, solo en el entorno remoto de CI.
set -euo pipefail
cd "$(dirname "$0")"
exec bash lib/correr-wizard-fixture.sh notes app "${NOTEPADS_UX_OUT:-/tmp/notepads-ux}" notepads-dinamico.mjs NOTEPADS_UX
