#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail
cd "$(dirname "$0")"
exec bash lib/correr-wizard-fixture.sh mail correo-externo "${MAIL_EXTERNAL_OUT:-/tmp/correo-externo}" correo-externo.mjs MAIL_EXTERNAL
