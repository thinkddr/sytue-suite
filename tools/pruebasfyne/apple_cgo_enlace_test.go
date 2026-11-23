// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Traducción de apps/fyne/scripts/apple-cgo-enlace-test.sh: compilación mínima
// REAL de Cgo con el Go y el cc del sistema. Las banderas salen de las
// funciones de enlace de empaquetar.sh (enlace_widgets_apple y
// permiso_enlace_widgets_apple), no de una copia: el Go real tiene que aceptar
// solo los dos RPATH canónicos y conservar nuestro único framework.

const appleCgoFunciones = `source <(sed -n '/^enlace_widgets_apple() {/,/^}/p; /^permiso_enlace_widgets_apple() {/,/^}/p' apps/fyne/scripts/empaquetar.sh)
`

// GNU ld no conoce frameworks. Este adaptador de prueba retira solo la flag
// exacta después del filtro real de Go; cualquier otra sigue siendo rechazada.
const appleCgoCC = `set -euo pipefail
args=()
for arg in "$@"; do
  if [ "$arg" = '-Wl,-needed_framework,SytueWidgetBridge' ]; then
    printf '%s\n' "$arg" >>"$CGO_FRAMEWORK_REGISTRO"
  else
    args+=("$arg")
  fi
done
exec "$CGO_FRAMEWORK_CC" "${args[@]}"`

type appleCgo struct {
	temporal string
	env      []string
}

func (c *appleCgo) construir(t *testing.T, env ...string) guiontest.Resultado {
	t.Helper()
	return guiontest.Ejecutar(t, guiontest.Opciones{Dir: c.temporal, Env: append(append([]string{}, c.env...), env...)},
		"go", "build", "-o", filepath.Join(c.temporal, "host"), ".")
}

func (c *appleCgo) debeConstruir(t *testing.T, que string, env ...string) {
	t.Helper()
	if r := c.construir(t, env...); r.Codigo != 0 {
		t.Fatalf("%s: go build falló (código %d)\n%s", que, r.Codigo, r.Todo())
	}
}

// rechazar: el Go real rechaza la flag de la fuente Cgo con su filtro.
func (c *appleCgo) rechazar(t *testing.T, que string) {
	t.Helper()
	r := c.construir(t)
	if r.Codigo == 0 {
		t.Fatalf("not ok - %s: se aceptó una flag de fuente Cgo fuera del permiso finito", que)
	}
	if !strings.Contains(r.Todo(), "invalid flag in #cgo LDFLAGS") {
		t.Fatalf("%s: falló por otro motivo\n%s", que, r.Todo())
	}
}

func (c *appleCgo) fuente(t *testing.T, ruta string) {
	t.Helper()
	guiontest.Escribir(t, filepath.Join(c.temporal, "main.go"),
		"package main\n/*\n#cgo LDFLAGS: -Wl,-rpath,"+ruta+"\n*/\nimport \"C\"\nfunc main() {}\n")
}

func appleCgoFuncion(t *testing.T, llamada string, env ...string) guiontest.Resultado {
	t.Helper()
	return guiontest.Bash(t, guiontest.Opciones{Env: env}, appleCgoFunciones+llamada)
}

