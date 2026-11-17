// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// Una lista de casillas de la web se ve en el folio nativo con sus casillas DIBUJADAS en
// el margen (Figtree no tiene ☐ ni ☑): vacía, solo el borde; hecha, rellena de latón y con
// el tic; y sin el texto de la marca delante.
// Con SYTUE_CAPTURAS=<dir> deja la captura para mirarla.
func TestLasCasillasSeDibujanEnElFolio(t *testing.T) {
	f := folioCon(t, `<ul data-checklist=""><li data-checked="1">Pedir presupuesto</li>`+
		`<li data-checked="0">Firmar el contrato</li></ul><ul><li>Una viñeta</li></ul>`)
	if len(f.cajas) != 3 {
		t.Fatalf("se han maquetado %d cajas y son 3", len(f.cajas))
	}
	hecha, vacia := f.cajas[0], f.cajas[1]
	if hecha.marcaTexto != "" || vacia.marcaTexto != "" {
		t.Errorf("la casilla sale como texto: %q y %q", hecha.marcaTexto, vacia.marcaTexto)
	}
	objs := pintarCasilla(hecha, 0, 0, 1)
	if len(objs) != 3 {
		t.Fatalf("la casilla hecha tiene que ser caja y tic (3 objetos) y son %d", len(objs))
	}
	if r := objs[0].(*canvas.Rectangle); r.FillColor != rico.RellenoCasilla || r.Size().Width < 10 {
		t.Errorf("la casilla hecha: relleno %v, lado %v", r.FillColor, r.Size().Width)
	}
	objs = pintarCasilla(vacia, 0, 0, 1)
	if len(objs) != 1 {
		t.Fatalf("la casilla vacía es solo la caja y son %d objetos", len(objs))
	}
	r := objs[0].(*canvas.Rectangle)
	if _, _, _, a := r.FillColor.RGBA(); r.StrokeColor != rico.TintaCasilla || a != 0 {
		t.Errorf("la casilla vacía: borde %v, relleno %v", r.StrokeColor, r.FillColor)
	}
	// En el margen, a la izquierda del texto, y a la altura de su primera línea.
	if x := r.Position().X + r.Size().Width; x >= vacia.izq {
		t.Errorf("la casilla acaba en %v y el texto empieza en %v: se pisan", x, vacia.izq)
	}
	if y := r.Position().Y; y < vacia.y || y > vacia.y+vacia.alto {
		t.Errorf("la casilla está en y=%v, fuera de su renglón [%v, %v]", y, vacia.y, vacia.y+vacia.alto)
	}

	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		return
	}
	w := test.NewTempWindow(t, f)
	w.Resize(fyne.NewSize(900, 400))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(filepath.Join(dir, "docus-casillas.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := png.Encode(out, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}
