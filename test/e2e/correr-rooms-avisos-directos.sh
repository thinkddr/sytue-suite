#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
exec bash "$(dirname "$0")/lib/correr-wizard-fixture.sh" rooms avisos-directos "${DM_ALERTS_OUT:-/tmp/rooms-conexiones/avisos-directos}" rooms-dm-notifications.mjs DM_ALERTS
