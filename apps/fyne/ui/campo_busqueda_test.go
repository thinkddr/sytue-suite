// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Un campo `type="search"` se vacía con Escape, como en el navegador, y avisa del cambio
// (el filtro se recalcula). Uno normal no. VISTO EN ROJO sin la rama de Busqueda en
// TypedKey: el texto seguía ahí.
func TestEscapeVaciaElCampoDeBusqueda(t *testing.T) {
	for _, c := range []struct {
		busqueda bool
		queda    string
	}{{true, ""}, {false, "acta"}} {
		e := NuevaEntrada("Buscar")
		e.Busqueda = c.busqueda
		var cambios []string
		e.AlCambiar(func(s string) { cambios = append(cambios, s) })
		e.PonerTexto("acta")
		e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
		if e.Texto() != c.queda {
			t.Errorf("busqueda=%v: queda %q, quiero %q", c.busqueda, e.Texto(), c.queda)
		}
		if c.busqueda && (len(cambios) == 0 || cambios[len(cambios)-1] != "") {
			t.Errorf("vaciarlo no avisa del cambio: %v", cambios)
		}
	}
}

// Requerida es el `required` al enviar: vacía, marca el campo (`aria-invalid`) y dice
// que no; al escribir, la marca se va. Así el botón de un formulario con un campo
// obligatorio vacío dice por qué no hace nada (CertiPDF: Comparar, Organizar y Capas).
func TestRequeridaMarcaElCampoVacio(t *testing.T) {
	e := NuevaEntrada("1,3,5-8")
	if e.Requerida() || !e.Invalida {
		t.Fatalf("vacía: Requerida()=true o sin marcar (Invalida=%v)", e.Invalida)
	}
	e.PonerTexto("2")
	if e.Invalida {
		t.Error("al escribir sigue marcada")
	}
	if !e.Requerida() || e.Invalida {
		t.Error("con texto no pasa o se marca")
	}
}

// TestLosAtajosDeTextoLleganAlCampo: Fyne entrega ⌘A/⌘C/⌘V al objeto con el foco, que
// es la Entrada y no su editor. Sin reenviarlos se iban al lienzo y ningún campo de la
// suite seleccionaba, copiaba ni pegaba (visto en una ventana real, macOS y X11). Se
// prueba como lo hace el driver: preguntando al lienzo quién tiene el foco. VISTO EN
// ROJO sin `Entrada.TypedShortcut`.
func TestLosAtajosDeTextoLleganAlCampo(t *testing.T) {
	e := NuevaEntrada("")
	w := test.NewWindow(e)
	defer w.Close()
	e.PonerTexto("hola")
	w.Canvas().Focus(e)
	s, ok := w.Canvas().Focused().(fyne.Shortcutable)
	if !ok {
		t.Fatal("el campo con el foco no atiende atajos: ⌘A, ⌘C y ⌘V se pierden")
	}
	s.TypedShortcut(&fyne.ShortcutSelectAll{})
	if got := e.SelectedText(); got != "hola" {
		t.Errorf("⌘A ha seleccionado %q", got)
	}
	portapapeles := test.NewClipboard()
	portapapeles.SetContent("adiós")
	s.TypedShortcut(&fyne.ShortcutPaste{Clipboard: portapapeles})
	if got := e.Texto(); got != "adiós" {
		t.Errorf("⌘V sobre todo seleccionado deja %q", got)
	}
}

// TestElTamanoDeUnResultadoSeDiceEnSuIdioma: el Hub manda el tamaño de un fichero como
// número además del texto «11.7 KiB» del fragmento, que salía en inglés. VISTO EN ROJO
// devolviendo siempre el fragmento.
func TestElTamanoDeUnResultadoSeDiceEnSuIdioma(t *testing.T) {
	n := int64(1536)
	h := api.HubHit{Snippet: "1.5 KiB", SizeBytes: &n}
	if got := DetalleHit("es", h); got != "1,5 KB" {
		t.Errorf("en español el tamaño sale %q", got)
	}
	if got := DetalleHit("es", api.HubHit{Snippet: "Hola"}); got != "Hola" {
		t.Errorf("sin tamaño, el fragmento: %q", got)
	}
}

// TestLasTeclasDelEditorPasanPorElCampo: con el puntero de verdad el foco cae en el
// EDITOR de dentro, no en la Entrada (es el objeto más profundo bajo el clic). Sus teclas
// tienen que pasar por la Entrada: si no, `AlTeclear` no ve Intro tras pulsar en el campo
// —el formulario no se envía— y Escape no vacía una búsqueda. Lo que la Entrada no se
// queda sigue llegando al motor.
//
// VISTO EN ROJO (2026-10-05, en el frontal): sin `e.editor.alTecla = e.TypedKey` en
// `montar`, «AlTeclear no ha visto Intro con el foco en el editor». Deshecho.
func TestLasTeclasDelEditorPasanPorElCampo(t *testing.T) {
	e := NuevaEntrada("")
	var vistas []fyne.KeyName
	e.AlTeclear = func(k *fyne.KeyEvent) bool {
		vistas = append(vistas, k.Name)
		return k.Name == fyne.KeyReturn
	}
	e.PonerTexto("ab")
	e.editor.CursorColumn = 2
	e.editor.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if len(vistas) != 1 || vistas[0] != fyne.KeyReturn {
		t.Fatalf("AlTeclear no ha visto Intro con el foco en el editor: %v", vistas)
	}
	e.editor.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if len(vistas) != 2 || e.Texto() != "a" {
		t.Errorf("lo que el campo no se queda tiene que llegar al motor UNA vez: vistas %v, texto %q", vistas, e.Texto())
	}

	b := NuevaEntrada("")
	b.Busqueda = true
	b.PonerTexto("factura")
	b.editor.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if b.Texto() != "" {
		t.Errorf("Escape con el foco en el editor no vacía la búsqueda: %q", b.Texto())
	}
}
