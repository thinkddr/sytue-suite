// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// elementosDe son los ElementoMenu de un menú, en orden.
func elementosDe(m *ui.Menu) []*ui.ElementoMenu {
	var out []*ui.ElementoMenu
	for _, o := range m.Elementos {
		if e, ok := o.(*ui.ElementoMenu); ok {
			out = append(out, e)
		}
	}
	return out
}

func elementoMenu(t *testing.T, m *ui.Menu, texto string) *ui.ElementoMenu {
	t.Helper()
	for _, e := range elementosDe(m) {
		if e.Texto == texto {
			return e
		}
	}
	t.Fatalf("el menú no tiene «%s»", texto)
	return nil
}

// TestLaPestanaAbreSuMenuYPoneElColor: el ▾ de la activa abre el menú, «Color de
// pestaña» abre la paleta, elegir un color lo deja en `tabColor` (y en lo que se guarda),
// y «Quitar color» lo quita. Mover a la derecha se alcanza desde el mismo menú.
func TestLaPestanaAbreSuMenuYPoneElColor(t *testing.T) {
	c := lienzoTraducido(t)
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(560, 300))
	p := c.Pestanas
	p.Refresh()
	if p.tira.chevron.Hidden {
		t.Fatal("la pestaña activa no lleva su ▾")
	}
	test.Tap(p.tira.chevron)
	m := p.MenuHoja()
	if m == nil {
		t.Fatal("el ▾ no abre el menú de la pestaña")
	}
	if m.Rotulo != "Opciones de la hoja: Hoja 1" {
		t.Errorf("el menú se anuncia como %q", m.Rotulo)
	}
	if e := elementoMenu(t, m, "Mover a la izquierda"); !e.Inerte {
		t.Error("la primera hoja se puede mover a la izquierda")
	}
	guarda(t, w.Canvas().Capture()) // con `SYTUE_CAPTURA`, el menú abierto
	test.Tap(elementoMenu(t, m, "Color de pestaña"))
	paleta := p.MenuHoja()
	if paleta == nil || paleta == m {
		t.Fatal("«Color de pestaña» no abre la paleta")
	}
	if n := len(elementosDe(paleta)); n != 1+len(editor.PaletaContenido) {
		t.Errorf("la paleta tiene %d opciones: «Quitar color» y las doce de contenido", n)
	}
	test.Tap(elementoMenu(t, paleta, "Color #DC2626"))
	if got := c.Libro.ColorPestana(0); got != "#DC2626" {
		t.Fatalf("la pestaña queda con el color %q", got)
	}
	if s := c.Serializar(); !strings.Contains(s, `"tabColor":"#DC2626"`) {
		t.Errorf("el color no sale en lo que se guarda: %s", s)
	}
	if !c.Libro.PuedeDeshacer() {
		t.Error("poner el color no se puede deshacer")
	}

	test.Tap(p.tira.chevron)
	test.Tap(elementoMenu(t, p.MenuHoja(), "Color de pestaña"))
	if e := elementoMenu(t, p.MenuHoja(), "Color #DC2626"); !e.Marcada {
		t.Error("la paleta no marca el color vigente")
	}
	test.Tap(elementoMenu(t, p.MenuHoja(), "Quitar color"))
	if got := c.Libro.ColorPestana(0); got != "" {
		t.Errorf("«Quitar color» deja %q", got)
	}

	test.Tap(p.tira.chevron)
	test.Tap(elementoMenu(t, p.MenuHoja(), "Mover a la derecha"))
	if got := c.Libro.Hojas(); got[1] != "Hoja 1" || c.Libro.Activa() != 1 {
		t.Errorf("mover a la derecha deja %v con la activa en %d", got, c.Libro.Activa())
	}
}

// TestLaPestanaSeManejaConElTeclado: F2 pide el nombre, y Mayús+F10 o el clic derecho
// en otra pestaña la activan y abren su menú.
func TestLaPestanaSeManejaConElTeclado(t *testing.T) {
	c := lienzoTraducido(t)
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(560, 300))
	p := c.Pestanas
	p.Refresh()
	p.tira.pestanas[0].TypedKey(&fyne.KeyEvent{Name: fyne.KeyF2})
	if len(w.Canvas().Overlays().List()) == 0 {
		t.Fatal("F2 no abre el diálogo de renombrar")
	}
	w.Canvas().Overlays().Remove(w.Canvas().Overlays().Top())

	p.tira.pestanas[1].TappedSecondary(&fyne.PointEvent{})
	if c.Libro.Activa() != 1 || p.MenuHoja() == nil {
		t.Fatalf("el clic derecho en «Ventas» deja la activa en %d y el menú abierto=%v",
			c.Libro.Activa(), p.MenuHoja() != nil)
	}
	if got := p.MenuHoja().Rotulo; got != "Opciones de la hoja: Ventas" {
		t.Errorf("el menú es de %q", got)
	}
}

// TestElMenuDeLaPestanaSeApagaConElLibroProtegido: con el libro protegido estructura y
// color salen inertes, como en Excel; y el ▾ no cambia la medida de las demás pestañas.
func TestElMenuDeLaPestanaSeApagaConElLibroProtegido(t *testing.T) {
	c := lienzoTraducido(t)
	c.Libro.ProtegerLibro(&ProteccionLibro{}, "")
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(560, 300))
	c.Pestanas.Refresh()
	test.Tap(c.Pestanas.tira.chevron)
	m := c.Pestanas.MenuHoja()
	if m == nil {
		t.Fatal("con el libro protegido el menú ni se abre")
	}
	for _, e := range elementosDe(m) {
		if !e.Inerte {
			t.Errorf("«%s» sigue viva con el libro protegido", e.Texto)
		}
	}
	c.PonerColorPestana(0, "#2563EB")
	if got := c.Libro.ColorPestana(0); got != "" {
		t.Errorf("con el libro protegido la pestaña se pinta de %q", got)
	}

	otra := c.Pestanas.tira.pestanas[1]
	if got, quiero := otra.Size(), MedidaPestanaHoja("Ventas", false); got.Width != quiero.Width {
		t.Errorf("la pestaña inactiva mide %v y su medida es %v", got, quiero)
	}
	ch := c.Pestanas.tira.chevron
	activa := c.Pestanas.tira.pestanas[0]
	if ch.Position().X != activa.Position().X+activa.Size().Width {
		t.Errorf("el ▾ cae en x=%v y no pegado a su pestaña", ch.Position().X)
	}
}

// TestLaPaletaMarcaElColorEnMinusculas: un libro guardado antes, o un .xlsx, puede traer
// el color en minúsculas; la paleta lo marca igual (la web compara en mayúsculas).
func TestLaPaletaMarcaElColorEnMinusculas(t *testing.T) {
	c := lienzoTraducido(t)
	c.Libro.f.hojas[0].pestana = "#dc2626"
	w := test.NewTempWindow(t, c)
	w.Resize(fyne.NewSize(560, 300))
	c.Pestanas.Refresh()
	test.Tap(c.Pestanas.tira.chevron)
	test.Tap(elementoMenu(t, c.Pestanas.MenuHoja(), "Color de pestaña"))
	if e := elementoMenu(t, c.Pestanas.MenuHoja(), "Color #DC2626"); !e.Marcada {
		t.Error("la paleta no marca el color vigente cuando viene en minúsculas")
	}
}
