// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las opciones que no caben se cuentan, sin invadir la columna siguiente. Al
// ampliar la rejilla vuelven a verse: el resumen no pierde los valores originales.
func TestLosChipsNoInvadenLaColumnaSiguiente(t *testing.T) {
	g := nuevaRejilla(func(clave string, _ ...i18n.Vars) string { return clave })
	g.Poner([]api.BasesField{
		{ID: "estado", Name: "Estado", Kind: api.BasesKindMultiselect, Choices: []string{"Pendiente de la aprobación definitiva del departamento", "Revisado", "Publicado"}},
		{ID: "importe", Name: "Importe", Kind: api.BasesKindNumber},
	}, []api.BasesRecord{{
		ID: "r1", Data: map[string]any{"estado": []string{"Pendiente de la aprobación definitiva del departamento", "Revisado", "Publicado"}, "importe": 125.5},
	}})
	r := test.WidgetRenderer(g).(*rejillaRenderer)
	g.Resize(g.MinSize())
	r.Layout(g.Size())
	chips := r.chips[celda{0, 0}]
	mas := r.masChips[celda{0, 0}]
	if mas == nil || !mas.Visible() || mas.Texto != "+2" {
		t.Fatal("no se cuenta lo que queda fuera de la celda")
	}
	x1, _ := r.columna(1)
	for _, ch := range append([]*ui.Insignia{mas}, chips...) {
		if ch.Visible() && ch.Position().X+ch.Size().Width > x1-margenCelda {
			t.Fatalf("%q invade la columna de importe", ch.Texto)
		}
	}
	// La celda numérica termina en el margen derecho de su columna.
	xt, ancho := r.columna(1)
	numero := r.textos[0][1]
	fin := numero.Position().X + ui.Medir(numero.Text, tema.TextoSM, ui.Normal).Width
	if fin < xt+ancho-margenCelda-1 || fin > xt+ancho-margenCelda+1 {
		t.Errorf("el importe termina en %v; debería terminar en %v", fin, xt+ancho-margenCelda)
	}
	g.Resize(fyne.NewSize(g.MinSize().Width+1200, g.MinSize().Height))
	r.Layout(g.Size())
	if mas.Visible() {
		t.Fatal("el contador sigue cuando ya caben todas las opciones")
	}
	for _, ch := range chips {
		if !ch.Visible() {
			t.Fatalf("%q no reaparece al ampliar", ch.Texto)
		}
	}
}

// La cabecera de una columna numérica va a la derecha, sobre sus cifras, como la web
// (`justify-end`); la de texto, a la izquierda.
// VISTO EN ROJO (2026-11-06): «Edad» salía a la izquierda y sus cifras a la derecha.
func TestLaCabeceraDeUnNumeroVaALaDerecha(t *testing.T) {
	g := nuevaRejilla(func(clave string, _ ...i18n.Vars) string { return clave })
	g.Poner([]api.BasesField{
		{ID: "n", Name: "Nombre", Kind: api.BasesKindText},
		{ID: "e", Name: "Edad", Kind: api.BasesKindNumber},
	}, []api.BasesRecord{{ID: "r1", Data: map[string]any{"n": "Ana", "e": 21.0}}})
	r := test.WidgetRenderer(g).(*rejillaRenderer)
	g.Resize(fyne.NewSize(g.MinSize().Width+200, g.MinSize().Height))
	r.Layout(g.Size())
	fin := func(col int) float32 {
		return r.titulos[col].Position().X + ui.Medir(r.titulos[col].Text, tema.TextoSM, ui.SemiNegrita).Width
	}
	x, ancho := r.columna(1)
	if f := fin(1); f < x+ancho-margenCelda-1 || f > x+ancho-margenCelda+1 {
		t.Errorf("«Edad» termina en %v; debería terminar en %v, con sus cifras", f, x+ancho-margenCelda)
	}
	if g0 := r.glifos[0].Position().X; g0 != margenCelda+r.ancho0 {
		t.Errorf("el glifo de «Nombre» empieza en %v; debería en %v", g0, r.ancho0+margenCelda)
	}
}

// La cabecera y la última fila siguen la esquina redondeada del marco, como la web
// (`rounded-lg overflow-auto`): la cabecera solo redondea arriba y el fondo de la
// última fila solo abajo.
// VISTO EN ROJO (2026-11-06): la cabecera era una píldora con una muesca abajo a la
// izquierda, y el fondo cuadrado de la última fila salía por la esquina del marco.
func TestLaCabeceraYLaUltimaFilaSiguenLaEsquinaDelMarco(t *testing.T) {
	g := nuevaRejilla(func(clave string, _ ...i18n.Vars) string { return clave })
	g.Poner([]api.BasesField{{ID: "n", Name: "Nombre", Kind: api.BasesKindText}},
		[]api.BasesRecord{{ID: "r1", Data: map[string]any{"n": "Uno"}}, {ID: "r2", Data: map[string]any{"n": "Dos"}}})
	g.hayEncima, g.filaEncima = true, 1
	w := test.NewWindow(g)
	defer w.Close()
	w.SetPadded(false)
	tam := fyne.NewSize(240, g.MinSize().Height)
	w.Resize(tam)
	img := w.Canvas().Capture()
	escala := float32(img.Bounds().Dx()) / tam.Width
	px := func(x, y float32) color.NRGBA {
		return color.NRGBAModel.Convert(img.At(int(x*escala), int(y*escala))).(color.NRGBA)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// Justo encima del filete de la primera fila, pegado al borde: es cabecera.
	if p, quiero := px(tema.Borde+1, altoCabeza-2), tema.Color("bg-subtle", v); p != quiero {
		t.Errorf("la cabecera no llega a la esquina de abajo: %v, quiero %v", p, quiero)
	}
	// Fuera de la esquina redondeada de abajo, lo mismo que fuera de la de arriba.
	if abajo, arriba := px(1, tam.Height-1), px(1, 1); abajo != arriba {
		t.Errorf("el fondo de la última fila sale por la esquina del marco: %v, fuera %v", abajo, arriba)
	}
}
