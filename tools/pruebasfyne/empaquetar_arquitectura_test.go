// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// apps/fyne usa el fork mantenido de Fyne, y el guion es ejecutable.
func TestEmpaquetarUsaElForkYEsEjecutable(t *testing.T) {
	d := empaquetarDirs(t)
	if !empaquetarHayLinea(guiontest.Leer(t, empaquetarUnir(d.fyne, "go.mod")), "replace fyne.io/fyne/v2 => github.com/thinkddr/fyne/v2 v2.8.2-sytue.8") {
		t.Error("apps/fyne debe usar el fork mantenido con URL entrantes y geometría UIKit de Fyne v2.8.2")
	}
	info, err := os.Stat(d.guion)
	if err != nil || info.Mode()&0o111 == 0 {
		t.Error("apps/fyne/scripts/empaquetar.sh no es ejecutable")
	}
}

func TestEmpaquetarGuardianDeArquitectura(t *testing.T) {
	dir, amd64, arm64 := empaquetarBinarios(t)

	// --- 1. El caso bueno pasa.
	t.Run("el caso bueno pasa", func(t *testing.T) {
		if r := empaquetar(t, "verificar", amd64, "amd64"); r.Codigo != 0 {
			t.Errorf("el guardián rechaza un binario amd64 declarado amd64: no dejaría publicar nada\n%s", r.Todo())
		}
	})

	// --- 2. EL CASO MALO FALLA, que es lo que se está comprobando.
	// Exactamente el fallo real: un binario ARM64 pasando por amd64.
	t.Run("el caso malo falla", func(t *testing.T) {
		r := empaquetar(t, "verificar", arm64, "amd64")
		switch {
		case r.Codigo == 0:
			t.Error("el guardián dio por bueno un binario arm64 declarado amd64: es el fallo de las cuatro versiones sin desplegar")
		case !strings.Contains(r.Todo(), "es arm64 y se declaró amd64"):
			t.Errorf("el guardián falla pero no dice qué vio ni qué esperaba: %s", empaquetarPrimera(r.Salida+r.Error))
		}
	})

	// --- 3. Y falla también cuando no puede leer la arquitectura.
	// El silencio es el peor resultado posible: «no lo sé» tiene que ser rojo, no
	// verde. Un fichero que no es un binario de Go es exactamente ese caso.
	t.Run("no saber es rojo", func(t *testing.T) {
		noEs := empaquetarUnir(dir, "no-es-binario")
		guiontest.Escribir(t, noEs, "esto no es un binario\n")
		if r := empaquetar(t, "verificar", noEs, "amd64"); r.Codigo == 0 {
			t.Error("el guardián da por buena una arquitectura que no ha podido leer")
		}
	})

	// --- FYP-06: el ejecutable de Linux es PIE.
	// Visto en rojo con el binario de prueba de siempre (sin -buildmode=pie).
	t.Run("PIE", func(t *testing.T) {
		pie := empaquetarConstruir(t, dir, "linux", "amd64", "-buildmode=pie")
		if r := empaquetar(t, "verificar-pie", pie); r.Codigo != 0 {
			t.Errorf("el guardián rechaza un binario PIE\n%s", r.Todo())
		}
		if r := empaquetar(t, "verificar-pie", amd64); r.Codigo == 0 {
			t.Error("el guardián da por PIE un binario que no lo es")
		}
	})
}

// --- 4. La CLI de Fyne va fijada, nunca desde @latest.
// Con `@latest` el mismo tag empaquetaba con una herramienta distinta según el día.
// La versión la fija versions.env y nadie la instala desde una referencia mutable.
func TestEmpaquetarCLIDeFyneFijada(t *testing.T) {
	d := empaquetarDirs(t)
	if !empaquetarHay(guiontest.Leer(t, empaquetarUnir(d.raiz, "deploy/sytue-git/versions.env")), `^FYNE_TOOLS_VERSION=[0-9]+\.[0-9]+\.[0-9]+$`) {
		t.Error("deploy/sytue-git/versions.env no fija FYNE_TOOLS_VERSION=X.Y.Z")
	}
	flujos, err := filepath.Glob(empaquetarUnir(d.raiz, ".github/workflows/*.yml"))
	if err != nil || len(flujos) == 0 {
		t.Fatalf("no hay workflows: %v", err)
	}
	for _, f := range append([]string{d.guion}, flujos...) {
		if l := empaquetarLineas(guiontest.Leer(t, f), `fyne.io/tools/cmd/fyne@latest`); len(l) > 0 {
			t.Errorf("la CLI de Fyne se instala desde @latest: %s: %s", f, l[0])
		}
	}
	// Y tampoco con `go install …@vX`: así Go ignora el go.mod de la app y la CLI sale con la
	// Fyne que pide fyne.io/tools (la 2.8.1), sin el fork. Se instala con fyne-cli.sh.
	for _, f := range append(flujos, empaquetarUnir(d.raiz, "apps/fyne/xcode/ci_scripts/ci_post_clone.sh")) {
		if l := empaquetarLineas(guiontest.Leer(t, f), `go install "?fyne.io/tools/cmd/fyne@`); len(l) > 0 {
			t.Errorf("la CLI de Fyne se instala sin el fork de la app (usa .github/actions/toolchain/fyne-cli.sh): %s: %s", f, l[0])
		}
	}
}

