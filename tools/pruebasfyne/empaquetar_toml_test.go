// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pruebasfyne

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// empaquetarArbolDePrueba arma en un directorio temporal un repositorio que es el de
// verdad salvo apps/fyne/cmd/<app>, que es una copia: el guion (que deduce FYNE y RAIZ de
// su propia ruta) trabaja entero, pero lo que reescriba de esa app no toca el árbol.
// Tampoco apps/fyne/dist, donde deja lo que produce.
func empaquetarArbolDePrueba(t *testing.T, app string) string {
	t.Helper()
	d := empaquetarDirs(t)
	raiz := empaquetarUnir(t.TempDir(), "raiz")
	enlazarSalvo := func(de, a string, salvo ...string) {
		t.Helper()
		entradas, err := os.ReadDir(de)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(a, 0o750); err != nil {
			t.Fatal(err)
		}
	siguiente:
		for _, e := range entradas {
			for _, s := range salvo {
				if e.Name() == s {
					continue siguiente
				}
			}
			if err := os.Symlink(empaquetarUnir(de, e.Name()), empaquetarUnir(a, e.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	enlazarSalvo(d.raiz, raiz, "apps")
	enlazarSalvo(empaquetarUnir(d.raiz, "apps"), empaquetarUnir(raiz, "apps"), "fyne")
	enlazarSalvo(d.fyne, empaquetarUnir(raiz, "apps/fyne"), "cmd", "dist")
	enlazarSalvo(empaquetarUnir(d.fyne, "cmd"), empaquetarUnir(raiz, "apps/fyne/cmd"), app)
	origen := empaquetarUnir(d.fyne, "cmd", app)
	err := filepath.WalkDir(origen, func(ruta string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(origen, ruta)
		destino := empaquetarUnir(raiz, "apps/fyne/cmd", app, rel)
		if e.IsDir() {
			return os.MkdirAll(destino, 0o750)
		}
		empaquetarCopiar(t, ruta, destino)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return raiz
}

// --- 11. Empaquetar no reescribe el FyneApp.toml.
// `fyne package` sube el Build y guarda el fichero sin comentarios. Con una CLI de
// mentira que hace lo mismo (y falla, para no seguir), el FyneApp.toml tiene que quedar
// como estaba. Visto en rojo quitando `restaurar_toml` del trap.
//
// El original lo hacía sobre el FyneApp.toml del Hub en el árbol, y lo restauraba si el
// guion no lo hacía; aquí, sobre una copia (empaquetarArbolDePrueba).
func TestEmpaquetarNoReescribeElFyneAppToml(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("el empaquetado de Linux solo se ejecuta en Linux, como en el original")
	}
	raiz := empaquetarArbolDePrueba(t, "hub")
	cli := t.TempDir()
	empaquetarEjecutable(t, empaquetarUnir(cli, "fyne"), `#!/bin/sh
case "$1" in
build) while [ $# -gt 0 ]; do [ "$1" = -o ] && : >"$2"; shift; done ;;
package) echo 'Build = 99' >>FyneApp.toml; exit 1 ;;
esac
`)
	empaquetarEjecutable(t, empaquetarUnir(cli, "pkg-config"), "#!/bin/sh\nexit 0\n")
	empaquetarEjecutable(t, empaquetarUnir(cli, "x86_64-linux-gnu-gcc"), "#!/bin/sh\nexit 0\n")
	toml := empaquetarUnir(raiz, "apps/fyne/cmd/hub/FyneApp.toml")
	antes := guiontest.Leer(t, toml)
	r := guiontest.Ejecutar(t, guiontest.Opciones{Env: []string{empaquetarGoprivate, empaquetarConPATH(cli)}},
		empaquetarUnir(raiz, "apps/fyne/scripts/empaquetar.sh"), "linux", "hub")
	if guiontest.Leer(t, toml) != antes {
		t.Errorf("empaquetar.sh deja el FyneApp.toml reescrito por fyne package\n%s", r.Todo())
	}
}
