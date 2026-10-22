// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"regexp"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestElCatalogoDeFirmasEsElDeLaWeb lee `formula-catalog.ts` y compara entrada a
// entrada con `firmas.go`, que se generó de él. Si la web cambia una firma o añade una
// función, esto lo dice.
func TestElCatalogoDeFirmasEsElDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/office/src/editor/sheets/formula-catalog.ts")
	re := regexp.MustCompile(`(?ms)^\s*'?([A-Z0-9_.]+)'?:\s*\{\s*args:\s*'((?:[^'\\]|\\.)*)',\s*summary:\s*'((?:[^'\\]|\\.)*)',?\s*\}`)
	web := re.FindAllStringSubmatch(fuente, -1)
	if len(web) < 100 {
		t.Fatalf("solo se leen %d firmas de la web: ¿ha cambiado el formato del fichero?", len(web))
	}
	if len(web) != len(catalogoFirmas) {
		t.Errorf("la web tiene %d firmas y firmas.go %d: regeneradlo", len(web), len(catalogoFirmas))
	}
	sinEscape := strings.NewReplacer(`\'`, "'")
	for _, m := range web {
		f, ok := buscarFirma(m[1])
		if !ok || f.Args != sinEscape.Replace(m[2]) || f.Resumen != sinEscape.Replace(m[3]) {
			t.Errorf("%s: la web dice %q / %q y aquí %+v", m[1], m[2], m[3], f)
		}
	}
	if n, p := partirFirma("SUM(número1; [número2]; …)"); n != "SUM" || strings.Join(p, "|") != "número1|[número2]|…" {
		t.Errorf("splitSignature da %q %q", n, p)
	}
	if n, p := partirFirma("PI()"); n != "PI" || len(p) != 0 {
		t.Errorf("splitSignature de PI() da %q %q", n, p)
	}
}

// teclear escribe runa a runa en la celda abierta, como el teclado.
func teclear(r *Rejilla, s string) {
	for _, ru := range s {
		r.ed.TypedRune(ru)
	}
}

// TestLaFormulaSeEscribeComoEnLaWeb: autocompletar, la pista, señalar una celda y un
// rango, F4, y los marcos y el tinte de las referencias.
func TestLaFormulaSeEscribeComoEnLaWeb(t *testing.T) {
	c := lienzoDePrueba()
	r := c.Rejilla
	r.Seleccionar(0, 5, false)
	r.Editar("=")
	teclear(r, "SUM")
	ac, ok := c.ac()
	if !ok || ac.items[0] != "SUM" || len(ac.items) != 8 {
		t.Fatalf("tecleando «=SUM» la lista es %v", ac.items)
	}
	r.ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	r.ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	r.ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := r.ed.Texto(); got != "=SUM(" || !r.Editando() {
		t.Fatalf("Intro con la lista abierta deja %q (editando: %v) y tiene que aceptar «SUM(»", got, r.Editando())
	}
	if f, arg, ok := c.pista(); !ok || arg != 0 || !strings.HasPrefix(f.Args, "SUM(") {
		t.Errorf("dentro de «=SUM(» la pista es %+v, argumento %d", f, arg)
	}
	if got := c.Barra.ed.Texto(); got != "=SUM(" {
		t.Errorf("la barra no refleja la fórmula de la celda: %q", got)
	}

	// Señalar: pulsar B2 escribe su referencia y la fórmula sigue abierta; arrastrar a
	// C3 la extiende.
	b2 := fyne.NewPos(AnchoCabecera+AnchoCelda+10, 2*AltoCelda+10)
	r.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: b2}})
	r.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: b2.AddXY(AnchoCelda, AltoCelda)}})
	r.DragEnd()
	if got := r.ed.Texto(); got != "=SUM(B2:C3" || !r.Editando() {
		t.Fatalf("señalar B2 y arrastrar a C3 deja %q (editando: %v)", got, r.Editando())
	}
	if len(r.refsFx) != 1 || r.refsFx[0].Rect != (formula.Rect{C1: 1, R1: 1, C2: 2, R2: 2}) {
		t.Errorf("los marcos de la rejilla son %+v", r.refsFx)
	}
	// F4 ancla la referencia bajo el cursor.
	r.ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyF4})
	if got := r.ed.Texto(); got != "=SUM($B$2:$C$3" {
		t.Errorf("F4 deja %q", got)
	}
	teclear(r, ")")
	r.ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := c.Libro.Crudo(0, 5); got != "=SUM($B$2:$C$3)" {
		t.Errorf("la fórmula confirmada es %q", got)
	}
	if c.fx != nil || len(r.refsFx) != 0 {
		t.Error("confirmar la celda no ha acabado la edición de la fórmula")
	}

	// La barra tiñe una fórmula aunque no se esté editando: A1 y B2, cada una de su color.
	c.Libro.Escribir(3, 0, "=A1+B2")
	r.Seleccionar(3, 0, false)
	img := pinta(t, 560, 300, c)
	guarda(t, img)
	var colores []color.Color
	for _, tr := range c.Barra.rend.tinte.trozos {
		if tr.Visible() {
			colores = append(colores, tr.Color)
		}
	}
	if len(colores) != 4 || colores[1] != ColorRefFx(0, tema.Claro) || colores[3] != ColorRefFx(1, tema.Claro) {
		t.Errorf("la barra tiñe «=A1+B2» con %v", colores)
	}
}

