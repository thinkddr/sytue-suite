#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Deja a git leyendo los forks PRIVADOS de los que depende apps/fyne
# —github.com/thinkddr/fyne, …/typesetting y …/glfw— y COMPRUEBA, en segundos, que
# el token puede leerlos.
#
#   SYTUE_GIT_FORKS_TOKEN=… .github/actions/toolchain/forks.sh
#
# POR QUÉ. Los forks son privados y el GITHUB_TOKEN de un job solo vale para
# sytue-one, así que hace falta un token que LEA esos tres repositorios (un token
# de grano fino de GitHub con «Contents: read» sobre ellos). Cuando el token no
# sirve, Go no falla al clonar: falla media hora después, al compilar, con un 403
# o un 404… y solo en el runner que no tenía el módulo en su caché (release-desktop
# v3.5.7 a v3.5.9, cuando los forks estaban en git.gay). Aquí se pide cada fork a la
# API con el token y se dice qué contestó GitHub y si el valor del secreto trae
# espacios o saltos de línea, que es como se pega un token y queda un 401 sin
# explicación.
set -euo pipefail

servidor="${SYTUE_FORKS_SERVER:-https://github.com}"
api="${SYTUE_FORKS_API:-https://api.github.com}"
dueno="${SYTUE_FORKS_OWNER:-thinkddr}"
token="${SYTUE_GIT_FORKS_TOKEN:-}"
if [ -z "$token" ]; then
  echo "::error::no hay token para los forks privados de Fyne: define el secreto SYTUE_GIT_FORKS_TOKEN (token de GitHub con Contents: read sobre $dueno/fyne, typesetting y glfw)" >&2
  exit 1
fi
limpio="$(printf '%s' "$token" | tr -d '[:space:]')"
if [ "$limpio" != "$token" ]; then
  echo "::error::SYTUE_GIT_FORKS_TOKEN lleva espacios o saltos de línea (${#token} caracteres, ${#limpio} sin ellos): vuelve a guardar el secreto sin ellos" >&2
  exit 1
fi
basic="$(printf 'x-access-token:%s' "$token" | base64 | tr -d '\n')"

for fork in fyne typesetting glfw; do
  cuerpo="$(curl -sS --max-time 30 -w '\n%{http_code}' -H "Authorization: Bearer $token" \
    -H 'Accept: application/vnd.github+json' "$api/repos/$dueno/$fork" 2>&1)" || cuerpo="$cuerpo"$'\n'000
  codigo="${cuerpo##*$'\n'}"
  case "$codigo" in
    200) ;;
    401) echo "::error::SYTUE_GIT_FORKS_TOKEN no es un token válido de GitHub (HTTP 401 en $dueno/$fork)" >&2; exit 1 ;;
    403|404) echo "::error::SYTUE_GIT_FORKS_TOKEN no puede leer $dueno/$fork (HTTP $codigo): el token necesita Contents: read sobre ese repositorio. Respuesta: ${cuerpo%$'\n'*}" >&2; exit 1 ;;
    *) echo "::error::$dueno/$fork contestó HTTP $codigo con SYTUE_GIT_FORKS_TOKEN: ${cuerpo%$'\n'*}" >&2; exit 1 ;;
  esac
  # `extraHeader` es una LISTA y se acumula por cada URL que casa: a `fyne.git` le
  # casan la entrada general de github.com (el token del job, que solo lee
  # sytue-one, si el clonado la dejó puesta) y esta, git manda las dos
  # `Authorization` y el servidor se queda con la primera: 404 aunque el token de
  # los forks sea bueno (ci-desktop 2652, cuando era git.gay). El valor vacío
  # VACÍA la lista para esta URL, y solo después va el token.
  clave="http.$servidor/$dueno/$fork.git/.extraHeader"
  git config --global --unset-all "$clave" 2>/dev/null || true
  git config --global --add "$clave" ""
  git config --global --add "$clave" "Authorization: Basic $basic"
  # Go pide el módulo sin `.git` en la URL: la misma cabecera para esa forma.
  clave="http.$servidor/$dueno/$fork/.extraHeader"
  git config --global --unset-all "$clave" 2>/dev/null || true
  git config --global --add "$clave" ""
  git config --global --add "$clave" "Authorization: Basic $basic"
  # Y la prueba de verdad, por el camino que usa Go (git por HTTPS), no solo la API.
  if ! git ls-remote -q "$servidor/$dueno/$fork.git" HEAD >/dev/null 2>/tmp/forks-$fork.err; then
    echo "::error::git no puede leer $dueno/$fork con SYTUE_GIT_FORKS_TOKEN aunque la API sí: $(tr '\n' ' ' </tmp/forks-$fork.err)" >&2
    exit 1
  fi
done
echo "forks de Fyne legibles con SYTUE_GIT_FORKS_TOKEN (${#token} caracteres), por API y por git"
