#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Remote CI only: actual Rooms UI with synthetic OAuth and API responses.
set -euo pipefail
cd "$(dirname "$0")"
bash lib/correr-wizard-fixture.sh rooms conexiones "${ROOMS_OUT:-/tmp/rooms-conexiones}" rooms-conexiones.mjs ROOMS

exec bash lib/correr-wizard-fixture.sh rooms conexiones "${ROOMS_OUT:-/tmp/rooms-conexiones}" rooms-canales-externos.mjs ROOMS
