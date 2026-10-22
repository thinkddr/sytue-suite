// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image"
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func paso(tipo string, v float64) api.PasoReceta {
	return api.PasoReceta{Tipo: tipo, Params: map[string]float64{"valor": v}}
}

// TestElColorEsElDeLaWebYElServicio: las fórmulas de motor.ts y de receta.go, y el
// recorte a 0..1 AL FINAL, no entre pasos (una exposición que quema y un contraste que
// baja no es lo mismo que quemar y luego bajar).
func TestElColorEsElDeLaWebYElServicio(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 128, G: 64, B: 200, A: 255})
	for _, c := range []struct {
		pasos []api.PasoReceta
		r     func(float64) float64
	}{
		{[]api.PasoReceta{paso(api.PasoExposicion, 1)}, func(x float64) float64 { return x * 2 }},
		{[]api.PasoReceta{paso(api.PasoContraste, 0.5)}, func(x float64) float64 { return (x-0.5)*1.5 + 0.5 }},
		{[]api.PasoReceta{paso(api.PasoExposicion, 1), paso(api.PasoExposicion, -1)}, func(x float64) float64 { return x }},
	} {
		got := colorear(src, api.Receta{Version: 1, Pasos: c.pasos}).NRGBAAt(0, 0)
		if want := a8(c.r(128.0 / 255)); got.R != want {
			t.Errorf("%v: rojo %d y la fórmula da %d", c.pasos, got.R, want)
		}
	}
	// Saturación −1: gris con la luminancia Rec.709.
	gris := colorear(src, api.Receta{Version: 1, Pasos: []api.PasoReceta{paso(api.PasoSaturacion, -1)}}).NRGBAAt(0, 0)
	l := 0.2126*128.0/255 + 0.7152*64.0/255 + 0.0722*200.0/255
	if gris.R != gris.G || gris.G != gris.B || gris.R != a8(l) {
		t.Errorf("saturación −1 da %v y toca gris %d", gris, a8(l))
	}
}

// TestLaGeometriaMuevePixelesComoGo: girar 90° a la derecha intercambia los lados y la
// esquina de arriba a la izquierda acaba arriba a la derecha; el espejo horizontal la
// lleva al otro lado.
func TestLaGeometriaMuevePixelesComoGo(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	rojo := color.NRGBA{R: 255, A: 255}
	src.SetNRGBA(0, 0, rojo)
	g := aplicarGeometria(src, api.Receta{Version: 1, Pasos: []api.PasoReceta{{Tipo: api.PasoGiro, Params: map[string]float64{"grados": 90}}}})
	if g.Rect.Dx() != 2 || g.Rect.Dy() != 3 || g.NRGBAAt(1, 0) != rojo {
		t.Errorf("girar 90: %v y el rojo en (1,0) es %v", g.Rect, g.NRGBAAt(1, 0))
	}
	e := aplicarGeometria(src, api.Receta{Version: 1, Pasos: []api.PasoReceta{{Tipo: api.PasoEspejo, Eje: "h"}}})
	if e.NRGBAAt(2, 0) != rojo {
		t.Error("el espejo horizontal no llevó la esquina al otro lado")
	}
	r := aplicarGeometria(src, api.Receta{Version: 1, Pasos: []api.PasoReceta{{Tipo: api.PasoRecorte, Params: map[string]float64{"x": 0, "y": 0, "ancho": 0.34, "alto": 0.5}}}})
	if r.Rect.Dx() != 1 || r.Rect.Dy() != 1 || r.NRGBAAt(0, 0) != rojo {
		t.Errorf("el recorte da %v", r.Rect)
	}
}

// TestUnDeslizadorEsUnPaso: mover un deslizador sustituye su paso, y a 0 lo quita.
func TestUnDeslizadorEsUnPaso(t *testing.T) {
	r := conValor(api.RecetaVacia(), api.PasoContraste, 0.3)
	r = conValor(r, api.PasoContraste, 0.5)
	if len(r.Pasos) != 1 || valorDe(r, api.PasoContraste) != 0.5 {
		t.Fatalf("dos movimientos dejan %v", r.Pasos)
	}
	if r = conValor(r, api.PasoContraste, 0); len(r.Pasos) != 0 {
		t.Errorf("a 0 el paso sigue: %v", r.Pasos)
	}
}

