#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# `bun install --frozen-lockfile` con una segunda oportunidad y la caché limpia.
#
# POR QUÉ EXISTE. La descarga de un tarball falla de vez en cuando —red, o una
# entrada de caché escrita a medias— y bun lo deja escrito así:
#
#     error: Fail extracting tarball for "lighthouse"
#     error: failed to download lighthouse@12.6.1: Fail
#
# No se cura sola: la clave de la caché sale del hash del lockfile, así que no
# cambia hasta que alguien toca una dependencia. El 2026-08-10 costó cuatro
# ejecuciones seguidas en rojo con el mismo error, y el job `quality` de
# ci-frontend se llevó por eso un reintento escrito a mano. El de `lighthouse` no
# lo tenía y es el que ha vuelto a caer, dos veces, por el mismo sitio.
#
# QUÉ CACHÉ BORRA. La que bun usa DE VERDAD, que es `$BUN_INSTALL_CACHE_DIR`
# cuando está puesta —ci-frontend la fija por job para que dos instalaciones a la
# vez en el mismo runner no se pisen los tarballs a medio extraer— y
# `~/.bun/install/cache` cuando no. Borrar la de por defecto teniendo la variable
# puesta es lo que hacía el reintento de `quality`: dejaba intacta la única que
# importaba y reintentaba contra la misma entrada rota.
#
# Uso: scripts/bun-install-reintento.sh [directorio]
set -uo pipefail

cd "${1:-.}" || exit 1

if bun install --frozen-lockfile; then
  exit 0
fi

cache="${BUN_INSTALL_CACHE_DIR:-$HOME/.bun/install/cache}"
echo "::warning::bun install falló en $PWD; se descarta la caché ($cache) y se reintenta desde cero"
rm -rf "$cache"
bun install --frozen-lockfile
