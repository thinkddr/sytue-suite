// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// fyne-cli.sh compila la CLI de Fyne con el fork de la app y FALLA si el binario no lo
// lleva. Con un `go` de mentira: ni red ni compilación, solo lo que el guion le pide y
// lo que `go version -m` contestaría.
//
//   - copia al módulo temporal los `replace` remotos del go.mod, no los de ruta local;
//   - instala la versión pedida de fyne.io/tools;
//   - sale en verde si el binario lleva el fork, en rojo si lleva la Fyne de fyne.io/tools;
//   - con GOARCH cruzado devuelve la carpeta bin/<os>_<arch>.

const fyneCLIGoMod = `module git.sytue.eu/sytue/one/apps/fyne

go 1.27.4

require fyne.io/fyne/v2 v2.8.2

replace github.com/thinkddr/astro-fyne/native => ../../tools/astro-fyne/native

replace fyne.io/fyne/v2 => github.com/thinkddr/fyne/v2 v2.8.2-sytue.8

replace github.com/go-text/typesetting => github.com/thinkddr/typesetting v0.3.5-sytue.1
`

// El `go` de mentira apunta lo que se le pide y contesta a `go env` y a
// `go version -m` con el fork que diga MOCK_FORK (vacío: sin reemplazar).
const fyneCLIGo = `printf 'go %s\n' "$*" >>"$MOCK_LOG"
case "$1" in
  env)
    case "$2" in
      GOPATH) echo "$MOCK_GOPATH" ;;
      GOOS | GOHOSTOS) echo linux ;;
      GOARCH) echo "${GOARCH:-arm64}" ;;
      GOHOSTARCH) echo arm64 ;;
    esac ;;
  version)
    printf '%s: go1.27.4\n\tpath\tfyne.io/tools/cmd/fyne\n\tdep\tfyne.io/fyne/v2\tv2.8.1\th1:x\n' "$3"
    [ -z "$MOCK_FORK" ] || printf '\t=>\t%s\th1:y\n' "$MOCK_FORK"
    printf '\tdep\tgithub.com/go-text/typesetting\tv0.3.0\th1:z\n' ;;
esac
exit 0`

const fyneCLIFork = "github.com/thinkddr/fyne/v2 v2.8.2-sytue.8"

// fyneCLICorrer lanza fyne-cli.sh con el `go` de mentira y el fork que llevaría
// el binario. Devuelve el resultado, el registro de lo pedido a `go` y el GOPATH.
// GOARCH se fija siempre (vacío = sin cruzar) para que el del entorno no cuente.
func fyneCLICorrer(t *testing.T, fork, goarch string) (r guiontest.Resultado, registro, gopath string) {
	t.Helper()
	tmp := t.TempDir()
	app := filepath.Join(tmp, "app")
	bin := filepath.Join(tmp, "bin")
	gopath = filepath.Join(tmp, "gopath")
	log := filepath.Join(tmp, "log")
	guiontest.Escribir(t, filepath.Join(app, "go.mod"), fyneCLIGoMod)
	guiontest.Escribir(t, filepath.Join(gopath, "bin", ".vacio"), "")
	guiontest.Escribir(t, log, "")
	guiontest.Doble(t, bin, "go", fyneCLIGo)
	r = guiontest.Guion(t, guiontest.Opciones{Env: []string{
		guiontest.ConPATH(bin), "MOCK_LOG=" + log, "MOCK_GOPATH=" + gopath, "MOCK_FORK=" + fork, "GOARCH=" + goarch,
	}}, ".github/actions/toolchain/fyne-cli.sh", app, "1.7.4")
	return r, guiontest.Leer(t, log), gopath
}

// fyneCLILinea dice si el registro tiene esa línea exacta (lo que hacía `grep -qx`).
func fyneCLILinea(registro, linea string) bool {
	return regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(linea) + `$`).MatchString(registro)
}

// 1. El binario lleva el fork: verde, y lo que pidió es lo que toca.
func TestFyneCLIConElForkDentro(t *testing.T) {
	r, registro, gopath := fyneCLICorrer(t, fyneCLIFork, "")
	t.Run("con el fork dentro sale en verde", func(t *testing.T) {
		if r.Codigo != 0 {
			t.Fatalf("con el fork dentro salió en rojo: %s", r.Error)
		}
	})
	t.Run("devuelve la carpeta del binario", func(t *testing.T) {
		if quiero := filepath.Join(gopath, "bin"); r.Texto() != quiero {
			t.Fatalf("devolvió «%s» y no %s", r.Texto(), quiero)
		}
	})
	t.Run("copia los replace remotos", func(t *testing.T) {
		if !fyneCLILinea(registro, "go mod edit -replace=fyne.io/fyne/v2=github.com/thinkddr/fyne/v2@v2.8.2-sytue.8") ||
			!fyneCLILinea(registro, "go mod edit -replace=github.com/go-text/typesetting=github.com/thinkddr/typesetting@v0.3.5-sytue.1") {
			t.Fatalf("no copió los replace remotos: %s", registro)
		}
	})
	t.Run("deja fuera los replace de ruta local", func(t *testing.T) {
		if strings.Contains(registro, "astro-fyne") {
			t.Fatalf("copió un replace de ruta local: %s", registro)
		}
	})
	t.Run("instala la versión pedida (con o sin v)", func(t *testing.T) {
		if !fyneCLILinea(registro, "go get fyne.io/tools/cmd/fyne@v1.7.4") {
			t.Fatalf("no pidió fyne.io/tools@v1.7.4: %s", registro)
		}
	})
}

// 2. El binario lleva la Fyne de fyne.io/tools (lo que hacía `go install …@v`): rojo.
func TestFyneCLISinElForkSaleEnRojoDiciendoCualEspera(t *testing.T) {
	r, _, _ := fyneCLICorrer(t, "", "")
	if r.Codigo == 0 {
		t.Fatal("sin el fork dentro salió en verde")
	}
	if !strings.Contains(r.Error, "v2.8.2-sytue.8") {
		t.Fatalf("salió en rojo sin decir qué fork espera: %s", r.Error)
	}
}

// 3. Otro fork que no es el de la app: también rojo.
func TestFyneCLIConUnForkDistintoDelDeLaAppSaleEnRojo(t *testing.T) {
	if r, _, _ := fyneCLICorrer(t, "github.com/thinkddr/fyne/v2 v2.8.2-sytue.4", ""); r.Codigo == 0 {
		t.Fatal("con un fork distinto salió en verde")
	}
}

// 4. GOARCH cruzado (release-mobile la quiere de amd64): bin/<os>_<arch>.
func TestFyneCLIConGOARCHCruzadoDevuelveBinLinuxAmd64(t *testing.T) {
	r, _, gopath := fyneCLICorrer(t, fyneCLIFork, "amd64")
	if quiero := filepath.Join(gopath, "bin", "linux_amd64"); r.Texto() != quiero {
		t.Fatalf("con GOARCH=amd64 devolvió «%s»", r.Texto())
	}
}