// TestLaFormulaLargaDeLaBarraSeTintaCorrida: una fórmula más larga que el campo corre
// como en el `<input>` de la web —el cursor, a la vista— y el tinte corre con ella (el
// `scrollLeft` que la web copia al fondo), recortado a la caja del editor.
func TestLaFormulaLargaDeLaBarraSeTintaCorrida(t *testing.T) {
	c := lienzoDePrueba()
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(300, 300))
	c.Libro.Escribir(0, 0, "=SUM(A1:A20)+SUM(B1:B20)+SUM(C1:C20)+SUM(D1:D20)")
	c.Rejilla.Seleccionar(0, 0, false)
	w.Canvas().Focus(c.Barra.ed)
	c.Barra.ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	c.Barra.Refresh()
	d := c.Barra.ed.Desplazamiento()
	if d <= 0 {
		t.Fatalf("con el cursor al final la fórmula no ha corrido (%v)", d)
	}
	r := c.Barra.rend
	if len(r.tinte.trozos) == 0 || !r.tinte.trozos[0].Visible() {
		t.Fatal("la fórmula larga no se tinta")
	}
	if x := r.tinte.trozos[0].Position().X; x >= 0 || x < -d-1 {
		t.Errorf("el primer trozo empieza en %v y el editor ha corrido %v", x, d)
	}
	if r.recorte.Size().Width <= 0 || r.recorte.Size().Height != AltoEntradaFx {
		t.Errorf("el recorte del tinte mide %v", r.recorte.Size())
	}
}

// TestLaRuedaSobreLaBarraCorreElTinte: la rueda sobre el campo corre la fórmula sin
// mover el cursor, y el tinte corre con ella en ese momento, no con la siguiente tecla.
func TestLaRuedaSobreLaBarraCorreElTinte(t *testing.T) {
	c := lienzoDePrueba()
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(300, 300))
	c.Libro.Escribir(0, 0, "=SUM(A1:A20)+SUM(B1:B20)+SUM(C1:C20)+SUM(D1:D20)")
	c.Rejilla.Seleccionar(0, 0, false)
	w.Canvas().Focus(c.Barra.ed)
	c.Barra.ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	c.Barra.Refresh()
	r := c.Barra.rend
	antes := r.tinte.trozos[0].Position().X
	campo := fyne.CurrentApp().Driver().AbsolutePositionForObject(c.Barra.ed).AddXY(20, 5)
	test.Scroll(w.Canvas(), campo, 40, 0)
	d := c.Barra.ed.Desplazamiento()
	if x := r.tinte.trozos[0].Position().X; x == antes || r.tinteX != d {
		t.Errorf("tras la rueda el editor ha corrido %v y el tinte sigue en %v (antes %v, apuntado %v)", d, x, antes, r.tinteX)
	}
}
