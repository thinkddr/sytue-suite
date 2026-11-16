// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLasFlechasRecorrenLasTarjetas: en la cuadrícula, ← y → van a la tarjeta de al lado,
// ↓ a la de debajo (las columnas salen de la medida: a 1280 son cinco), Inicio y Fin a los
// extremos. Es el `moverFocoEnRejilla` de `FileGrid` en la web (`apps/drive/src/lib/flechas.test.ts`).
func TestLasFlechasRecorrenLasTarjetas(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana, e.Rejilla = Exito, 1280, true
	e.Elementos = elementosDePrueba(time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC))
	w := test.NewWindow(e.Objeto(tema.Claro))
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 600))
	if len(e.tarjetas) != len(e.Elementos) {
		t.Fatalf("hay %d tarjetas para %d elementos", len(e.tarjetas), len(e.Elementos))
	}
	tarjeta := func(i int) *TarjetaDrive { return e.tarjetas[i].t }
	foco := func() fyne.Focusable { return w.Canvas().Focused() }
	enfocada := func(i int) bool { return foco() == e.tarjetas[i].objeto.(fyne.Focusable) }

	w.Canvas().Focus(e.tarjetas[0].objeto.(fyne.Focusable))
	tarjeta(0).TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if !enfocada(1) {
		t.Fatalf("→ desde la primera deja el foco en %v; tenía que ir a la segunda", foco())
	}
	tarjeta(1).TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if !enfocada(6) {
		t.Fatalf("↓ desde la segunda (de cinco columnas) deja el foco en %v; tenía que ir a la séptima", foco())
	}
	tarjeta(6).TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if !enfocada(5) {
		t.Fatal("← desde la primera de la segunda fila no volvió a la de al lado en el orden de lectura")
	}
	tarjeta(5).TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if !enfocada(len(e.tarjetas) - 1) {
		t.Fatal("Fin no fue a la última tarjeta")
	}
	tarjeta(len(e.tarjetas) - 1).TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	if !enfocada(0) {
		t.Fatal("Inicio no fue a la primera tarjeta")
	}
	// Intro sigue abriendo.
	var abierto string
	e.AlAbrir = func(el Elemento) { abierto = el.ID }
	e.Objeto(tema.Claro)
	e.tarjetas[0].t.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if abierto != e.Elementos[0].ID {
		t.Errorf("Intro sobre la primera tarjeta abrió %q", abierto)
	}
}

// TestLaListaAbreConIntroYSaltaConInicioYFin: en la lista el foco es de la lista, que ya
// mueve su resalte con ↑ y ↓. Inicio y Fin lo llevan a los extremos, e Intro y Espacio
// abren la fila resaltada, como el botón de la fila en la web.
func TestLaListaAbreConIntroYSaltaConInicioYFin(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 1280
	// Sin carpetas: en la forma Marco irían a la franja y la lista tendría solo el fichero.
	for i := range 30 {
		e.Elementos = append(e.Elementos, Elemento{
			ID: string(rune('a' + i)), Nombre: string(rune('a'+i)) + ".txt", Tamano: 10,
		})
	}
	var abiertos []string
	e.AlAbrir = func(el Elemento) { abiertos = append(abiertos, el.ID) }
	w := test.NewWindow(e.Objeto(tema.Claro))
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 500))
	n := len(e.filasDeLista())
	if n != 30 {
		t.Fatalf("la lista tiene %d filas; quería 30", n)
	}
	w.Canvas().Focus(e.filas)
	e.filas.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	// Pasar el ratón por otra fila avisa por OnHighlighted (lo hace el `listItem` de Fyne)
	// sin mover el resalte del teclado: Intro abre la resaltada, no la de debajo del puntero.
	e.filas.OnHighlighted(7)
	e.filas.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	e.filas.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	e.filas.TypedKey(&fyne.KeyEvent{Name: fyne.KeySpace})
	e.filas.TypedRune(' ')
	e.filas.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	e.filas.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnter})
	quiero := []string{e.filasDeLista()[1].ID, e.filasDeLista()[n-1].ID, e.filasDeLista()[0].ID}
	if len(abiertos) != len(quiero) {
		t.Fatalf("se abrieron %v; quería %v", abiertos, quiero)
	}
	for i := range quiero {
		if abiertos[i] != quiero[i] {
			t.Fatalf("se abrieron %v; quería %v", abiertos, quiero)
		}
	}
	if e.filas.GetScrollOffset() > 0.5 {
		t.Error("Inicio no devolvió la lista arriba")
	}
}
