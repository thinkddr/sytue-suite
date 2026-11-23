// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Ejercita el instalador real con binarios y go sintéticos: cambiar el pin debe
// sustituir una versión preinstalada distinta, sin compilar ni acceder a la red.

// toolchainGovulncheckGuion carga instalar_govulncheck de accion.sh con un PATH
// que solo ve el binario de mentira. $1 = versión preinstalada (o «ausente»);
// $2 = la versión que dejaría `go install` (vacío = la del pin). El `go` de
// mentira apunta cada instalación en $GOVULN_FIXTURE/install y `anunciar`, la
// carpeta anunciada en $GOVULN_FIXTURE/path.
const toolchainGovulncheckGuion = `set -euo pipefail
funcion="$(sed -n '/^instalar_govulncheck() {$/,/^}$/p' .github/actions/toolchain/accion.sh)"
[ -n "$funcion" ] || { echo 'falta el instalador real' >&2; exit 97; }
eval "$funcion"
mkdir -p "$GOVULN_FIXTURE/bin"
export PATH="$GOVULN_FIXTURE/bin:/usr/bin:/bin"
GOVULNCHECK_VERSION=9.8.7
[ -z "$2" ] || VERSION_INSTALADA="$2"

binario() {
  printf '%s\n' "$1" >"$GOVULN_FIXTURE/version"
  cat >"$GOVULN_FIXTURE/bin/govulncheck" <<'BINARIO'
#!/usr/bin/env bash
printf 'Go: go-fixture\nScanner: govulncheck@v%s\n' "$(cat "$GOVULN_FIXTURE/version")"
BINARIO
  chmod +x "$GOVULN_FIXTURE/bin/govulncheck"
}
go() {
  [ "$*" = "install golang.org/x/vuln/cmd/govulncheck@v${GOVULNCHECK_VERSION}" ] || return 1
  printf '%s\n' "$*" >>"$GOVULN_FIXTURE/install"
  binario "${VERSION_INSTALADA:-$GOVULNCHECK_VERSION}"
}
anunciar() {
  [ "$1" = /usr/local/bin ] || return 1
  printf '%s\n' "$1" >>"$GOVULN_FIXTURE/path"
}

[ "$1" = ausente ] || binario "$1"
instalar_govulncheck
`

// toolchainGovulncheckInstalar corre el instalador sobre un directorio nuevo y
// devuelve cómo salió, cuántas instalaciones hizo y lo que anunció al PATH.
func toolchainGovulncheckInstalar(t *testing.T, preinstalada, instalada string) (r guiontest.Resultado, instalaciones int, anuncio string) {
	t.Helper()
	dir := t.TempDir()
	r = guiontest.Bash(t, guiontest.Opciones{Env: []string{"GOVULN_FIXTURE=" + dir}},
		toolchainGovulncheckGuion, preinstalada, instalada)
	if r.Codigo == 97 {
		t.Fatal("falta el instalador real")
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "install")); err == nil { //nolint:gosec // G304: ruta dentro del directorio temporal de la prueba.
		instalaciones = strings.Count(string(raw), "\n")
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "path")); err == nil { //nolint:gosec // G304: ruta dentro del directorio temporal de la prueba.
		anuncio = strings.TrimRight(string(raw), "\n")
	}
	return r, instalaciones, anuncio
}

func TestToolchainGovulncheckRespetaElPinYReemplazaVersionesDistintas(t *testing.T) {
	const pin = "9.8.7"
	for _, c := range []struct {
		version  string
		esperado int
	}{
		{pin, 0},
		{"1.7.0", 1},
		{pin + "0", 1},
		{"desconocida", 1},
		{"ausente", 1},
	} {
		t.Run(c.version, func(t *testing.T) {
			r, instalaciones, anuncio := toolchainGovulncheckInstalar(t, c.version, "")
			if r.Codigo != 0 {
				t.Fatalf("el instalador rechazó %s: %s", c.version, r.Todo())
			}
			if instalaciones != c.esperado {
				t.Fatalf("%s realizó %d instalaciones, se esperaban %d", c.version, instalaciones, c.esperado)
			}
			if c.esperado == 1 && anuncio != "/usr/local/bin" {
				t.Fatalf("%s: se anunció %q al PATH y no /usr/local/bin", c.version, anuncio)
			}
		})
	}
}

func TestToolchainGovulncheckVerificaLaInstalacion(t *testing.T) {
	r, _, _ := toolchainGovulncheckInstalar(t, "desconocida", "1.7.0")
	if r.Codigo == 0 {
		t.Fatal("se aceptó una instalación que no coincide con el pin")
	}
	if !strings.Contains(r.Todo(), "instalado no coincide") {
		t.Fatalf("la instalación incorrecta falló por otro motivo: %s", r.Todo())
	}
}
