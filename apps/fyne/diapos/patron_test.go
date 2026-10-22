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

// TestLasFormasDelDisenoSeEditanComoElPatron: al editar un diseño con EditarDiseno(id),
// se editan tanto las formas del diseño (`decor`, debajo) como sus marcadores (`boxes`,
// encima): mover, recolorear, insertar forma o cuadro de texto y borrar reparten cada
// caja de vuelta entre `decor` y `boxes`, y se conservan en el JSON del modelo (`Fiel`).
func TestLasFormasDelDisenoSeEditanComoElPatron(t *testing.T) {
	src := `{"slides":[{"id":"a","layout":"portada","boxes":[{"id":"t","x":10,"y":10,"w":40,"h":20,"html":"<p>t</p>"}]}],` +
		`"master":{"boxes":[{"id":"m","x":0,"y":0,"w":20,"h":10,"html":"<p>Patrón</p>"}],` +
		`"layouts":[{"id":"portada","name":"Portada",` +
		`"decor":[{"id":"franja","kind":"shape","shape":"rect","x":0,"y":0,"w":80,"h":10,"fill":"#1F3864","html":""},` +
		`{"id":"pie","x":8,"y":90,"w":40,"h":6,"html":"<p>Pie fijo</p>"}],` +
		`"boxes":[{"id":"marc","x":8,"y":30,"w":84,"h":20,"html":"<h1>Título</h1>"}]}]}}`
	e, _, _ := montar(t, src)
	e.EditarDiseno("portada")
	if !e.escenario.Patron || e.DisenoEditado() != "portada" || e.escenario.Diseno != "portada" {
		t.Fatalf("EditarDiseno(portada): Patron=%v DisenoEditado=%q Diseno=%q", e.escenario.Patron, e.DisenoEditado(), e.escenario.Diseno)
	}
	if ids := idsDe(e.cajasEdit()); ids != "franja,pie,marc" {
		t.Fatalf("se editan %q (esperaba decor + boxes del diseño: franja,pie,marc)", ids)
	}
	if ids := idsDe(e.escenario.cajasVisibles()); ids != "m,franja,pie,marc" {
		t.Fatalf("el escenario pinta %q (esperaba el patrón detrás y decor + boxes del diseño)", ids)
	}
	// Mover y recolorear la franja (forma en decor):
	e.sel = "franja"
	e.Empujar(2, 3, false)
	e.PonerRelleno("#C00000")
	// Mover el pie (texto que ya estaba en decor, sigue en decor):
	e.sel = "pie"
	e.Empujar(1, 0, false)
	// Insertar una forma nueva (va a decor) y un cuadro de texto nuevo (va a boxes):
	e.NuevaForma("ellipse")
	idForma := e.sel
	e.NuevaCajaTexto()
	idTexto := e.sel

	d := e.m.Patron.Disenos[0]
	if ids := idsDe(d.Decor); ids != "franja,pie,"+idForma {
		t.Fatalf("decor = %q (esperaba franja,pie,%s)", ids, idForma)
	}
	if d.Decor[0].X != 2 || d.Decor[0].Y != 3 || d.Decor[0].Relleno != "#C00000" {
		t.Errorf("franja editada = %+v", d.Decor[0])
	}
	if d.Decor[1].X != 9 {
		t.Errorf("pie editado x = %v (esperaba 9)", d.Decor[1].X)
	}
	if ids := idsDe(d.Cajas); ids != "marc,"+idTexto {
		t.Fatalf("boxes = %q (esperaba marc,%s)", ids, idTexto)
	}
	// Borrar la forma insertada de decor:
	e.sel = idForma
	e.BorrarSeleccion()
	if ids := idsDe(e.m.Patron.Disenos[0].Decor); ids != "franja,pie" {
		t.Errorf("tras borrar forma, decor = %q", ids)
	}
	// Verificar que el viaje por JSON (Serializar -> Leer) preserva decor y boxes:
	raw := e.Serializar()
	m2, err := Leer([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if ids := idsDe(m2.Patron.Disenos[0].Decor); ids != "franja,pie" {
		t.Errorf("reimportado decor = %q", ids)
	}
	if ids := idsDe(m2.Patron.Disenos[0].Cajas); ids != "marc,"+idTexto {
		t.Errorf("reimportado boxes = %q", ids)
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
