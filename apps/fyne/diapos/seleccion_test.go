// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

const conFormas = `{"slides":[{"id":"s","boxes":[` +
	`{"id":"t","x":10,"y":10,"w":40,"h":20,"html":"<p>hola</p>"},` +
	`{"id":"f","x":60,"y":50,"w":20,"h":20,"html":"","kind":"shape","shape":"rect"},` +
	`{"id":"g","x":10,"y":60,"w":20,"h":20,"html":"","kind":"shape","shape":"ellipse"}],` +
	`"connectors":[{"id":"k","kind":"straight","from":{"boxId":"f"},"to":{"boxId":"g"}}]}]}`

// casi compara dos % del lienzo: el arrastre pasa por px de pantalla en float32.
func casi(a, b float64) bool { return a-b < 1e-3 && b-a < 1e-3 }

// marcoDe busca el marco de una caja entre los hijos de la capa.
func marcoDe(t *testing.T, e *Editor, id string) *marcoCaja {
	t.Helper()
	for _, o := range test.WidgetRenderer(e.capa).Objects() {
		if m, ok := o.(*marcoCaja); ok && m.id == id {
			return m
		}
	}
	t.Fatalf("la caja %q no tiene marco en la capa", id)
	return nil
}

func asaDe(t *testing.T, e *Editor, redimensionar bool) *asa {
	t.Helper()
	for _, o := range test.WidgetRenderer(e.capa).Objects() {
		if a, ok := o.(*asa); ok && a.redimensionar == redimensionar {
			return a
		}
	}
	t.Fatal("la primaria no tiene sus tiradores")
	return nil
}

// TestSeEligeSeMueveYSeBorra es el ciclo de una forma en el escenario, contra
// `onHandleDown`/`onHandleMove`/`onStageKeyDown` de la web: pulsar la elige (y la
// mini-barra gana sus muestras y el enlace de acción), arrastrar la mueve en % del
// lienzo con el tope del borde, las flechas la empujan un 1 %, Mayús + flecha la
// redimensiona, Supr la borra con su conector y pulsar el fondo deselecciona.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el tope de la envolvente, arrastrar
// la forma 1000 px a la derecha la dejaba en x = 174 %, fuera del lienzo.
func TestSeEligeSeMueveYSeBorra(t *testing.T) {
	e, ctx, _ := montar(t, conFormas)
	antes := len(e.miniBarra.Objects)
	test.Tap(marcoDe(t, e, "f"))
	if e.sel != "f" {
		t.Fatalf("pulsar la forma no la elige (sel = %q)", e.sel)
	}
	// Divisor, relleno, borde, divisor y «Al hacer clic».
	if got := len(e.miniBarra.Objects) - antes; got != 5 {
		t.Errorf("la mini-barra gana %d controles con una forma elegida y son 5", got)
	}

	k := e.escenario.Escala
	m := marcoDe(t, e, "f")
	m.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: 96 * k, DY: 54 * k}}) // 10 % y 10 %
	m.DragEnd()
	if c := e.m.caja(0, "f"); !casi(c.X, 70) || !casi(c.Y, 60) {
		t.Errorf("tras arrastrar 10 %% la forma está en (%v, %v) y tocaba (70, 60)", c.X, c.Y)
	}
	if ctx.Guardado != editor.Sucio {
		t.Error("soltar tiene que marcar el documento como sin guardar")
	}
	m = marcoDe(t, e, "f")
	m.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: 1000 * k}})
	m.DragEnd()
	if c := e.m.caja(0, "f"); !casi(c.X, 80) {
		t.Errorf("un arrastre fuera del lienzo deja la forma en x = %v y el tope es 80", c.X)
	}

	e.capa.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if c := e.m.caja(0, "f"); !casi(c.Y, 59) {
		t.Errorf("↑ deja la forma en y = %v y empuja un 1 %% (59)", c.Y)
	}
	e.capa.KeyDown(&fyne.KeyEvent{Name: "LeftShift"})
	e.capa.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	e.capa.KeyUp(&fyne.KeyEvent{Name: "LeftShift"})
	if c := e.m.caja(0, "f"); c.W != 19 || !casi(c.X, 80) {
		t.Errorf("Mayús + ← deja la forma de %v de ancho en x = %v: redimensiona, no mueve", c.W, c.X)
	}

	e.capa.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDelete})
	if e.m.caja(0, "f") != nil || len(e.m.Diapos[0].Conectores) != 0 || e.sel != "" {
		t.Errorf("Supr no borra la forma con su conector (%d conectores, sel %q)",
			len(e.m.Diapos[0].Conectores), e.sel)
	}

	test.Tap(marcoDe(t, e, "g"))
	test.Tap(e.capa)
	if e.sel != "" {
		t.Error("pulsar el fondo del escenario deselecciona")
	}
}

// TestElTextoSeMuevePorSuBarra: el cuerpo de una caja de texto es para escribir y
// arrastrarlo no la mueve; la barra de arriba sí, y el cuadro de la esquina la
// redimensiona con el mínimo del 4 %.
func TestElTextoSeMuevePorSuBarra(t *testing.T) {
	e, _, _ := montar(t, conFormas)
	test.Tap(marcoDe(t, e, "t"))
	k := e.escenario.Escala
	m := marcoDe(t, e, "t")
	m.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: 96 * k}})
	m.DragEnd()
	if c := e.m.caja(0, "t"); !casi(c.X, 10) {
		t.Errorf("arrastrar el cuerpo de un texto lo ha movido a x = %v", c.X)
	}
	a := asaDe(t, e, false)
	a.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: 96 * k}})
	a.DragEnd()
	if c := e.m.caja(0, "t"); !casi(c.X, 20) {
		t.Errorf("la barra de mover deja el texto en x = %v y tocaba 20", c.X)
	}
	r := asaDe(t, e, true)
	r.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: -1000 * k, DY: 0}})
	r.DragEnd()
	if c := e.m.caja(0, "t"); !casi(c.W, 4) {
		t.Errorf("encoger del todo deja el texto de %v de ancho y el mínimo es 4", c.W)
	}
}

// TestMayusculasAñadenALaSeleccion: Mayús + clic añade (y quita) cajas, la primaria no
// se va, y mover arrastra a todas juntas.
func TestMayusculasAñadenALaSeleccion(t *testing.T) {
	e, _, _ := montar(t, conFormas)
	test.Tap(marcoDe(t, e, "f"))
	e.capa.mayus = true
	test.Tap(marcoDe(t, e, "g"))
	test.Tap(marcoDe(t, e, "f")) // la primaria no se quita con Mayús
	e.capa.mayus = false
	if e.sel != "f" || len(e.extra) != 1 || e.extra[0] != "g" {
		t.Fatalf("selección = %q + %v; tocaba f + [g]", e.sel, e.extra)
	}
	k := e.escenario.Escala
	m := marcoDe(t, e, "g")
	m.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DY: -54 * k}})
	m.DragEnd()
	if f, g := e.m.caja(0, "f"), e.m.caja(0, "g"); !casi(f.Y, 40) || !casi(g.Y, 50) {
		t.Errorf("mover la selección deja f en y=%v y g en y=%v; tocaba 40 y 50", f.Y, g.Y)
	}
}
