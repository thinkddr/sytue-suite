#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
# Compilación mínima real de Cgo: se ejecuta en CI, nunca en el portátil.
set -euo pipefail
AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
temporal="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/sytue-cgo-link.XXXXXX")"
trap 'rm -rf "$temporal"' EXIT
# shellcheck disable=SC1090
source <(sed -n '/^enlace_widgets_apple() {/,/^}/p; /^permiso_enlace_widgets_apple() {/,/^}/p' "$AQUI/empaquetar.sh")
printf 'module sytue.test/cgo-link\n\ngo 1.27.0\n' >"$temporal/go.mod"
printf 'package main\nimport "C"\nfunc main() {}\n' >"$temporal/main.go"
cd "$temporal"
export GOWORK=off GOTOOLCHAIN=local CGO_ENABLED=1 GOFLAGS='' CGO_LDFLAGS_ALLOW=''
for ruta in '@executable_path/Frameworks' '@executable_path/../Frameworks'; do
  CGO_LDFLAGS="-Wl,-rpath,$ruta" go build -o "$temporal/host" .
done
mkdir -p "$temporal/SytueWidgetBridge.framework"
export SYTUE_WIDGET_PRODUCTOS="$temporal"
# Linux no enlaza frameworks Apple, pero -L utiliza el mismo parser de Go que
# -F. Se mide la ruta real del helper (con espacios) sin ejecutar el resultado.
mkdir -p "$temporal/productos con espacios/SytueWidgetBridge.framework"
SYTUE_WIDGET_PRODUCTOS="$temporal/productos con espacios"
banderas="$(enlace_widgets_apple '@executable_path/Frameworks')"
busqueda="${banderas%% -Wl,*}"
CGO_LDFLAGS="${busqueda/-F/-L}" go build -o "$temporal/host" .
for ruta in '@otro_path/Frameworks' '@executable_path/../../Frameworks'; do
  if enlace_widgets_apple "$ruta" >"$temporal/resultado.log" 2>&1; then
    echo "not ok - el empaquetador aceptó una ruta no autorizada: $ruta" >&2; exit 1
  fi
done
# Las fuentes #cgo atraviesan también el filtro que Darwin aplica a las flags
# que conserva en go:cgo_ldflag. Se prueba la excepción finita con el Go real.
fuente() { printf 'package main\n/*\n#cgo LDFLAGS: -Wl,-rpath,%s\n*/\nimport "C"\nfunc main() {}\n' "$1" > main.go; }
rechazar() {
  if go build -o "$temporal/host" . >"$temporal/resultado.log" 2>&1; then
    echo 'not ok - se aceptó una flag de fuente Cgo fuera del permiso finito' >&2; exit 1
  fi
  grep -q 'invalid flag in #cgo LDFLAGS' "$temporal/resultado.log" || { cat "$temporal/resultado.log" >&2; exit 1; }
}
fuente '@executable_path/Frameworks'
rechazar
CGO_LDFLAGS_ALLOW="$(permiso_enlace_widgets_apple)"
export CGO_LDFLAGS_ALLOW
for ruta in '@executable_path/Frameworks' '@executable_path/../Frameworks'; do
  fuente "$ruta"
  go build -o "$temporal/host" .
done
fuente '@otro_path/Frameworks'
rechazar
fuente '@executable_path/../../Frameworks'
rechazar
# GNU ld no conoce frameworks. Este adaptador de prueba retira sólo la flag
# exacta después del filtro real de Go; cualquier otra sigue siendo rechazada.
cc_real="$(command -v cc)"
cat >"$temporal/cc-framework" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
args=()
for arg in "$@"; do
  if [ "$arg" = '-Wl,-needed_framework,SytueWidgetBridge' ]; then
    printf '%s\n' "$arg" >>"$CGO_FRAMEWORK_REGISTRO"
  else
    args+=("$arg")
  fi
done
exec "$CGO_FRAMEWORK_CC" "${args[@]}"
SH
chmod +x "$temporal/cc-framework"
export CC="$temporal/cc-framework" CGO_FRAMEWORK_CC="$cc_real" CGO_FRAMEWORK_REGISTRO="$temporal/framework.log"
printf 'package main\n/*\n#cgo LDFLAGS: -Wl,-needed_framework,SytueWidgetBridge\n*/\nimport "C"\nfunc main() {}\n' >main.go
go build -o "$temporal/host" .
grep -qxF -- '-Wl,-needed_framework,SytueWidgetBridge' "$CGO_FRAMEWORK_REGISTRO"
sed -i.bak 's/SytueWidgetBridge/OtherFramework/' main.go
rechazar
echo 'ok - Cgo real sólo permite los dos RPATH canónicos y conserva nuestro único framework'
