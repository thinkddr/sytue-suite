// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLasFilasEsqueletoMidenLaFila es el espejo de «cada fila mide py-2 más lo más alto
// de sus hijos» de `Skeleton.test.tsx`: el esqueleto ocupa lo que ocuparán las filas de
// verdad y la lista no salta al llegar los datos.
//
// VISTO EN ROJO: con un alto fijo de una línea para todas las filas, la tesela de 36 de
// Drive desborda y el caso de la tesela dice «mide 36 y la fila de Drive 50».
func TestLasFilasEsqueletoMidenLaFila(t *testing.T) {
	for _, c := range []struct {
		d      DelanteroEsqueleto
		lineas int
		quiere float32
	}{
		{SinDelantero, 1, 2*AltoFila + tema.TextoBase.Linea},
		{SinDelantero, 2, 2*AltoFila + tema.TextoBase.Linea + tema.TextoSM.Linea},
		{DelanteroAvatar, 1, 2*AltoFila + 32},
		// La fila de carga de Drive mide lo que la fila de verdad.
		{DelanteroTesela, 1, AltoFilaDrive},
	} {
		if got := AltoFilaEsqueleto(c.d, c.lineas); got != c.quiere {
			t.Errorf("delantero %d con %d líneas: la fila mide %g y tiene que medir %g", c.d, c.lineas, got, c.quiere)
		}
		f := NuevasFilasEsqueleto(4, c.d, c.lineas)
		if h := f.MinSize().Height; h != 4*c.quiere {
			t.Errorf("cuatro filas (delantero %d, %d líneas) piden %g de alto y son %g", c.d, c.lineas, h, 4*c.quiere)
		}
	}
}

// TestLasFilasEsqueletoLatenAUna: una sola animación para toda la lista y todos los
// bloques del mismo tono en cada instante, como los `.sytue-skeleton` de la web, que
// arrancan juntos.
//
// VISTO EN ROJO: montando las filas con un `Esqueleto` por bloque —lo que hacían las
// recetas de Drive— el renderer tiene widgets y no rectángulos, y cada uno su animación.
func TestLasFilasEsqueletoLatenAUna(t *testing.T) {
	f := NuevasFilasEsqueleto(5, DelanteroTesela, 2)
	f.Meta = []MetaEsqueleto{{Ancho: 44}, {Ancho: 76}}
	f.Resize(fyne.NewSize(600, f.MinSize().Height))
	r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
	defer f.Parar()
	if f.anim == nil {
		t.Fatal("las filas no han arrancado su latido")
	}
	// Tesela, título, subtítulo y dos metas por fila: 25 bloques, todos de lienzo.
	if n := len(r.bloques); n != 25 {
		t.Errorf("cinco filas pintan %d bloques y tienen que pintar 25", n)
	}
	for _, o := range r.Objects() {
		if _, es := o.(*canvas.Rectangle); !es {
			t.Fatalf("un bloque de la lista es %T: cada widget late por su cuenta", o)
		}
	}
	f.mu.Lock()
	f.fase = 0.5
	f.mu.Unlock()
	r.tenir()
	primero := r.bloques[0].FillColor
	for i, b := range r.bloques {
		if b.FillColor != primero {
			t.Fatalf("el bloque %d pinta %v y el primero %v: no laten a una", i, b.FillColor, primero)
		}
	}
	if opaco := tintaEsqueleto("", 0); primero == opaco {
		t.Error("a mitad de latido los bloques siguen opacos: el latido no llega a los bloques")
	}
}

// TestLasFilasEsqueletoSeAnuncianUnaVez: un solo «Cargando» para toda la lista (§3.18),
// no uno por fila; con rótulo propio, el propio.
func TestLasFilasEsqueletoSeAnuncianUnaVez(t *testing.T) {
	f := NuevasFilasEsqueleto(6, DelanteroAvatar, 1)
	if got, quiere := f.AccessibilityLabel(), textoComun("ui.loading"); got != quiere || got == "" {
		t.Errorf("las filas se anuncian %q y tienen que decir %q", got, quiere)
	}
	f.Rotulo = "Cargando archivos…"
	if got := f.AccessibilityLabel(); got != f.Rotulo {
		t.Errorf("con rótulo propio se anuncian %q", got)
	}
	r := test.WidgetRenderer(f)
	defer f.Parar()
	for _, o := range r.Objects() {
		if _, es := o.(fyne.Accessible); es {
			t.Fatalf("un bloque (%T) llega al lector: la carga se diría una vez por fila", o)
		}
	}
}

// TestLasFilasEsqueletoColocanLaMeta: la meta va pegada a la derecha, la oculta no ocupa
// sitio y el título no se mete debajo de ella.
func TestLasFilasEsqueletoColocanLaMeta(t *testing.T) {
	f := NuevasFilasEsqueleto(1, SinDelantero, 1)
	f.Meta = []MetaEsqueleto{{Ancho: 44, Oculta: true}, {Ancho: 76}}
	f.Resize(fyne.NewSize(400, f.MinSize().Height))
	r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
	defer f.Parar()
	r.Layout(f.Size())
	fila := r.filas[0]
	if fila.meta[0].Visible() {
		t.Error("la columna oculta se pinta")
	}
	if fin := fila.meta[1].Position().X + fila.meta[1].Size().Width; fin != 400-MargenFila {
		t.Errorf("la meta acaba en %g y tiene que acabar en el px-3 de la fila (%g)", fin, 400-MargenFila)
	}
	if fin := fila.titulo.Position().X + fila.titulo.Size().Width; fin > fila.meta[1].Position().X {
		t.Errorf("el título acaba en %g, encima de la meta que empieza en %g", fin, fila.meta[1].Position().X)
	}
}

// TestLasFilasEsqueletoSeQuedanQuietasSinMovimiento: con `prefers-reduced-motion` no hay
// latido y los bloques se quedan opacos.
func TestLasFilasEsqueletoSeQuedanQuietasSinMovimiento(t *testing.T) {
	SinMovimiento = true
	defer func() { SinMovimiento = false }()
	f := NuevasFilasEsqueleto(3, SinDelantero, 1)
	r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
	if f.anim != nil {
		t.Error("con SinMovimiento las filas han arrancado el latido")
	}
	if c := r.bloques[0].FillColor; c != tintaEsqueleto("", 0) {
		t.Errorf("quietos, los bloques pintan %v y tienen que ser opacos", c)
	}
}

// TestLasFilasEsqueletoSinUltimoFilete: con `last:border-b-0` la última fila no lleva
// filete, que dentro de una caja con borde sería una raya doble abajo.
func TestLasFilasEsqueletoSinUltimoFilete(t *testing.T) {
	f := NuevasFilasEsqueleto(3, SinDelantero, 2)
	f.Filete, f.SinUltimoFilete = true, true
	r := test.WidgetRenderer(f).(*filasEsqueletoRenderer)
	defer f.Parar()
	for i, fila := range r.filas {
		if quiere := i < 2; (fila.filete != nil) != quiere {
			t.Errorf("la fila %d lleva filete: %v, y tiene que llevarlo: %v", i, fila.filete != nil, quiere)
		}
	}
}
