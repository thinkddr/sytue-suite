#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Runs on CI only. The fixture uses real Drive UI with mocked provider/API responses.
set -euo pipefail
cd "$(dirname "$0")"
exec bash lib/correr-wizard-fixture.sh drive almacenamiento "${STORAGE_OUT:-/tmp/almacenamiento-externo}" almacenamiento-externo.mjs STORAGE
