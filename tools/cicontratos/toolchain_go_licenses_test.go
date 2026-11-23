// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// toolchainGoLicensesGuion carga el instalador real de go-licenses de accion.sh
// y lo ejercita con un `go` y un `anunciar` sintéticos (funciones de shell: ni
// compila ni toca la red ni /usr/local/bin). $1 = la versión que dejaría
// instalada `go install`; vacío = la del pin. Al terminar bien imprime cuántas
// instalaciones hubo, qué versión quedó y qué carpeta se anunció al PATH.
const toolchainGoLicensesGuion = `set -euo pipefail
funcion="$(sed -n '/^instalar_go_licenses() {$/,/^}$/p' .github/actions/toolchain/accion.sh)"
[ -n "$funcion" ] || { echo 'falta el instalador real de go-licenses' >&2; exit 97; }
eval "$funcion"
GO_LICENSES_VERSION=9.8.7
VERSION_LICENCIAS=1.6.0
INSTALACIONES_LICENCIAS=0
ANUNCIO_LICENCIAS=
[ -z "$1" ] || INSTALADA_LICENCIAS="$1"
go() {
  case "$*" in
    "install github.com/google/go-licenses/v2@v${GO_LICENSES_VERSION}")
      [ "$GOBIN" = /usr/local/bin ] || return 1
      INSTALACIONES_LICENCIAS=$((INSTALACIONES_LICENCIAS + 1))
      VERSION_LICENCIAS="${INSTALADA_LICENCIAS:-$GO_LICENSES_VERSION}"
      ;;
    'version -m /usr/local/bin/go-licenses')
      printf 'mod\tgithub.com/google/go-licenses/v2\tv%s\th1:fixture\n' "$VERSION_LICENCIAS"
      ;;
    *) return 1 ;;
  esac
}
anunciar() { ANUNCIO_LICENCIAS="$1"; }
instalar_go_licenses
printf 'instalaciones=%s version=%s anuncio=%s\n' "$INSTALACIONES_LICENCIAS" "$VERSION_LICENCIAS" "$ANUNCIO_LICENCIAS"
`

func toolchainGoLicensesInstalar(t *testing.T, instalada string) guiontest.Resultado {
	t.Helper()
	r := guiontest.Bash(t, guiontest.Opciones{}, toolchainGoLicensesGuion, instalada)
	if r.Codigo == 97 {
		t.Fatal("falta el instalador real de go-licenses")
	}
	return r
}

func TestToolchainGoLicensesSustituyeElModuloAntiguoAnunciaElPATHYValidaElPin(t *testing.T) {
	t.Run("security usa el instalador compartido", func(t *testing.T) {
		if !strings.Contains(guiontest.Leer(t, ".github/workflows/security.yml"), "accion.sh go-licenses") {
			t.Fatal("security no usa el instalador compartido de go-licenses")
		}
	})
	t.Run("instala el pin en /usr/local/bin y lo anuncia", func(t *testing.T) {
		r := toolchainGoLicensesInstalar(t, "")
		if r.Codigo != 0 || r.Texto() != "instalaciones=1 version=9.8.7 anuncio=/usr/local/bin" {
			t.Fatalf("puede sobrevivir una versión antigua o sombrear el binario nuevo (código %d): %s", r.Codigo, r.Todo())
		}
	})
	t.Run("rechaza un binario instalado con versión incorrecta", func(t *testing.T) {
		r := toolchainGoLicensesInstalar(t, "1.6.0")
		if r.Codigo == 0 {
			t.Fatal("se aceptó un binario instalado con versión incorrecta")
		}
		if !strings.Contains(r.Todo(), "instalado no coincide") {
			t.Fatalf("la instalación incorrecta falló por otro motivo: %s", r.Todo())
		}
	})
}
