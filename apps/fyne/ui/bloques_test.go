// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func cajaDePrueba(w, h float32) fyne.CanvasObject {
	r := fcanvas.NewRectangle(color.NRGBA{})
	r.SetMinSize(fyne.NewSize(w, h))
	return r
}

// TestLaPilaDeBloquesPoneElMargenDebajoYSeSaltaLosNil: cada bloque a todo el ancho, su
// margen DEBAJO (el `mb-*` de la web), los nil fuera —las ramas que en la web
// devuelven `null`— y un Alto propio manda sobre el mínimo.
func TestLaPilaDeBloquesPoneElMargenDebajoYSeSaltaLosNil(t *testing.T) {
	a, b, c := cajaDePrueba(50, 10), cajaDePrueba(80, 20), cajaDePrueba(30, 5)
	p := PilaBloques(
		Bloque{Obj: a, Margen: 4},
		Bloque{},
		Bloque{Obj: b, Margen: 6},
		Bloque{Obj: c, Alto: func(ancho float32) float32 { return ancho / 10 }},
	)
	if len(p.Objects) != 3 {
		t.Fatalf("la pila lleva %d hijos y el nil no cuenta: tocan 3", len(p.Objects))
	}
	if h := AltoA(p, 200); h != 10+4+20+6+20 {
		t.Errorf("a 200 mide %v y toca 60 (10+4, 20+6 y el Alto propio, 200/10)", h)
	}
	p.Resize(fyne.NewSize(200, 60))
	if b.Position().Y != 14 || c.Position().Y != 40 || c.Size() != fyne.NewSize(200, 20) {
		t.Errorf("b en %v (toca 14) y c en %v con %v (toca 40, 200×20)", b.Position(), c.Position(), c.Size())
	}
	if m := p.MinSize(); m.Width != 80 {
		t.Errorf("el mínimo toma el ancho del más ancho, 80, y da %v", m.Width)
	}
}

// TestAltoAMideElParrafoAlAncho: un párrafo que se reparte mide más cuanto más
// estrecho, también dentro de una pila y envuelto en AlAncho. Es para lo que existe
// AltoA: `fyne.Layout.MinSize` no recibe el ancho.
//
// Sin `test.NewTempApp`: TestMain ya montó la aplicación con el tema de la suite, y
// `NewTempApp` la sustituye por una de serie para las pruebas que vengan detrás (así
// fallaba TestLaCaraSeMideComoSePinta).
func TestAltoAMideElParrafoAlAncho(t *testing.T) {
	texto := strings.Repeat("Una frase que se reparte en varias líneas. ", 6)
	par := NuevoParrafo(texto, tema.TextoSM, Normal, "text", tema.Claro)
	ancho, estrecho := AltoA(par, 800), AltoA(par, 160)
	if estrecho <= ancho {
		t.Fatalf("a 160 mide %v y a 800 %v: estrecho tiene que medir más", estrecho, ancho)
	}
	if h := AltoA(PilaBloques(Bloque{Obj: par, Margen: 8}), 160); h != estrecho+8 {
		t.Errorf("la pila mide %v y toca el párrafo a 160 más su margen, %v", h, estrecho+8)
	}
	if m := AlAnchoFluido(160, par).MinSize(); m != fyne.NewSize(0, estrecho) {
		t.Errorf("el ancho fluido impide encoger la ventana: %v", m)
	}
	if m := AlAncho(160, par).MinSize(); m != fyne.NewSize(160, estrecho) {
		t.Errorf("AlAncho(160) pide %v y toca 160×%v", m, estrecho)
	}
}

