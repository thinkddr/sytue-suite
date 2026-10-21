// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElPatronSeEditaComoEnLaWeb: «Editar patrón» pone en el escenario las cajas del
// patrón con sus campos sin sustituir, cierra el panel de abajo, cambia la mini-barra
// (número de página sí; notas, animaciones, fondo y transición no) y enseña la banda
// de 33 de alto (`border-b` + `py-1` + `h-6`), con «Cerrar patrón» de 24 pegado al
// `px-3` de 12 (SlidesCanvas.tsx, rem 16).
// Insertar, mover y borrar tocan el patrón y no la diapositiva; los conectores no
// entran. Al cerrarlo, la diapositiva pinta el patrón con {n} ya sustituido.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `objetivo` devolviendo siempre la
// diapositiva, se editaban sus cajas y el número de página no llegaba al patrón.
func TestElPatronSeEditaComoEnLaWeb(t *testing.T) {
	e, _, w := montar(t, `{"slides":[{"id":"a","boxes":[{"id":"t","x":10,"y":10,"w":40,"h":20,"html":"<p>t</p>"}]},`+
		`{"id":"b","boxes":[]}],"master":{"boxes":[{"id":"m","x":0,"y":0,"w":20,"h":10,"html":"<p>Logo</p>"}]}}`)
	e.EditarPatron()
	if !e.escenario.Patron || e.abajo != "" {
		t.Fatal("«Editar patrón» pone el patrón en el escenario y cierra el panel de abajo")
	}
	if ids := idsDe(e.cajasEdit()); ids != "m" {
		t.Errorf("se editan %q y son las del patrón", ids)
	}
	if ids := idsDe(e.escenario.cajasVisibles()); ids != "m" {
		t.Errorf("el escenario pinta %q: solo el patrón", ids)
	}
	var rotulos []string
	for _, o := range e.miniBarra.Objects {
		if b, ok := o.(*editor.BotonCinta); ok {
			rotulos = append(rotulos, b.Rotulo)
		}
	}
	if got := strings.Join(rotulos, "|"); got != "Cuadro de texto|Insertar gráfico|Insertar número de diapositiva" {
		t.Errorf("la mini-barra del patrón lleva %q", got)
	}
	banda, ok := e.zona.Objects[1].(*bandaPatron)
	if !ok {
		t.Fatal("bajo la mini-barra va la banda del patrón")
	}
	if h := banda.Size().Height; h != 33 {
		t.Errorf("la banda mide %v de alto y en la web 33", h)
	}
	if pos, tam := banda.cerrar.Position(), banda.cerrar.Size(); tam.Height != 24 ||
		pos.X+tam.Width != banda.Size().Width-12 || pos.Y != 4 {
		t.Errorf("«Cerrar patrón» en %v con %v", pos, tam)
	}
	if got := pixelEn(w, banda.cerrar, 0.5); !cerca(got, tema.Color("border", e.variante())) {
		t.Errorf("el borde de «Cerrar patrón» pinta %v: la caja nació sin borde", got)
	}
	guarda(t, "patron.png", w.Canvas().Capture())

	e.NuevoNumeroPagina()
	nuevo := e.sel
	if c := e.m.caja(IndicePatron, nuevo); c == nil || c.HTML != `<p style="text-align:right">{n}</p>` || c.X != 84 {
		t.Fatalf("el número de página va al patrón y elegido: %+v", c)
	}
	if n := len(e.m.Diapos[0].Cajas); n != 1 {
		t.Errorf("la diapositiva tiene %d cajas y no ha cambiado", n)
	}
	e.Empujar(-1, 0, false)
	if c := e.m.caja(IndicePatron, nuevo); c.X != 83 {
		t.Errorf("mover empuja la caja del patrón: x = %v", c.X)
	}
	e.NuevoConector("straight")
	if len(e.m.Diapos[0].Conectores) != 0 {
		t.Error("en el patrón no se insertan conectores")
	}
	e.BorrarSeleccion()
	if e.m.caja(IndicePatron, nuevo) != nil {
		t.Error("borrar quita la caja del patrón")
	}

	test.Tap(banda.cerrar)
	if e.escenario.Patron || e.patron {
		t.Fatal("«Cerrar patrón» vuelve a la diapositiva")
	}
	if ids := idsDe(e.escenario.cajasVisibles()); ids != "m,t" {
		t.Errorf("la diapositiva pinta %q: el patrón detrás y las suyas", ids)
	}
	if _, ok := e.zona.Objects[1].(*bandaPatron); ok {
		t.Error("sin patrón no hay banda")
	}
}

func idsDe(cs []Caja) string {
	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.ID)
	}
	return strings.Join(ids, ",")
}

// pixelEn es el color de la ventana en el centro del borde de arriba de `o`, `dy` px
// por debajo de él.
func pixelEn(w fyne.Window, o fyne.CanvasObject, dy float32) color.NRGBA {
	img := w.Canvas().Capture()
	esc := w.Canvas().Scale()
	p := fyne.CurrentApp().Driver().AbsolutePositionForObject(o).AddXY(o.Size().Width/2, dy)
	return color.NRGBAModel.Convert(img.At(int(p.X*esc), int(p.Y*esc))).(color.NRGBA)
}
