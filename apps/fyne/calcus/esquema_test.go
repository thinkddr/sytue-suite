// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// TestUnLibroConEsquemaSeEditaYSaleIgual: con `rowGroups` y `colGroups` el libro ya no
// se abre en solo lectura, y al guardarlo el esquema sale con los mismos datos.
func TestUnLibroConEsquemaSeEditaYSaleIgual(t *testing.T) {
	doc := `{"active":0,"sheets":[{"name":"H","cells":{"A1":"x"},` +
		`"rowGroups":[{"start":1,"end":3,"level":1,"collapsed":true}],"colGroups":[{"start":2,"end":3,"level":1}]}]}`
	lz := NuevoLienzo(NuevoLibro())
	if !lz.Cargar(doc) {
		t.Fatal("un libro con esquema se abre en solo lectura")
	}
	var a, b any
	_ = json.Unmarshal([]byte(doc), &a)
	_ = json.Unmarshal([]byte(lz.Serializar()), &b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("entra\n %s\ny sale\n %s", doc, lz.Serializar())
	}
	oc := lz.Libro.FilasOcultas()
	if len(oc) < 5 || !oc[1] || !oc[2] || !oc[3] || oc[4] {
		t.Errorf("el grupo contraído no esconde las filas 2 a 4: %v", oc[:5])
	}
	if lz.Libro.ColumnasOcultas() != nil {
		t.Error("un grupo de columnas abierto no esconde nada")
	}
}

// TestAgruparContraerYNiveles son los botones del grupo Esquema y los del canalón.
func TestAgruparContraerYNiveles(t *testing.T) {
	lz := NuevoLienzo(NuevoLibro())
	r := lz.Rejilla
	r.sel, r.ext = Celda{C: 0, F: 1}, &Celda{C: 0, F: 3}
	lz.Mandar(CmdEsquema, `{"axis":"row","op":"group"}`)
	if got := lz.Libro.GruposFilas(); !reflect.DeepEqual(got, []analisis.Grupo{{Start: 1, End: 3, Level: 1}}) {
		t.Fatalf("agrupar las filas 2 a 4 deja %v", got)
	}
	lz.Mandar(CmdEsquema, `{"axis":"row","op":"toggle","start":1,"end":3,"level":1}`)
	if !lz.Libro.GruposFilas()[0].Collapsed || !lz.Libro.FilasOcultas()[2] {
		t.Error("contraer no esconde el grupo")
	}
	lz.Mandar(CmdEsquema, `{"axis":"row","op":"level","level":2}`)
	if lz.Libro.GruposFilas()[0].Collapsed {
		t.Error("enseñar hasta el nivel 2 no abre el grupo de nivel 1")
	}
	lz.Deshacer(false)
	if !lz.Libro.GruposFilas()[0].Collapsed {
		t.Error("deshacer no vuelve a contraerlo")
	}
	r.sel, r.ext = Celda{C: 1, F: 0}, &Celda{C: 2, F: 0}
	lz.Mandar(CmdEsquema, `{"axis":"col","op":"group"}`)
	lz.Mandar(CmdEsquema, `{"axis":"col","op":"toggle","start":1,"end":2,"level":1}`)
	if oc := lz.Libro.ColumnasOcultas(); len(oc) < 4 || !oc[1] || !oc[2] || oc[3] {
		t.Error("contraer un grupo de columnas no las esconde")
	}
}