// Las columnas de formulario tienen párrafos y grupos con margen dentro, no solo
// un párrafo directo. Medirlos sin ancho deja a la fila siguiente encima del texto.
func TestLaColumnaMideLosParrafosAnidadosAlAnchoDisponible(t *testing.T) {
	par := NuevoParrafo(strings.Repeat("Ayuda del campo que ocupa varias líneas. ", 5), tema.TextoSM, Normal, "text", tema.Claro)
	grupo := container.New(Margen{Izq: 12, Der: 12, Arr: 8, Abj: 8}, par)
	final := cajaDePrueba(40, 44)
	col := container.New(Columna{Hueco: 8}, grupo, final)
	ancho := float32(240)
	altoGrupo := AltoParrafo(par, ancho-24) + 16
	alto := AltoA(col, ancho)
	if alto != altoGrupo+8+44 {
		t.Fatalf("la columna no cuenta el texto envuelto: alto=%v, esperado=%v", alto, altoGrupo+8+44)
	}
	col.Resize(fyne.NewSize(ancho, alto))
	if final.Position().Y != altoGrupo+8 || grupo.Size().Height != altoGrupo {
		t.Errorf("el campo siguiente pisa la ayuda: grupo=%v, campo y=%v", grupo.Size(), final.Position().Y)
	}
	if AltoA(col, 600) >= alto {
		t.Error("al recuperar ancho el formulario conserva la altura del teléfono")
	}
}

func TestLaRejillaMideElTextoAlAnchoDeCadaColumna(t *testing.T) {
	objetos := make([]fyne.CanvasObject, 0, 4)
	for range 4 {
		par := NuevoParrafo(strings.Repeat("Una ayuda que ocupa varias líneas. ", 3), tema.TextoSM, Normal, "text", tema.Claro)
		objetos = append(objetos, container.New(Margen{Izq: 12, Der: 12, Arr: 8, Abj: 8}, par))
	}
	r := Rejilla{Fijas: 2, Hueco: 8}
	ancho := r.MinSizeCon(objetos, 600).Height
	estrecho := r.MinSizeCon(objetos, 240).Height
	if estrecho <= ancho {
		t.Fatalf("la rejilla ignora el texto envuelto: estrecho=%v, ancho=%v", estrecho, ancho)
	}
	for _, w := range []float32{600, 240, 600} {
		r.Layout(objetos, r.MinSizeCon(objetos, w))
		for _, o := range objetos {
			grupo := o.(*fyne.Container)
			par := grupo.Objects[0].(*Parrafo)
			if par.Size().Height != AltoParrafo(par, par.Size().Width) || o.Size().Height < par.Size().Height+16 {
				t.Errorf("el texto queda fuera de la tesela al cambiar a %v: tarjeta=%v, párrafo=%v", w, o.Size(), par.Size())
			}
		}
		if objetos[2].Position().Y < objetos[0].Size().Height+8 {
			t.Fatal("la segunda fila se monta sobre el texto de la primera")
		}
	}
}

// TestLaFilaSeParteComoFlexWrap: lo que no cabe baja de fila, el `ml-auto` empuja solo
// en la suya, el elástico llena lo que sobra y el que no cabe solo encoge.
func TestLaFilaSeParteComoFlexWrap(t *testing.T) {
	caja := func(w float32) fyne.CanvasObject { return cajaDePrueba(w, 20) }
	a, b, c := caja(100), caja(100), caja(100)
	f := FilaAuto{Hueco: 10, Derecha: 1, Partir: true, SaltoFila: 5}
	objs := []fyne.CanvasObject{a, b, c}
	if h := f.AltoCon(objs, 250); h != 45 {
		t.Errorf("a 250 son dos filas de 20 y un salto de 5, y mide %v", h)
	}
	f.Layout(objs, fyne.NewSize(250, 45))
	if b.Position().X != 150 || c.Position() != fyne.NewPos(0, 25) {
		t.Errorf("b en %v (toca 150, empujado) y c en %v (toca 0,25: la fila nueva empieza a la izquierda)", b.Position(), c.Position())
	}
	// El elástico se lleva lo que sobra en su fila.
	e := FilaAuto{Hueco: 10, Partir: true, Elastico: 2}
	d := caja(40)
	e.Layout([]fyne.CanvasObject{a, d, b}, fyne.NewSize(300, 20))
	if d.Size().Width != 80 {
		t.Errorf("el elástico mide %v y le tocan los 80 que sobran", d.Size().Width)
	}
	// Uno más ancho que la fila se queda solo y encoge.
	ancho := caja(400)
	f.Layout([]fyne.CanvasObject{ancho, b}, fyne.NewSize(250, 45))
	if ancho.Size().Width != 250 || b.Position().Y != 25 {
		t.Errorf("el ancho mide %v (toca 250) y b está en %v (toca la fila de abajo)", ancho.Size().Width, b.Position())
	}
}
