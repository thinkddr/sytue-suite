#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Real Account layout, BFF session and CSP; only the plan and Drive quota are synthetic.
set -euo pipefail
cd "$(dirname "$0")"
exec bash lib/correr-wizard-fixture.sh account almacenamiento "${ACCOUNT_CAPACITY_OUT:-${STORAGE_OUT:-/tmp/almacenamiento-externo}/account}" account-capacity.mjs ACCOUNT_CAPACITY account ''