// TestLosSubtotalesSumanComoEnLaWeb: el asistente escribe SUBTOTAL, el total general no
// cuenta dos veces y, contraído el detalle, el 109 deja de verlo.
func TestLosSubtotalesSumanComoEnLaWeb(t *testing.T) {
	lz := NuevoLienzo(NuevoLibro())
	lz.T = nil // las claves tal cual: el rótulo del total sale como su clave
	for ref, v := range map[Celda]string{
		{0, 0}: "Zona", {1, 0}: "Ventas", {0, 1}: "N", {1, 1}: "10", {0, 2}: "N", {1, 2}: "20", {0, 3}: "S", {1, 3}: "5",
	} {
		lz.Libro.CargarEn(0, ref.C, ref.F, v)
	}
	lz.Mandar(CmdSubtotal, `{"range":"A1:B4","groupCol":0,"fn":"sum"}`)
	l := lz.Libro
	if got := l.HTML(1, 3); got != "<b>=SUBTOTAL(9;B2:B3)</b>" {
		t.Fatalf("el subtotal de N es %q", got)
	}
	for _, c := range []struct {
		f    int
		want float64
	}{{3, 30}, {5, 5}, {6, 35}} {
		if got := l.Valor(1, c.f); got != c.want {
			t.Errorf("B%d vale %v; quería %v", c.f+1, got, c.want)
		}
	}
	if len(l.GruposFilas()) != 3 {
		t.Errorf("el esquema tiene %d grupos", len(l.GruposFilas()))
	}
	lz.Libro.CargarEn(0, 3, 0, "=SUBTOTAL(109,B2:B7)")
	lz.Libro.CargarEn(0, 4, 0, "=SUBTOTAL(9,B2:B7)")
	if l.Valor(3, 0) != 35.0 {
		t.Errorf("con todo a la vista, el 109 da %v", l.Valor(3, 0))
	}
	lz.Mandar(CmdEsquema, `{"axis":"row","op":"level","level":2}`)
	if got := l.Valor(3, 0); got != 0.0 {
		t.Errorf("con el detalle contraído, el 109 da %v y no 0", got)
	}
	if got := l.Valor(4, 0); got != 35.0 {
		t.Errorf("con el detalle contraído, el 9 da %v y no 35", got)
	}
	lz.Mandar(CmdSubtotal, `{"range":"A1:B7","remove":true}`)
	if l.HTML(0, 3) != "S" || l.GruposFilas() != nil || l.HTML(1, 4) != "" {
		t.Errorf("«Quitar todos» deja %q en A4, %v de esquema y %q en B5", l.HTML(0, 3), l.GruposFilas(), l.HTML(1, 4))
	}
}

// TestElCanalonSePintaYSePulsa: con un grupo contraído, el canalón corre las celdas
// 19 px (un carril de 14 y 5 de aire), el «+» cae en la fila del resumen y pulsarlo abre
// el grupo. Con `SYTUE_CAPTURA` deja el PNG a los dos anchos.
func TestElCanalonSePintaYSePulsa(t *testing.T) {
	doc := `{"active":0,"sheets":[{"name":"H","cells":{"A1":"<b>Zona</b>","B1":"<b>Ventas</b>","A2":"N","B2":"10",` +
		`"A3":"N","B3":"20","A4":"<b>Total N</b>","B4":"<b>=SUBTOTAL(9;B2:B3)</b>","A5":"S","B5":"5",` +
		`"A6":"<b>Total S</b>","B6":"<b>=SUBTOTAL(9;B5:B5)</b>","A7":"<b>Total general</b>","B7":"<b>=SUBTOTAL(9;B2:B6)</b>"},` +
		`"rowGroups":[{"start":1,"end":2,"level":2,"collapsed":true},{"start":4,"end":4,"level":2},{"start":1,"end":5,"level":1}],` +
		`"colGroups":[{"start":2,"end":3,"level":1}]}]}`
	for _, tam := range []fyne.Size{fyne.NewSize(1280, 800), fyne.NewSize(360, 740)} {
		lz := NuevoLienzo(NuevoLibro())
		if !lz.Cargar(doc) {
			t.Fatal("no se entiende el libro")
		}
		r := lz.Rejilla
		r.Resize(tam)
		img := pinta(t, tam.Width, tam.Height, r)
		if tam.Width == 1280 {
			guarda(t, img)
		}
		cx, cy := r.canal()
		if cx != 2*PasoEsquema+AireEsquema || cy != PasoEsquema+AireEsquema {
			t.Fatalf("el canalón mide %v × %v", cx, cy)
		}
		// La primera celda empieza tras el canalón y la cabecera de fila.
		x0, y0 := AnchoCabecera+cx+1, AltoCelda+cy+1
		if z, c := r.zonaEn(fyne.NewPos(x0, y0)); z != zonaCelda || c != (Celda{}) {
			t.Errorf("en (%v, %v) está la zona %d, celda %v", x0, y0, z, c)
		}
		// El «+» del grupo contraído (nivel 2, segundo carril) está en la fila 4, que es
		// la segunda que se ve.
		boton := fyne.NewPos(PasoEsquema+PasoEsquema/2, AltoCelda+cy+AltoCelda+AltoCelda/2)
		borde := tema.Color("border", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		if got := px(img, boton.X-ladoConmutador/2+0.25, boton.Y); !cerca(got, borde) {
			t.Errorf("el borde del «+» no está donde se pulsa: %v y no %v", got, borde)
		}
		r.Tapped(&fyne.PointEvent{Position: boton})
		if lz.Libro.GruposFilas()[0].Collapsed {
			t.Error("pulsar el «+» no abre el grupo")
		}
	}
}
