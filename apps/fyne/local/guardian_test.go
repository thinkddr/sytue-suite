// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// prohibidos son los paquetes que un binario de Sytue Office no puede enlazar (R-SIN-SYTUE):
// el cifrado de la suite, el cliente de Matrix, los medios de las llamadas y el olm de C.
var prohibidos = []string{
	"git.sytue.eu/sytue/one/apps/fyne/cifrado",
	"git.sytue.eu/sytue/one/apps/nucleo/matrix",
	"git.sytue.eu/sytue/one/apps/fyne/sesion",
	// Las llamadas: con `edicionlocal`, `apps/fyne/llamada` es solo su API vacía
	// (llamada_sin.go), y lo que no puede entrar son sus medios.
	"github.com/livekit/",
	"github.com/pion/",
	"maunium.net/go/mautrix/crypto/libolm",
	// La prueba de arranque (`local/prueba`, con el driver de pruebas de Fyne) es solo de
	// los `_test.go`: abre las puertas de prueba de `red` y de `local`.
	"git.sytue.eu/sytue/one/apps/fyne/local/prueba",
	"fyne.io/fyne/v2/test",
}

// TestLasAppsLocalesNoEnlazanCifradoNiMatrix recorre las dependencias de cada
// `cmd-local/*`. Rooms es la excepción del plan (§6.14): su proveedor de Matrix sin
// cifrado puede enlazar el SDK, pero nunca el cifrado de la suite ni libolm.
//
// `sesion` (la sesión de Sytue ID) tampoco entra: `ui` lo nombra solo en `ui/suite.go`,
// que con `edicionlocal` cambia por una sesión que nunca entra (C-04), y lo que no es
// sesión vive en `apps/fyne/anfitrion`. Rooms es la excepción: lo arrastra `cifrado`.
// `nucleo/auth` (el login de Sytue ID y su llavero) tampoco: TestAuthNoEntraEnLasApps.
func TestLasAppsLocalesNoEnlazanCifradoNiMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("recorre las dependencias con `go list`")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("sin la orden go")
	}
	apps, _ := filepath.Glob(filepath.Join("..", "cmd-local", "*", "main.go"))
	if len(apps) == 0 {
		t.Fatal("no hay apps en cmd-local")
	}
	for _, m := range apps {
		dir := "./" + filepath.ToSlash(filepath.Dir(filepath.Join("cmd-local", filepath.Base(filepath.Dir(m)), "x")))
		// Con las etiquetas con que se empaqueta la edición (empaquetar.sh, SYTUE_ARBOL=cmd-local).
		cmd := exec.Command("go", "list", "-tags", "goolm,edicionlocal", "-deps", dir)
		cmd.Dir = ".."
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list %s: %v", dir, err)
		}
		for _, dep := range strings.Fields(string(out)) {
			for _, p := range prohibidos {
				// El cliente de Matrix es el paquete exacto: `matrix/cierre` y
				// `matrix/sessionproof` son tipos pequeños que arrastra `sesion` (ver arriba).
				exacto := p == "git.sytue.eu/sytue/one/apps/nucleo/matrix" || p == "git.sytue.eu/sytue/one/apps/fyne/sesion"
				if dep == p || (!exacto && strings.HasPrefix(dep, p+"/")) || (strings.HasSuffix(p, "/") && strings.HasPrefix(dep, p)) {
					// Rooms es la excepción del plan (§6.14): la pantalla compartida de Rooms
					// importa `cifrado` (el panel de dispositivos) y el cliente de Matrix, que
					// el binario local enlaza sin usarlos —sus cuentas Matrix van sin cifrar por
					// `nucleo/local/chat`—. Sacarlos pide partir esa pantalla. libolm y los medios
					// de las llamadas siguen prohibidos también ahí.
					// `cifrado` importa `sesion` (su panel abre la sesión de la suite).
					if strings.HasSuffix(dir, "/rooms") && (p == "git.sytue.eu/sytue/one/apps/nucleo/matrix" || p == "git.sytue.eu/sytue/one/apps/fyne/cifrado" || p == "git.sytue.eu/sytue/one/apps/fyne/sesion") {
						continue
					}
					t.Errorf("%s enlaza %s", dir, dep)
				}
			}
		}
	}
}

// TestLosMainLocalesNoAbrenSesion: ningún `cmd-local/*` importa `sesion`, `auth` ni
// `cifrado`: la cáscara es `local`, y sin login no hay sesión que abrir.
func TestLosMainLocalesNoAbrenSesion(t *testing.T) {
	ficheros, _ := filepath.Glob(filepath.Join("..", "cmd-local", "*", "*.go"))
	for _, f := range ficheros {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		arbol, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range arbol.Imports {
			ruta := strings.Trim(imp.Path.Value, `"`)
			for _, no := range []string{"/apps/fyne/sesion", "/apps/fyne/cifrado", "/apps/nucleo/auth"} {
				if strings.HasSuffix(ruta, no) {
					t.Errorf("%s importa %s", f, ruta)
				}
			}
		}
	}
}

