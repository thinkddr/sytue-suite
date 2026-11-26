#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Instala la CLI de Fyne (fyne.io/tools) compilada contra la MISMA Fyne que la app.
#
#   fyne-cli.sh <apps/fyne> <versión de fyne.io/tools>
#
# POR QUÉ NO BASTA `go install fyne.io/tools/cmd/fyne@vX`: así Go ignora el go.mod de
# la app y compila la CLI con la Fyne que pide fyne.io/tools —la 1.7.4 pide la 2.8.1,
# sin el fork—. Y no es solo la etiqueta: la CLI genera con SU copia de Fyne el
# envoltorio de iOS y Android (Info.plist, el arranque nativo), que es justo donde el
# fork pone las URL entrantes, los widgets y el despertar por push. release-desktop
# lo hacía así y empaquetaba con la v2.8.1 (2026-11-07).
#
# Se compila en un módulo de usar y tirar que copia los `replace` REMOTOS del go.mod de
# la app (el fork de Fyne, glfw y typesetting); los de ruta local son de la app y no
# hacen falta. Después se COMPRUEBA en el binario, con `go version -m`, que lleva el
# mismo fork que la app: un verde que no lo mire no demuestra nada.
#
# Respeta GOARCH/GOPATH del entorno (release-mobile la quiere de amd64) e imprime la
# carpeta del binario, para añadirla al PATH.
set -euo pipefail

app="$(cd "${1:?falta la carpeta de apps/fyne}" && pwd)"
version="${2:?falta la versión de fyne.io/tools}"
version="v${version#v}"

# `replace A => B vX` de una línea; el go.mod de la app no usa bloques `replace (`.
# Sin `mapfile`: en macOS (job de Apple, Xcode Cloud) el bash del sistema es el 3.2.
remotos="$(awk '$1 == "replace" && $3 == "=>" && NF == 5 {print $2 "=" $4 "@" $5}' "$app/go.mod")"
esperado="$(awk '$1 == "replace" && $2 == "fyne.io/fyne/v2" && $3 == "=>" {print $4 " " $5}' "$app/go.mod")"
[ -n "$esperado" ] || { echo "::error::$app/go.mod no reemplaza fyne.io/fyne/v2 por el fork" >&2; exit 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
(
  cd "$tmp"
  go mod init sytue/fyne-cli >/dev/null 2>&1
  printf '%s\n' "$remotos" | while read -r r; do go mod edit -replace="$r"; done
  go get "fyne.io/tools/cmd/fyne@$version" >/dev/null
  go install fyne.io/tools/cmd/fyne
)

bin="$(go env GOPATH)/bin"
[ "$(go env GOOS)/$(go env GOARCH)" = "$(go env GOHOSTOS)/$(go env GOHOSTARCH)" ] ||
  bin="$bin/$(go env GOOS)_$(go env GOARCH)"

# La Fyne que de verdad lleva dentro: la línea `=>` que sigue a `dep fyne.io/fyne/v2`.
lleva="$(go version -m "$bin/fyne" | awk 'prev == "fyne.io/fyne/v2" && $1 == "=>" {print $2 " " $3} {prev = $2}')"
[ "$lleva" = "$esperado" ] || {
  echo "::error::la CLI de Fyne se compiló con «${lleva:-fyne.io/fyne/v2 sin reemplazar}» y la app usa «$esperado»" >&2
  exit 1
}
echo "CLI fyne $version con $lleva" >&2
printf '%s\n' "$bin"
