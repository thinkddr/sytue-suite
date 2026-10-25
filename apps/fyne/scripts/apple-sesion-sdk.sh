#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
# Compila los bindings públicos de Apple. NUNCA ejecuta el binario resultante,
# lee credenciales, accede al Keychain ni firma un producto.
set -euo pipefail
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RAIZ="$(cd "$AQUI/../../.." && pwd)"
fuente="$(mktemp -d "$RAIZ/apps/nucleo/.apple-sdk.XXXXXX")"
salida="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/sytue-apple-sdk.XXXXXX")"
trap 'rm -rf "$fuente" "$salida"' EXIT

cat >"$fuente/main.go" <<'GO'
package main

import "C"
import (
    "runtime"

    "git.sytue.eu/sytue/one/apps/nucleo/ajustes"
    "git.sytue.eu/sytue/one/apps/nucleo/auth"
    "git.sytue.eu/sytue/one/apps/nucleo/sistema"
    "git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

func main() {
    _, _ = auth.Load()
    _ = auth.Save(auth.Tokens{})
    _, _ = auth.LeerSecreto("ci-no-ejecutar")
    _ = auth.GuardarSecreto("ci-no-ejecutar", []byte{0})
    _ = auth.BorrarSecreto("ci-no-ejecutar")
    _, _ = ajustes.Carpeta()
    _ = ajustes.Leer()
    widgets.Notify("")
    // Referencias enlazadas: no se consulta el llavero ni se registra un dominio.
    // Este programa nunca se ejecuta, tampoco en el gate del SDK real.
    runtime.KeepAlive([]any{
        sistema.ArchivosDisponible,
        sistema.GuardarCredencialArchivos,
        sistema.HayCredencialArchivos,
        sistema.RegistrarArchivos,
        sistema.AbrirArchivos,
        sistema.OlvidarArchivos,
    })
}
GO

# Se prueban las funciones de enlace del empaquetador, no una copia de flags.
# shellcheck disable=SC1090
source <(sed -n '/^enlace_widgets_apple() {/,/^}/p; /^permiso_enlace_widgets_apple() {/,/^}/p; /^verificar_minimo_ios() {/,/^}/p' "$AQUI/empaquetar.sh")

verificar_puente() { # <ejecutable> <rpath esperado>
  local cargas rutas
  cargas="$(xcrun otool -L "$1")"
  grep -qE '^[[:space:]]*@rpath/SytueWidgetBridge\.framework/(Versions/A/)?SytueWidgetBridge \(' <<<"$cargas" || {
    echo '::error::el host Go no carga el puente público WidgetKit.' >&2; return 1; }
  rutas="$(xcrun otool -l "$1")"
  python3 - "$2" "$rutas" <<'PY'
import re, sys
if not re.search(r'^\s*path ' + re.escape(sys.argv[1]) + r' \(offset \d+\)$', sys.argv[2], re.M):
    sys.exit('::error::el host Go no puede resolver el puente WidgetKit')
PY
}

minimo() {
  local valor
  valor="$(sed -n "s/^$1 = \([0-9][0-9.]*\)$/\1/p" "$RAIZ/apps/fyne/xcode/Sytue.xcconfig")"
  [ -n "$valor" ] && [ "$(printf '%s\n' "$valor" | wc -l | tr -d ' ')" = 1 ] || {
    echo "::error::Sytue.xcconfig debe declarar un mínimo único para $1" >&2; return 1; }
  printf '%s\n' "$valor"
}

cd "$RAIZ/apps/nucleo"
for sdk in macosx iphoneos; do
  raiz_sdk="$(xcrun --sdk "$sdk" --show-sdk-path)"
  cc="$(xcrun --sdk "$sdk" -f clang)"
  cxx="$(xcrun --sdk "$sdk" -f clang++)"
  if [ "$sdk" = macosx ]; then
    objetivo=darwin
    minimo_os="$(minimo MACOSX_DEPLOYMENT_TARGET)"
    version="-mmacosx-version-min=$minimo_os"
    arquitecturas=(arm64 amd64)
  else
    objetivo=ios
    minimo_os="$(minimo IPHONEOS_DEPLOYMENT_TARGET)"
    version="-miphoneos-version-min=$minimo_os"
    arquitecturas=(arm64)
  fi
  widget_flags=""
  widget_permiso=""
  if [ -n "${SYTUE_WIDGET_PRODUCTOS_ROOT:-}" ]; then
    widget_plataforma=macos
    widget_rpath='@executable_path/../Frameworks'
    if [ "$objetivo" = ios ]; then widget_plataforma=ios; widget_rpath='@executable_path/Frameworks'; fi
    export SYTUE_WIDGET_PRODUCTOS="$SYTUE_WIDGET_PRODUCTOS_ROOT/$widget_plataforma/productos"
    widget_flags="$(enlace_widgets_apple "$widget_rpath")"
    widget_permiso="$(permiso_enlace_widgets_apple)"
  fi
  for arquitectura in "${arquitecturas[@]}"; do
    apple_arch="$arquitectura"
    [ "$arquitectura" != amd64 ] || apple_arch=x86_64
    flags="-isysroot \"$raiz_sdk\" $version -arch $apple_arch"
    for etiquetas in goolm tienda,goolm; do
      [ "$objetivo" != ios ] || [ "$etiquetas" = goolm ] || continue
      opciones=()
      [ "$objetivo" != ios ] || opciones=(-buildmode=c-archive)
      destino="$salida/$objetivo-$arquitectura-${etiquetas//,/-}"
      echo "Bindings Apple: $objetivo $arquitectura $etiquetas, mínimo $minimo_os (sólo compilar)"
      env GOOS="$objetivo" GOARCH="$arquitectura" CGO_ENABLED=1 CC="$cc" CXX="$cxx" \
        CGO_CFLAGS="$flags" CGO_CXXFLAGS="$flags" CGO_LDFLAGS="$flags${widget_flags:+ $widget_flags}" CGO_LDFLAGS_ALLOW="$widget_permiso" \
        go build -trimpath -tags "$etiquetas" ${opciones[@]+"${opciones[@]}"} -o "$destino" "$fuente"
      if [ -n "$widget_flags" ]; then
        # El archivo C de iOS comprueba bindings; el ejecutable aparte mide los
        # LC_LOAD/LC_RPATH que recibirá el host Fyne, sin lanzarlo.
        if [ "$objetivo" = ios ]; then
          destino="$destino-host"
          env GOOS=ios GOARCH=arm64 CGO_ENABLED=1 CC="$cc" CXX="$cxx" \
            CGO_CFLAGS="$flags" CGO_CXXFLAGS="$flags" CGO_LDFLAGS="$flags $widget_flags" CGO_LDFLAGS_ALLOW="$widget_permiso" \
            go build -trimpath -tags "$etiquetas" -o "$destino" "$fuente"
          verificar_minimo_ios "$destino" "$minimo_os"
        fi
        verificar_puente "$destino" "$widget_rpath"
      fi
    done
  done
done
