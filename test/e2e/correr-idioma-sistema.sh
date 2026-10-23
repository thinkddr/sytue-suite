#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
exec bash "$(dirname "$0")/lib/correr-wizard-fixture.sh" rooms idioma-sistema "${SYSTEM_LANGUAGE_OUT:-/tmp/rooms-conexiones/idioma-sistema}" system-language.mjs SYSTEM_LANGUAGE
