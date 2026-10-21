#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Remote CI only: real Console wizard with synthetic Gateway consent responses.
set -euo pipefail
cd "$(dirname "$0")"
exec bash lib/correr-wizard-fixture.sh console conectar-app "${CONSOLE_OUT:-/tmp/conectar-app}" conectar-app.mjs CONSOLE admin