// TestLaProporcionEsEnPixeles repite los casos de proporcion.test.ts.
func TestLaProporcionEsEnPixeles(t *testing.T) {
	casi := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
	m := conProporcion(Marco{0, 0, 0.5, 0.5}, 1, 1000, 500, 0, 0)
	if !casi(m.W*1000, m.H*500) || !casi(m.W, 0.25) || !casi(m.H, 0.5) {
		t.Errorf("un cuadrado en un lienzo de 1000×500 da %+v", m)
	}
	m = conProporcion(Marco{0.9, 0, 0.3, 0.9}, 1, 1000, 500, 0.9, 0)
	if m.X+m.W > 1+1e-9 || m.Y+m.H > 1+1e-9 || !casi(m.W*1000/(m.H*500), 1) {
		t.Errorf("se sale del lienzo: %+v", m)
	}
	m = conProporcion(Marco{0.2, 0.2, 0.6, 0.6}, 1, 1000, 500, 0.8, 0.8)
	if !casi(m.X+m.W, 0.8) || !casi(m.Y+m.H, 0.8) {
		t.Errorf("hacia arriba a la izquierda no queda pegado al ancla: %+v", m)
	}
	if m := (Marco{0.1, 0.1, 0.2, 0.2}); conProporcion(m, 0, 1000, 500, 0.1, 0.1) != m {
		t.Error("una proporción absurda cambió el marco")
	}
}

// TestElLienzoPintaElColorDeLaReceta: lo que se ve en el lienzo del editor es la fuente
// CON el color de la receta —exposición +1 dobla la luz— y, recortando, el velo del 55 %
// fuera del marco y la foto limpia dentro.
func TestElLienzoPintaElColorDeLaReceta(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 100, 100, 100, 255
	}
	l := NuevoLienzoEditor()
	l.Poner(src, api.Receta{Version: 1, Pasos: []api.PasoReceta{paso(api.PasoExposicion, 1)}}, FondoCrema)
	l.Resize(fyne.NewSize(40, 20))
	img := pinta(t, 40, 20, l)
	if got, quiero := px(img, 40, 20), (color.NRGBA{R: 200, G: 200, B: 200, A: 255}); !cerca(got, quiero) {
		t.Errorf("el centro del lienzo es %v y con exposición +1 toca %v", got, quiero)
	}
	l.Recortando, l.Marco = true, &Marco{X: 0.5, Y: 0, W: 0.5, H: 1}
	l.Refresh()
	img = pinta(t, 40, 20, l)
	// Fuera del marco: el gris de 200 bajo un negro al 55 % (alfa 140/255).
	fuera := uint8(math.Round(200 * (1 - 140.0/255)))
	if got := px(img, 10, 20); !cerca(got, color.NRGBA{R: fuera, G: fuera, B: fuera, A: 255}) {
		t.Errorf("fuera del marco hay %v y toca el velo, %d", got, fuera)
	}
	if got := px(img, 60, 20); !cerca(got, color.NRGBA{R: 200, G: 200, B: 200, A: 255}) {
		t.Errorf("dentro del marco hay %v y toca la foto limpia", got)
	}
	guardar(t, "editor-lienzo-recorte", img)
}

// TestElDameroEmpiezaArribaALaDerecha: el `repeating-conic-gradient(gris 0% 25%,
// transparent 0% 50%)` de la web pinta el cuarto de arriba a la derecha de cada baldosa
// y el de abajo a la izquierda; el de arriba a la izquierda va transparente.
func TestElDameroEmpiezaArribaALaDerecha(t *testing.T) {
	m := pintarDameroCon(16, 16, 8)
	if m.NRGBAAt(4, 4).A != 0 || m.NRGBAAt(12, 4).A == 0 || m.NRGBAAt(4, 12).A == 0 || m.NRGBAAt(12, 12).A != 0 {
		t.Errorf("el damero tiene la fase cambiada: arriba izq %v, arriba der %v", m.NRGBAAt(4, 4), m.NRGBAAt(12, 4))
	}
}