// paqueteAuth es `nucleo/auth`: el login OIDC de Sytue ID, los almacenes de la sesión,
// el llavero de la suite (con cgo en Apple: `llavero_apple.go`, `compartido_darwin.go`,
// `store_apple.c`) y sus migraciones. El cliente `api`, la caché `offline`, el OAuth de
// las nubes y las pantallas compartidas usan de él solo los testigos, los reclamos, sus
// dos errores y la vuelta a 127.0.0.1, que viven en la hoja `nucleo/autorizacion`;
// `offline` usa además el llavero de la suite para su caché cifrada, que con
// `edicionlocal` no existe (`offline/system_secrets_local.go`).
const paqueteAuth = "git.sytue.eu/sytue/one/apps/nucleo/auth"

// sistemasDeLaEdicion son los sistemas para los que se empaqueta Sytue Office, con la
// variante de la tienda de macOS (`tienda`, empaquetar.sh). Cada uno elige sus ficheros
// (`//go:build darwin`, `ios`…) y un importador que solo exista en uno no se ve desde
// los demás; CGO_ENABLED=1 porque los del llavero de Apple son cgo. `go list` no compila
// nada: no hace falta el SDK de cada sistema.
var sistemasDeLaEdicion = []struct{ goos, goarch, etiquetas string }{
	{"linux", "amd64", ""},
	{"windows", "amd64", ""},
	{"darwin", "arm64", ""},
	{"darwin", "arm64", ",tienda"},
	{"ios", "arm64", ""},
	{"android", "arm64", ""},
}

// importadoresDeAuthEnRooms son los ÚNICOS paquetes por los que `nucleo/auth` entra en
// Rooms: su pantalla compartida (`apps/rooms`) importa `cifrado` y el cliente de Matrix,
// y `cifrado` la sesión de la suite, que trae los demás. Se compara por igualdad: uno que
// deje de importarlo se borra de aquí.
var importadoresDeAuthEnRooms = []string{
	"git.sytue.eu/sytue/one/apps/fyne/cifrado",
	"git.sytue.eu/sytue/one/apps/fyne/sesion",
	"git.sytue.eu/sytue/one/apps/nucleo/documentos",
	"git.sytue.eu/sytue/one/apps/nucleo/matrix",
	"git.sytue.eu/sytue/one/apps/nucleo/nube",
	"git.sytue.eu/sytue/one/apps/nucleo/widgets",
}

// TestAuthNoEntraEnLasApps: ningún `cmd-local/*` enlaza `nucleo/auth`, en ningún
// sistema de la edición. Rooms es la excepción documentada (§6.14): lo enlaza solo por
// [importadoresDeAuthEnRooms]. Si falla, dice quién lo importa: lo que haga falta de
// `auth` va a `nucleo/autorizacion` (con alias en `auth`) o detrás de
// `//go:build !edicionlocal`.
func TestAuthNoEntraEnLasApps(t *testing.T) {
	if testing.Short() {
		t.Skip("recorre las dependencias con `go list`")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("sin la orden go")
	}
	apps, _ := filepath.Glob(filepath.Join("..", "cmd-local", "*", "main.go"))
	var dirs []string
	for _, m := range apps {
		if app := filepath.Base(filepath.Dir(m)); app != "rooms" {
			dirs = append(dirs, "./cmd-local/"+app)
		}
	}
	if len(dirs) == 0 {
		t.Fatal("no hay apps en cmd-local")
	}
	for _, so := range sistemasDeLaEdicion {
		sistema := so.goos + "/" + so.goarch + so.etiquetas
		if got := importadoresDeAuth(t, so.goos, so.goarch, so.etiquetas, dirs...); len(got) > 0 {
			t.Errorf("en %s, %s entra en Sytue Office por:\n  %s", sistema, paqueteAuth, strings.Join(got, "\n  "))
		}
		if got := importadoresDeAuth(t, so.goos, so.goarch, so.etiquetas, "./cmd-local/rooms"); !slices.Equal(got, importadoresDeAuthEnRooms) {
			t.Errorf("en %s, importan %s en Rooms:\n  %s\nquiero exactamente:\n  %s", sistema, paqueteAuth, strings.Join(got, "\n  "), strings.Join(importadoresDeAuthEnRooms, "\n  "))
		}
	}
}

// importadoresDeAuth son, ordenados, los paquetes que importan `nucleo/auth` en las
// dependencias de dirs compiladas para goos/goarch con las etiquetas de la edición.
func importadoresDeAuth(t *testing.T, goos, goarch, etiquetas string, dirs ...string) []string {
	t.Helper()
	args := append([]string{"list", "-tags", "goolm,edicionlocal" + etiquetas, "-deps", "-f", "{{.ImportPath}} {{join .Imports \" \"}}"}, dirs...)
	cmd := exec.Command("go", args...)
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list (%s/%s%s): %v", goos, goarch, etiquetas, err)
	}
	var importadores []string
	for _, linea := range strings.Split(string(out), "\n") {
		campos := strings.Fields(linea)
		if len(campos) > 0 && slices.Contains(campos[1:], paqueteAuth) {
			importadores = append(importadores, campos[0])
		}
	}
	slices.Sort(importadores)
	return slices.Compact(importadores)
}
