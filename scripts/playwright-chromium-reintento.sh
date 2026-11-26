#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# `playwright install --with-deps chromium` con reintentos.
#
# POR QUÉ EXISTE. `--with-deps` instala con apt las bibliotecas que necesita
# Chromium, y apt baja los índices de deb.debian.org. El 2026-11-25 (ci-frontend de
# v3.27.3, ejecución 45712055771) el runner no llegó a conectar con ese espejo
# («connection timed out»): apt no encontró ningún paquete, Chromium no se instaló
# y las cinco pruebas de navegador de `accesibilidad-office` cayeron sin haber
# arrancado («Looks like Playwright was just installed»). Ningún fallo del código;
# la release, en rojo igual.
#
# QUÉ HACE. Pide a apt que reintente cada descarga (Acquire::Retries, solo si el
# job es root y hay apt: el contenedor de CI) y repite la instalación entera hasta
# cuatro veces, con espera creciente. Si la cuarta también falla, sale con su
# código: un espejo caído de verdad sigue siendo rojo, con lo que contestó.
#
# Uso: scripts/playwright-chromium-reintento.sh [directorio con node_modules]
set -uo pipefail

cd "${1:-.}" || exit 1

if [ "$(id -u)" = 0 ] && [ -d /etc/apt/apt.conf.d ]; then
  printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\n' >/etc/apt/apt.conf.d/80-sytue-reintentos
fi

intento=1
while true; do
  ./node_modules/.bin/playwright install --with-deps chromium && exit 0
  codigo=$?
  [ "$intento" -lt 4 ] || exit "$codigo"
  echo "::warning::playwright install --with-deps chromium falló (intento $intento de 4); se reintenta en $((intento * 20)) s"
  sleep $((intento * 20))
  intento=$((intento + 1))
done
