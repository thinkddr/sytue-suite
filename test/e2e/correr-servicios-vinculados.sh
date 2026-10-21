#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail
cd "$(dirname "$0")"
exec bash lib/correr-wizard-fixture.sh calendar servicios-vinculados "${LINKED_OUT:-/tmp/servicios-vinculados}" servicios-vinculados.mjs LINKED
