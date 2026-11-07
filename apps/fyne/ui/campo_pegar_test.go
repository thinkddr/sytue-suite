// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// portapapelesFijo es un portapapeles con un texto puesto.
type portapapelesFijo struct{ texto string }

func (p *portapapelesFijo) Content() string     { return p.texto }
func (p *portapapelesFijo) SetContent(s string) { p.texto = s }

// TestAlPegarVeLoPegadoAntesQueElCampo: el ⌘V del teclado llega por la Entrada y el
// «Pegar» del menú de `widget.Entry` por su editor; los dos pasan por AlPegar, y lo que
// este se queda no entra en el campo. Lo que deja pasar se pega como siempre. Visto en
// rojo sin el gancho en `editor.TypedShortcut` (las dos vías pegaban).
func TestAlPegarVeLoPegadoAntesQueElCampo(t *testing.T) {
	e := NuevaEntrada("")
	var visto []string
	e.AlPegar = func(s string) bool {
		visto = append(visto, s)
		return s == "a\nb"
	}
	pp := &portapapelesFijo{"a\nb"}
	e.TypedShortcut(&fyne.ShortcutPaste{Clipboard: pp})
	e.editor.TypedShortcut(&fyne.ShortcutPaste{Clipboard: pp}) // la vía del menú contextual
	if e.Texto() != "" || len(visto) != 2 {
		t.Fatalf("AlPegar se quedó lo pegado y el campo dice %q (lo vio %d veces)", e.Texto(), len(visto))
	}
	pp.texto = "suelto"
	e.TypedShortcut(&fyne.ShortcutPaste{Clipboard: pp})
	if e.Texto() != "suelto" {
		t.Errorf("lo que AlPegar deja pasar no se pega: el campo dice %q", e.Texto())
	}
}

// TestPegarDiceDondeHaQuedado: `Pegar` pega en lugar de lo seleccionado —hacia delante o
// hacia atrás, que desde fuera no se distingue—, junta las líneas como el campo y dice
// entre qué runas ha quedado; sin pasar por AlPegar, que es quien lo llama.
//
// VISTO EN ROJO (2026-11-05, en el frontal): devolviendo `fin` sin restarle lo pegado,
// `inicio` caía detrás de lo pegado; y pegando con `e.TypedShortcut` en vez del motor,
// AlPegar se volvía a llamar a sí mismo.
func TestPegarDiceDondeHaQuedado(t *testing.T) {
	e := NuevaEntrada("")
	llamadas := 0
	e.AlPegar = func(string) bool { llamadas++; return true }
	e.PonerTexto("ab cd ef")
	// «cd» seleccionado de derecha a izquierda: el cursor queda delante.
	e.editor.CursorColumn = 5
	e.editor.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	e.editor.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	e.editor.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	e.editor.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	if e.SelectedText() != "cd" {
		t.Fatalf("la prueba no ha seleccionado «cd»: %q", e.SelectedText())
	}
	inicio, fin := e.Pegar("uno\ndós")
	if e.Texto() != "ab uno dós ef" || inicio != 3 || fin != 10 || llamadas != 0 {
		t.Errorf("Pegar deja %q entre %d y %d (AlPegar %d veces)", e.Texto(), inicio, fin, llamadas)
	}
	e.Inerte(true)
	if i, f := e.Pegar("x"); e.Texto() != "ab uno dós ef" || i != f {
		t.Errorf("un campo inerte no pega: %q, %d-%d", e.Texto(), i, f)
	}
}