func TestAppleCgoEnlaceSoloPermiteLosDosRPATHCanonicosYConservaNuestroUnicoFramework(t *testing.T) {
	temporal := t.TempDir()
	c := &appleCgo{temporal: temporal, env: []string{"GOWORK=off", "GOTOOLCHAIN=local", "CGO_ENABLED=1", "GOFLAGS=", "CGO_LDFLAGS_ALLOW="}}
	guiontest.Escribir(t, filepath.Join(temporal, "go.mod"), "module sytue.test/cgo-link\n\ngo 1.27.0\n")
	guiontest.Escribir(t, filepath.Join(temporal, "main.go"), "package main\nimport \"C\"\nfunc main() {}\n")
	canonicas := []string{"@executable_path/Frameworks", "@executable_path/../Frameworks"}

	t.Run("CGO_LDFLAGS con los RPATH canónicos", func(t *testing.T) {
		for _, ruta := range canonicas {
			c.debeConstruir(t, ruta, "CGO_LDFLAGS=-Wl,-rpath,"+ruta)
		}
	})

	// Linux no enlaza frameworks Apple, pero -L utiliza el mismo parser de Go
	// que -F. Se mide la ruta real del helper (con espacios) sin ejecutar el
	// resultado.
	productos := filepath.Join(temporal, "productos con espacios")
	for _, d := range []string{filepath.Join(temporal, "SytueWidgetBridge.framework"), filepath.Join(productos, "SytueWidgetBridge.framework")} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	conProductos := "SYTUE_WIDGET_PRODUCTOS=" + productos

	t.Run("la búsqueda del helper con espacios pasa el parser de Go", func(t *testing.T) {
		r := appleCgoFuncion(t, `enlace_widgets_apple '@executable_path/Frameworks'`, conProductos)
		if r.Codigo != 0 {
			t.Fatalf("enlace_widgets_apple: código %d\n%s", r.Codigo, r.Error)
		}
		busqueda, _, _ := strings.Cut(r.Texto(), " -Wl,")
		c.debeConstruir(t, "búsqueda", "CGO_LDFLAGS="+strings.Replace(busqueda, "-F", "-L", 1))
	})

	t.Run("el empaquetador rechaza rutas no autorizadas", func(t *testing.T) {
		for _, ruta := range []string{"@otro_path/Frameworks", "@executable_path/../../Frameworks"} {
			if r := guiontest.Bash(t, guiontest.Opciones{Env: []string{conProductos}}, appleCgoFunciones+`enlace_widgets_apple "$1"`, ruta); r.Codigo == 0 {
				t.Errorf("not ok - el empaquetador aceptó una ruta no autorizada: %s", ruta)
			}
		}
	})

	// Las fuentes #cgo atraviesan también el filtro que Darwin aplica a las
	// flags que conserva en go:cgo_ldflag. Se prueba la excepción finita con el
	// Go real.
	t.Run("sin permiso, la fuente Cgo con RPATH se rechaza", func(t *testing.T) {
		c.fuente(t, "@executable_path/Frameworks")
		c.rechazar(t, "sin CGO_LDFLAGS_ALLOW")
	})

	r := appleCgoFuncion(t, `permiso_enlace_widgets_apple`)
	if r.Codigo != 0 {
		t.Fatalf("permiso_enlace_widgets_apple: código %d\n%s", r.Codigo, r.Error)
	}
	c.env = append(c.env, "CGO_LDFLAGS_ALLOW="+r.Texto())

	t.Run("con el permiso finito, solo los RPATH canónicos", func(t *testing.T) {
		for _, ruta := range canonicas {
			c.fuente(t, ruta)
			c.debeConstruir(t, ruta)
		}
		c.fuente(t, "@otro_path/Frameworks")
		c.rechazar(t, "@otro_path/Frameworks")
		c.fuente(t, "@executable_path/../../Frameworks")
		c.rechazar(t, "@executable_path/../../Frameworks")
	})

	t.Run("se conserva nuestro único framework y ningún otro", func(t *testing.T) {
		cc, err := exec.LookPath("cc")
		if err != nil {
			t.Fatal(err)
		}
		registro := filepath.Join(temporal, "framework.log")
		c.env = append(c.env, "CC="+guiontest.Doble(t, temporal, "cc-framework", appleCgoCC),
			"CGO_FRAMEWORK_CC="+cc, "CGO_FRAMEWORK_REGISTRO="+registro)
		main := filepath.Join(temporal, "main.go")
		guiontest.Escribir(t, main, "package main\n/*\n#cgo LDFLAGS: -Wl,-needed_framework,SytueWidgetBridge\n*/\nimport \"C\"\nfunc main() {}\n")
		c.debeConstruir(t, "needed_framework")
		encontrado := false
		for _, l := range strings.Split(guiontest.Leer(t, registro), "\n") {
			encontrado = encontrado || l == "-Wl,-needed_framework,SytueWidgetBridge"
		}
		if !encontrado {
			t.Error("la flag del framework no llegó al enlazador")
		}
		guiontest.Escribir(t, main, strings.ReplaceAll(guiontest.Leer(t, main), "SytueWidgetBridge", "OtherFramework"))
		c.rechazar(t, "OtherFramework")
	})
}
