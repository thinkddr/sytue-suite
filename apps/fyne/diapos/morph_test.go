// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

// TestElMorphEmparejaComoLaWeb es `morphPairs`: primero por id, luego por contenido
// (el texto sin etiquetas ni mayúsculas, la forma, el src), cada origen una sola vez; lo
// que no empareja entra nuevo.
func TestElMorphEmparejaComoLaWeb(t *testing.T) {
	desde := Diapo{Cajas: []Caja{
		{ID: "a", Tipo: "shape", Forma: "ellipse"},
		{ID: "x", HTML: "<h1>Hola  <b>Mundo</b></h1>"},
		{ID: "y", Tipo: "shape"},
	}}
	hasta := Diapo{Cajas: []Caja{
		{ID: "a", Tipo: "shape", Forma: "rect"}, // por id, aunque la forma cambie
		{ID: "z", HTML: "<p>hola mundo</p>"},    // por contenido
		{ID: "w", Tipo: "shape"},                // un rect: el de «y» (el de «a» ya se usó)
		{ID: "n", Tipo: "shape"},                // no queda otro rect: entra nuevo
	}}
	pares, entran := ParesMorph(desde, hasta)
	quiero := map[string]string{"a": "a", "z": "x", "w": "y"}
	if len(pares) != 3 || len(entran) != 1 || entran[0].ID != "n" {
		t.Fatalf("pares %+v, entran %+v", pares, entran)
	}
	for _, p := range pares {
		if quiero[p.Hasta.ID] != p.Desde.ID {
			t.Errorf("%s empareja con %s y quiero %s", p.Hasta.ID, p.Desde.ID, quiero[p.Hasta.ID])
		}
	}
}

// TestElMorphLlevaCadaCajaDesdeDondeEstaba: al avanzar a una diapositiva «morph», el
// primer fotograma pone la caja emparejada donde estaba en la anterior y con su tamaño
// (el FLIP desde la esquina de arriba a la izquierda), el último en su sitio; la nueva
// empieza transparente. Al retroceder no hay morph, como en la web.
//
// VISTO EN ROJO (2026-09-26, en el frontal): con el morph instantáneo de antes no había
// efectos que arrancar y el primer fotograma ya era el reposo.
func TestElMorphLlevaCadaCajaDesdeDondeEstaba(t *testing.T) {
	m, err := Leer([]byte(`{"slides":[
		{"id":"a","boxes":[{"id":"f","kind":"shape","shape":"rect","html":"","x":0,"y":0,"w":20,"h":20}]},
		{"id":"b","transition":"morph","boxes":[
			{"id":"f","kind":"shape","shape":"rect","html":"","x":50,"y":50,"w":40,"h":40},
			{"id":"n","kind":"shape","shape":"ellipse","html":"","x":0,"y":60,"w":10,"h":10}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPresentacion(m, 0, traductor(t))
	w := test.NewTempWindow(t, p)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(960, 540))
	p.Avanzar(1)
	if len(p.efectos) != 2 {
		t.Fatalf("el morph arranca %d efectos y son dos (el par y la que entra)", len(p.efectos))
	}
	forma := p.lienzo.r.porCaja["f"][0]
	cerca := func(a, b float32) bool { return math.Abs(float64(a-b)) < 0.6 }
	p.efectos[0].Tick(0)
	pos, tam := forma.o.Position(), forma.o.Size()
	origen := p.lienzo.r.esquinas["f"].SubtractXY(0.5*960, 0.5*540) // de 50 % a 0 %
	if !cerca(pos.X, origen.X) || !cerca(pos.Y, origen.Y) || !cerca(tam.Width, 0.2*960) || !cerca(tam.Height, 0.2*540) {
		t.Errorf("el morph empieza en %v con %v; quiero %v con 192 × 108", pos, tam, origen)
	}
	p.efectos[0].Tick(1)
	if forma.o.Position() != forma.pos || forma.o.Size() != forma.tam {
		t.Error("al acabar, la caja está en su sitio y con su tamaño")
	}
	p.efectos[1].Tick(0)
	if nueva := p.lienzo.r.porCaja["n"][0]; translucida(nueva) != 1 {
		t.Error("la caja nueva entra desde transparente")
	}
	p.Avanzar(-1)
	if len(p.efectos) != 0 {
		t.Error("al retroceder no hay morph")
	}
}

// translucida es cuánto deja ver una primitiva de una caja: 1 es invisible.
func translucida(p primitiva) float64 {
	if r, ok := p.o.(*canvas.Raster); ok {
		return r.Translucency
	}
	return -1
}