// --- FYP-05: UninstallString y QuietUninstallString entre comillas.
// La carpeta lleva espacio («Sytue One»): sin comillas, Windows prueba antes
// «…\Programs\Sytue.exe».
func TestEmpaquetarDesinstaladorEntreComillas(t *testing.T) {
	guion := guiontest.Leer(t, empaquetarDirs(t).guion)
	if !strings.Contains(guion, `"UninstallString" '"\$INSTDIR\\Desinstalar.exe"'`) {
		t.Error("el UninstallString del instalador de Windows no va entre comillas")
	}
	if !strings.Contains(guion, `"QuietUninstallString" '"\$INSTDIR\\Desinstalar.exe" /S'`) {
		t.Error("el QuietUninstallString del instalador de Windows no lleva la ruta entre comillas")
	}
}

// --- FYP-07: lo que escribe `fyne build` al compilar no ensucia el árbol.
func TestEmpaquetarMetadatosDeFyneIgnorados(t *testing.T) {
	r := guiontest.Ejecutar(t, guiontest.Opciones{}, "git", "-C", empaquetarDirs(t).raiz, "check-ignore", "-q", "apps/fyne/cmd/drive/fyne_metadata_init.go")
	if r.Codigo != 0 {
		t.Error("fyne_metadata_init.go no está ignorado: todo binario saldría +dirty")
	}
}

// --- FYP-04: los originales de los forks, a la vista de osv-scanner.
// Cada `replace X => github.com/thinkddr/…` tiene su X en apps/fyne/upstream/go.mod con la
// versión que pide apps/fyne/go.mod: si no, un aviso contra el original no se vería nunca.
// Visto en rojo quitando la línea de typesetting de upstream/go.mod.
func TestEmpaquetarOriginalesDeLosForks(t *testing.T) {
	d := empaquetarDirs(t)
	gomod := guiontest.Leer(t, empaquetarUnir(d.fyne, "go.mod"))
	upstream := guiontest.Leer(t, empaquetarUnir(d.fyne, "upstream/go.mod"))
	originales := empaquetarSed(gomod, `^replace ([^ ]*) => github.com/thinkddr/.*`)
	if len(originales) == 0 {
		t.Fatal("apps/fyne/go.mod no tiene ningún replace hacia los forks")
	}
	for _, original := range originales {
		// awk -v m="$original" '$1 == m && $2 ~ /^v/ {print $2; exit}'
		version := ""
		for l := range strings.SplitSeq(gomod, "\n") {
			if c := strings.Fields(l); len(c) >= 2 && c[0] == original && strings.HasPrefix(c[1], "v") {
				version = c[1]
				break
			}
		}
		patron := `^[[:space:]]*` + strings.ReplaceAll(original, ".", `\.`) + `[[:space:]]+` + strings.ReplaceAll(version, ".", `\.`) + `$`
		if !empaquetarHay(upstream, patron) {
			t.Errorf("apps/fyne/upstream/go.mod no lleva %s %s, el original de un fork", original, version)
		}
	}
	if !strings.Contains(guiontest.Leer(t, empaquetarUnir(d.raiz, ".github/workflows/security.yml")), "apps/fyne/upstream/go.mod") {
		t.Error("security.yml no le pasa a osv-scanner apps/fyne/upstream/go.mod")
	}
}

// BASH 3.2: lo que corre en el Mac (runner sytue-macos) lo lee el bash de macOS, y ese no
// entiende un `case` dentro de `$(…)` o `<(…)` cuyo patrón no abra con `(`: toma el `)`
// por el cierre y el guion ENTERO no se lee, con cualquier destino. Así cayó el primer
// `android-fyne` de verdad (v3.6.24). Aquí no hay bash 3.2, así que se mira la forma.
func TestEmpaquetarSinCaseQueBash32NoLee(t *testing.T) {
	d := empaquetarDirs(t)
	acciones, _ := filepath.Glob(empaquetarUnir(d.raiz, ".github/actions/toolchain/*.sh"))
	ficheros := make([]string, 0, len(acciones)+4)
	ficheros = append(ficheros, d.guion, empaquetarUnir(d.aqui, "apple-firma.sh"))
	ficheros = append(ficheros, acciones...)
	ficheros = append(ficheros, empaquetarUnir(d.raiz, "deploy/sytue-git/bin/publicar-asset.sh"), empaquetarUnir(d.raiz, ".github/workflows/release-desktop.yml"))
	re := regexp.MustCompile(`[<$]\(case [^)]* in(\s*$|\s+[^([:space:]])`)
	for _, f := range ficheros {
		for i, l := range strings.Split(empaquetarLeer(f), "\n") { // 2>/dev/null: un fichero que falte no cuenta
			if re.MatchString(l) {
				t.Errorf("case dentro de $(…)/<(…) que el bash 3.2 de macOS no sabe leer: %s:%d:%s", f, i+1, l)
			}
		}
	}
}
