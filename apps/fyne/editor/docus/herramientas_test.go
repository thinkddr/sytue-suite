// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"golang.org/x/text/language"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Lo nuevo del folio contra los VECTORES COMPARTIDOS con la web
// (`packages/office/test/vectores/docus-herramientas.json`, que recorre
// `docs-herramientas.test.ts`), y sobre un folio de verdad.
const rutaHerramientas = "../../../frontend/packages/office/test/vectores/docus-herramientas.json"

type vectoresDocus struct {
	Ordenar []struct {
		Idioma    string
		Textos    []string
		Asc, Desc []int
	}
	Ciclo []struct{ Texto, Sale string }
	Talla []struct {
		Actual string
		Dir    int
		Sale   string
	}
	Lectura []struct{ Palabras, Sale int }
	//nolint:tagliatelle // el nombre lo fija la web
	TextoPlano []struct{ Texto, Sale string } `json:"textoPlano"`
}

func leerVectoresDocus(t *testing.T) vectoresDocus {
	t.Helper()
	datos, err := os.ReadFile(rutaHerramientas)
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresDocus
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLasHerramientasConLosVectoresDeLaWeb(t *testing.T) {
	v := leerVectoresDocus(t)
	for _, caso := range v.Ordenar {
		tag := language.MustParse(caso.Idioma)
		if got := rico.OrdenDeTextos(caso.Textos, false, tag); !slices.Equal(got, caso.Asc) {
			t.Errorf("%q de la A a la Z: %v, quería %v", caso.Textos, got, caso.Asc)
		}
		if got := rico.OrdenDeTextos(caso.Textos, true, tag); !slices.Equal(got, caso.Desc) {
			t.Errorf("%q de la Z a la A: %v, quería %v", caso.Textos, got, caso.Desc)
		}
	}
	for _, caso := range v.Ciclo {
		if got := rico.SiguienteModoMayusculas(caso.Texto); string(got) != caso.Sale {
			t.Errorf("Mayús+F3 sobre %q: %s, quería %s", caso.Texto, got, caso.Sale)
		}
	}
	for _, caso := range v.Talla {
		if got := PasoDeTalla(caso.Actual, caso.Dir); got != caso.Sale {
			t.Errorf("talla %q %+d: %q, quería %q", caso.Actual, caso.Dir, got, caso.Sale)
		}
	}
	for _, caso := range v.Lectura {
		if got := editor.MinutosDeLectura(caso.Palabras); got != caso.Sale {
			t.Errorf("%d palabras: %d min, quería %d", caso.Palabras, got, caso.Sale)
		}
	}
	for _, caso := range v.TextoPlano {
		if got := editor.TextoPlanoDeDescarga(caso.Texto); got != caso.Sale {
			t.Errorf("%q: %q, quería %q", caso.Texto, got, caso.Sale)
		}
	}
}

// Ordenar mueve los párrafos elegidos, deja los de fuera en su sitio, se deshace de una
// vez y no mezcla una lista con los párrafos de alrededor.
func TestOrdenarParrafosEnElFolio(t *testing.T) {
	const antes = `<p>fuera</p><p>pera</p><p>Árbol</p><p>manzana</p><h2>título</h2>`
	f := folioCon(t, antes)
	f.anc, f.cur, f.conCursor = punto{1, 0}, punto{3, 2}, true
	f.Mandar(editor.OrdenarParrafos, "asc")
	if got := f.Serializar(); got != `<p>fuera</p><p>Árbol</p><p>manzana</p><p>pera</p><h2>título</h2>` {
		t.Fatalf("de la A a la Z: %s", got)
	}
	f.deshacer()
	if got := f.Serializar(); got != antes {
		t.Errorf("ordenar es un solo paso de deshacer: %s", got)
	}
	g := folioCon(t, `<p>z</p><ul><li>b</li><li>a</li></ul>`)
	g.anc, g.cur, g.conCursor = punto{0, 0}, punto{2, 1}, true
	g.Mandar(editor.OrdenarParrafos, "asc")
	if got := g.Serializar(); got != `<p>z</p><ul><li>b</li><li>a</li></ul>` {
		t.Errorf("una lista no se mezcla con un párrafo: %s", got)
	}
}

// Ctrl+Mayús+. y Ctrl+Mayús+, (los del registro) suben y bajan un peldaño de letra.
func TestAumentarYReducirLaLetra(t *testing.T) {
	f := folioCon(t, `<p>hola</p>`)
	f.anc, f.cur, f.conCursor = punto{0, 0}, punto{0, 4}, true
	ctrlMayus := fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift
	f.pag.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyPeriod, Modifier: ctrlMayus})
	if got := f.Formato().Tamano; got != "4" {
		t.Fatalf("tras Ctrl+Mayús+. la talla es %q", got)
	}
	f.pag.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: ctrlMayus})
	f.pag.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: ctrlMayus})
	if got := f.Formato().Tamano; got != "2" {
		t.Errorf("tras dos Ctrl+Mayús+, la talla es %q", got)
	}
}

// Mayús+F3 recorre minúsculas → MAYÚSCULAS → Cada Palabra sobre lo elegido.
func TestMayusF3AlternaLasMayusculas(t *testing.T) {
	f := folioCon(t, `<p>hola mundo</p>`)
	f.anc, f.cur, f.conCursor = punto{0, 0}, punto{0, 10}, true
	f.mayus = true
	for _, quiero := range []string{"HOLA MUNDO", "Hola Mundo", "hola mundo"} {
		f.pag.TypedKey(&fyne.KeyEvent{Name: fyne.KeyF3})
		if got := f.textoSeleccionado(); got != quiero {
			t.Errorf("Mayús+F3 dejó %q, quería %q", got, quiero)
		}
	}
	f.mayus = false
	f.pag.TypedKey(&fyne.KeyEvent{Name: fyne.KeyF3})
	if got := f.textoSeleccionado(); got != "hola mundo" {
		t.Errorf("F3 sin Mayús no cambia nada: %q", got)
	}
}

// El texto sin formato es una línea por párrafo; la fecha y la hora, las cuatro formas.
func TestTextoPlanoYFechaHora(t *testing.T) {
	f := folioCon(t, `<p>uno</p><p><br></p><h2>dos</h2><ul><li>tres</li></ul>`)
	if got := f.TextoPlano(); got != "uno\ndos\ntres\n" {
		t.Errorf("texto plano %q", got)
	}
	ahora := time.Date(2026, 11, 25, 9, 7, 0, 0, time.Local)
	got := FormatosFechaHora("es", ahora)
	if len(got) != 4 || got[1] != "25 de noviembre de 2026" || got[2] != "miércoles, 25 de noviembre de 2026" || got[3] != "9:07" {
		t.Errorf("formatos %q", got)
	}
	if !strings.Contains(strings.Join(FormatosFechaHora(i18n.Locale("de"), ahora), "|"), "November") {
		t.Errorf("en alemán: %q", FormatosFechaHora("de", ahora))
	}
}
