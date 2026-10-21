// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestElIdiomaSaleDelSistemaYSiNoDelEntorno fija el orden y las formas que hay que
// aceptar: lo que diga el sistema manda, y si no dice nada, el entorno. La respuesta del
// sistema se le da hecha, para que el orden se pruebe igual en cualquier máquina; la de
// Android es JNI y necesita un teléfono, y la de macOS la mira su propio test.
func TestElIdiomaSaleDelSistemaYSiNoDelEntorno(t *testing.T) {
	for _, c := range []struct {
		nombre      string
		lcAll, lang string
		quiero      string
	}{
		{"nada puesto", "", "", ""},
		{"POSIX no es un idioma", "POSIX", "", ""},
		{"C tampoco", "C", "C", ""},
		{"LANG con codificación", "", "es_ES.UTF-8", "es-ES"},
		{"LANG sin codificación", "", "pt_BR", "pt-BR"},
		{"LC_ALL manda sobre LANG", "fr_FR.UTF-8", "es_ES.UTF-8", "fr-FR"},
		{"LC_ALL sin valor cae en LANG", "", "de_DE.UTF-8", "de-DE"},
		{"LC_ALL en C cae en LANG", "C", "it_IT.UTF-8", "it-IT"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			t.Setenv("LC_ALL", c.lcAll)
			t.Setenv("LANG", c.lang)
			if got := idiomaCon(""); got != c.quiero {
				t.Errorf("Idioma() = %q, quiero %q", got, c.quiero)
			}
			if got := idiomaCon("sv-SE"); got != "sv-SE" {
				t.Errorf("con el sistema en sueco, Idioma() = %q", got)
			}
		})
	}
}

// TestSinSistemaNoSeInventaUnTema: fuera de Android nadie sabe el tema, y decirlo es
// parte de la respuesta. Un `false` que parezca «modo claro» dejaría al escritorio y a la
// web sin poder distinguir «claro» de «no me consta».
//
// `go test` solo corre donde hay máquina que lo corra —Linux, macOS, Windows—, nunca en
// Android ni en iOS, así que aquí `temaOscuro` es siempre el de `sistema_otras.go`.
func TestSinSistemaNoSeInventaUnTema(t *testing.T) {
	if _, sabido := Oscuro(); sabido {
		t.Error("Oscuro() dice saber el tema en una plataforma sin sistema al que preguntar")
	}
}

// TestElNucleoNoImportaNingunToolkit es EL guardián de la partición: lo que hace que
// `sistema.Idioma()` —y todo `apps/nucleo`— se pueda llamar desde una app de Fyne.
//
// Se mira el FUENTE y no el grafo de compilación porque el fallo que esto impide es de
// OTRO módulo: `apps/fyne` no compila el núcleo para todos los destinos en su CI, así que
// un import de toolkit vuelto a colar solo se vería cuando alguien intentara usarlo y se
// encontrara con «missing go.sum entry for gioui.org/io/event», que es exactamente lo que
// pasó y lo que dejó tres apps con tres rodeos distintos.
//
// Desde el 2026-09-23 vale también para los ficheros `//go:build android`: el puente JNI
// ya no cuelga de `gioui.org/app.JavaVM()`, sino de [EnAndroid], que rellena quien tiene
// el toolkit. Se mira el MÓDULO entero, porque la sesión en el Keystore vive en `auth`.
// VISTO EN ROJO volviendo a poner el import de Gio en `auth/store_android.go`.
func TestElNucleoNoImportaNingunToolkit(t *testing.T) {
	vistos := 0
	err := filepath.WalkDir("..", func(ruta string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(ruta, ".go") {
			return err
		}
		arbol, err := parser.ParseFile(token.NewFileSet(), ruta, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		vistos++
		for _, imp := range arbol.Imports {
			ruta2, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if strings.HasPrefix(ruta2, "gioui.org") || strings.HasPrefix(ruta2, "fyne.io") {
				t.Errorf("%s importa %q: eso mete el toolkit en el go.mod de cualquiera que use el\n"+
					"  núcleo. Lo que pida el toolkit lo pone quien lo tiene, como sistema.EnAndroid.", ruta, ruta2)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if vistos < 50 {
		t.Fatalf("solo se han mirado %d ficheros: el guardián estaría mirando el directorio equivocado", vistos)
	}
}
