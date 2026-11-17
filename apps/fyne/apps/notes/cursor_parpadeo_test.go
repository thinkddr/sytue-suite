// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"runtime"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// relojFalso es la hora que el test adelanta a mano.
type relojFalso struct{ t time.Time }

func (r *relojFalso) ahora() time.Time      { return r.t }
func (r *relojFalso) pasar(d time.Duration) { r.t = r.t.Add(d) }
func nuevoRelojFalso() *relojFalso {
	return &relojFalso{t: time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)}
}
func fotograma(z *zonaEdicion) { fyne.DoAndWait(z.ticParpadeo) }
func cursorSeVe(z *zonaEdicion) bool {
	var ver bool
	fyne.DoAndWait(func() { ver = z.cursorEncendido() })
	return ver
}
func enfocarZona(w fyne.Window, z *zonaEdicion) { fyne.DoAndWait(func() { w.Canvas().Focus(z) }) }

// editorEnVentana es un editor enfocable en una ventana de pruebas, con el reloj falso.
func editorEnVentana(t *testing.T) (*EditorNota, fyne.Window, *relojFalso) {
	t.Helper()
	ed := NuevoEditorNota(trInterno(t), Leer("<p>hola</p>"), "")
	r := nuevoRelojFalso()
	ed.zona.ahora = r.ahora
	var w fyne.Window
	fyne.DoAndWait(func() {
		w = test.NewWindow(ed)
		w.Resize(fyne.NewSize(400, 300))
	})
	return ed, w, r
}

// TestElCursorParpadea: con el foco, la raya se enciende y se apaga cada
// PeriodoParpadeo; cada tecla la vuelve a encender y empieza la cuenta, que es lo que hace
// el navegador; y sin foco no queda animación.
//
// VISTO EN ROJO (2026-11-09): sin cursor_parpadeo.go la raya sigue encendida al cabo del
// periodo (el límite (2) que tenía el comentario de editor.go).
func TestElCursorParpadea(t *testing.T) {
	ed, w, r := editorEnVentana(t)
	defer fyne.DoAndWait(w.Close)
	z := ed.zona
	enfocarZona(w, z)
	if !cursorSeVe(z) {
		t.Fatal("al enfocar el cuerpo, el cursor no se ve")
	}
	r.pasar(PeriodoParpadeo - time.Millisecond)
	fotograma(z)
	if !cursorSeVe(z) {
		t.Error("antes de acabar el periodo el cursor ya se ha apagado")
	}
	r.pasar(time.Millisecond)
	fotograma(z)
	if cursorSeVe(z) {
		t.Fatalf("al cabo de %v el cursor sigue encendido: no parpadea", PeriodoParpadeo)
	}
	r.pasar(PeriodoParpadeo)
	fotograma(z)
	if !cursorSeVe(z) {
		t.Error("al cabo de otro periodo el cursor no vuelve a encenderse")
	}

	// Apagado, una tecla lo enciende y la cuenta empieza de nuevo desde ella.
	r.pasar(PeriodoParpadeo)
	fotograma(z)
	if cursorSeVe(z) {
		t.Fatal("el cursor tenía que estar en su fase apagada")
	}
	fyne.DoAndWait(func() { z.TypedRune('!') })
	if !cursorSeVe(z) {
		t.Error("al teclear, el cursor apagado no se enciende")
	}
	r.pasar(PeriodoParpadeo / 2)
	fotograma(z)
	if !cursorSeVe(z) {
		t.Error("medio periodo después de la tecla el cursor ya se ha apagado: la cuenta no se reinicia")
	}
	// Una flecha contra el borde no mueve nada, pero también se ve dónde se escribe.
	r.pasar(PeriodoParpadeo)
	fotograma(z)
	fyne.DoAndWait(func() { z.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd}) })
	if !cursorSeVe(z) {
		t.Error("tras una tecla de mover, el cursor no se ve")
	}

	// Sin foco no hay raya ni animación.
	fyne.DoAndWait(func() { w.Canvas().Unfocus() })
	if z.parpadeo != nil {
		t.Error("sin foco, la animación del parpadeo sigue en marcha")
	}
	if cursorSeVe(z) {
		t.Error("sin foco, el cursor se pinta")
	}
}

// TestElCursorNoParpadeaConMenosMovimiento: con «menos movimiento» la raya se queda
// encendida y quieta, y no arranca ninguna animación (lo que se repite sin fin se queda
// quieto, ui/menosmovimiento.go). Pedirlo con el parpadeo en marcha también lo para.
func TestElCursorNoParpadeaConMenosMovimiento(t *testing.T) {
	t.Run("lo pide la app", func(t *testing.T) {
		antes := ui.SinMovimiento
		ui.SinMovimiento = true
		defer func() { ui.SinMovimiento = antes }()
		ed, w, r := editorEnVentana(t)
		defer fyne.DoAndWait(w.Close)
		z := ed.zona
		enfocarZona(w, z)
		if z.parpadeo != nil {
			t.Error("con menos movimiento arranca la animación del parpadeo")
		}
		for range 3 {
			r.pasar(PeriodoParpadeo)
			fotograma(z)
			if !cursorSeVe(z) {
				t.Fatal("con menos movimiento el cursor se apaga")
			}
		}
	})
	t.Run("lo pide el sistema a medias", func(t *testing.T) {
		ed, w, r := editorEnVentana(t)
		defer fyne.DoAndWait(w.Close)
		z := ed.zona
		enfocarZona(w, z)
		r.pasar(PeriodoParpadeo)
		fotograma(z)
		if cursorSeVe(z) {
			t.Fatal("sin menos movimiento el cursor tenía que estar apagado")
		}
		defer sistema.FijarMenosMovimiento(true)()
		fotograma(z)
		if !cursorSeVe(z) || z.parpadeo != nil {
			t.Errorf("al pedir menos movimiento el cursor se ve %v y la animación sigue %v", cursorSeVe(z), z.parpadeo != nil)
		}
	})
}

// TestElCursorNoDejaNadaVivoAlCerrar: el parpadeo es una animación de Fyne, sin rutinas
// propias; al cerrar la ventana el primer fotograma la para, y no queda ninguna rutina
// nueva viva.
func TestElCursorNoDejaNadaVivoAlCerrar(t *testing.T) {
	// La ventana de la suite sigue abierta: se cierra solo la que tiene la nota.
	var suite fyne.Window
	fyne.DoAndWait(func() { suite = test.NewWindow(nil) })
	defer fyne.DoAndWait(func() { suite.Close() })
	antes := runtime.NumGoroutine()
	ed, w, r := editorEnVentana(t)
	z := ed.zona
	enfocarZona(w, z)
	if z.parpadeo == nil {
		t.Fatal("con el foco no arranca el parpadeo")
	}
	fyne.DoAndWait(w.Close)
	r.pasar(PeriodoParpadeo)
	fotograma(z)
	if z.parpadeo != nil {
		t.Error("con la ventana cerrada la animación del parpadeo sigue en marcha")
	}
	// Las rutinas del driver de pruebas tardan un poco en irse: se les da un margen.
	for i := 0; i < 50 && runtime.NumGoroutine() > antes; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if ahora := runtime.NumGoroutine(); ahora > antes {
		t.Errorf("al cerrar la ventana quedan %d rutinas más que antes de abrirla", ahora-antes)
	}
}
