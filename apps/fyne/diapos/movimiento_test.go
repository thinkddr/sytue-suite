// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

// TestLasTrayectoriasSonLasDeLaWeb: los vértices de `motionPoints` (la recta de 30, la
// diagonal de 21, el circuito que cierra en el origen, el arco de 12 tramos por
// (20, 26) y la personalizada saneada) y `motionRestOffset`, que suma los finales de
// las ya disparadas.
func TestLasTrayectoriasSonLasDeLaWeb(t *testing.T) {
	if got := PuntosMovimiento("motion-right", nil); !slices.Equal(got, []Punto{{}, {DX: 30}}) {
		t.Errorf("motion-right = %v", got)
	}
	if got := PuntosMovimiento("motion-down-left", nil); got[1] != (Punto{DX: -21, DY: 21}) {
		t.Errorf("motion-down-left = %v", got)
	}
	if got := PuntosMovimiento("motion-loop", nil); len(got) != 5 || got[4] != (Punto{}) {
		t.Errorf("el circuito tiene que volver al origen: %v", got)
	}
	arco := PuntosMovimiento("motion-arc", nil)
	if len(arco) != 13 || arco[6] != (Punto{DX: 20, DY: 13}) || arco[12] != (Punto{DX: 40}) {
		t.Errorf("el arco = %v", arco)
	}
	if got := PuntosMovimiento("motion-custom", []Punto{{DX: 5}}); !slices.Equal(got, []Punto{{}, {DX: 25, DY: 15}}) {
		t.Errorf("una personalizada de un punto es la diagonal de arranque: %v", got)
	}
	if got := PuntosMovimiento("motion-custom", []Punto{{DX: 3, DY: 3}, {DX: 300, DY: -5}}); got[0] != (Punto{}) || got[1].DX != 100 {
		t.Errorf("la personalizada empieza en el origen y se topa a 100: %v", got)
	}
	suyas := []Planeada{
		{Anim: Anim{Tipo: "fade"}, Categoria: Entrada, Paso: 1},
		{Anim: Anim{Tipo: "motion-right"}, Categoria: Movimiento, Paso: 2},
		{Anim: Anim{Tipo: "motion-down"}, Categoria: Movimiento, Paso: 3},
	}
	if d := DesfaseReposo(suyas, 2); d != (Punto{DX: 30}) {
		t.Errorf("tras el paso 2 la caja está desplazada %v", d)
	}
	if d := DesfaseReposo(suyas, 3); d != (Punto{DX: 30, DY: 30}) {
		t.Errorf("tras el paso 3, %v", d)
	}
}

// TestLaCajaSeQuedaDondeLaDejaLaTrayectoria: al presentar, una caja con «Mover a la
// derecha» está en su sitio antes del clic y 30 puntos a la derecha después, que es la
// traslación estática de la web.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `DesfaseReposo` en `cajasVisibles`, la
// caja volvía a x = 10 tras el clic.
func TestLaCajaSeQuedaDondeLaDejaLaTrayectoria(t *testing.T) {
	m := diapoDe(t, `{"id":"c","x":10,"y":20,"w":20,"h":10,"html":"<p>c</p>","anims":[{"type":"motion-right","order":1}]}`)
	l := NuevoLienzo(m, 0)
	l.ConPasos = true
	if c := l.cajasVisibles(); c[0].X != 10 {
		t.Errorf("antes del clic la caja está en x = %v", c[0].X)
	}
	l.Paso = 1
	if c := l.cajasVisibles(); c[0].X != 40 || c[0].Y != 20 {
		t.Errorf("tras el clic la caja está en (%v, %v) y tiene que quedarse en (40, 20)", c[0].X, c[0].Y)
	}
}

// TestLaTrayectoriaDeLaCajaElegidaSeVeYSeReorienta: con la caja elegida, su trayectoria
// sale en el escenario con el final como tirador; arrastrarlo la hace personalizada con
// el último vértice movido (la web: `setMotionPoints` con `motion-custom`).
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin escribir los puntos, el arrastre dejaba
// «motion-right» tal cual.
func TestLaTrayectoriaDeLaCajaElegidaSeVeYSeReorienta(t *testing.T) {
	e, _, _ := montar(t, `{"slides":[{"id":"a","boxes":[
		{"id":"c","x":10,"y":20,"w":20,"h":10,"html":"<p>c</p>","anims":[{"type":"motion-right","order":1}]}]}]}`)
	if n := len(buscar[*asaRedonda](e.capa)); n != 0 {
		t.Fatalf("sin elegir la caja no hay trayectoria (%d tiradores)", n)
	}
	e.SeleccionarSolo("c")
	asas := buscar[*asaRedonda](e.capa)
	if len(asas) != 1 {
		t.Fatalf("la trayectoria elegida lleva %d tiradores y es uno, el final", len(asas))
	}
	k := e.escenario.Escala
	centro := asas[0].Position().Add(fyne.NewPos(asas[0].Size().Width/2, asas[0].Size().Height/2))
	// El centro de la caja (20 %, 25 %) más los 30 puntos de la recta.
	quiero := fyne.NewPos(50.0/100*AnchoEscenario*k, 25.0/100*e.m.AltoEscenario()*k)
	if d := centro.Subtract(quiero); d.X*d.X+d.Y*d.Y > 0.01 {
		t.Errorf("el final está en %v y tiene que estar en %v", centro, quiero)
	}
	asas[0].Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(0, e.m.AltoEscenario()*k/10)})
	asas[0].DragEnd()
	a := AnimsDe(*e.m.caja(0, "c"))[0]
	if a.Tipo != "motion-custom" || !slices.Equal(a.Puntos, []Punto{{}, {DX: 30, DY: 10}}) {
		t.Errorf("arrastrar el final deja %s %v", a.Tipo, a.Puntos)
	}
}

// TestLasEntradasSeAnimanComoEnLaWeb: al revelar un paso, «fundido» empieza
// transparente, «volar» además 26 px más abajo (a la escala de la presentación) y la
// trayectoria desde donde estaba la caja antes de moverse; todo acaba en su reposo. Al
// retroceder no se anima nada, como `boxFiringAtStep`. Las curvas son las de CSS.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `animarPaso`, no había efectos que
// arrancar y el primer fotograma ya era el reposo.
func TestLasEntradasSeAnimanComoEnLaWeb(t *testing.T) {
	m, err := Leer([]byte(`{"slides":[{"id":"a","boxes":[
		{"id":"f","x":0,"y":0,"w":40,"h":20,"html":"<p>fundido</p>","anims":[{"type":"fade","order":1}]},
		{"id":"v","x":0,"y":40,"w":40,"h":20,"html":"<p>vuela</p>","anims":[{"type":"fly","order":2,"trigger":"with"}]},
		{"id":"m","x":50,"y":40,"w":20,"h":20,"kind":"shape","shape":"rect","html":"","anims":[{"type":"motion-right","order":3}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPresentacion(m, 0, traductor(t))
	w := test.NewTempWindow(t, p)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(960, 540))
	texto := func(id string) *canvas.Text {
		_, tx, _ := textoDe(t, p.lienzo.r.porCaja[id])
		return tx
	}
	sitio := func(id string) fyne.Position {
		_, _, en := textoDe(t, p.lienzo.r.porCaja[id])
		return en()
	}
	p.Avanzar(1)
	if len(p.efectos) != 2 {
		t.Fatalf("el paso 1 dispara %d efectos y son dos (fundido y, con él, volar)", len(p.efectos))
	}
	base := sitio("v")
	for _, a := range p.efectos {
		a.Tick(0)
	}
	if a := texto("f").Color.(color.NRGBA).A; a != 0 {
		t.Errorf("el fundido empieza con alfa %d", a)
	}
	if dy := sitio("v").Y - base.Y; dy != 26 || texto("v").Color.(color.NRGBA).A != 0 {
		t.Errorf("volar empieza %v px más abajo (quiero 26) y transparente", dy)
	}
	if got := salidaCSS(0.5); got < 0.6 || got > 0.8 {
		t.Errorf("ease-out a la mitad da %v; CSS da 0,69", got)
	}
	for _, a := range p.efectos {
		a.Tick(1)
	}
	if sitio("v") != base || texto("f").Color.(color.NRGBA).A != 255 {
		t.Error("al acabar, las dos cajas están en su reposo")
	}
	p.Avanzar(1)
	if len(p.efectos) != 1 {
		t.Fatalf("el paso 2 dispara %d efectos y es uno, la trayectoria", len(p.efectos))
	}
	forma := p.lienzo.r.porCaja["m"][0]
	p.efectos[0].Tick(0)
	if dx := forma.o.Position().X - forma.pos.X; dx != -30.0/100*960 {
		t.Errorf("la trayectoria empieza %v px antes de su reposo y son −288", dx)
	}
	p.Avanzar(-1)
	if len(p.efectos) != 0 {
		t.Error("al retroceder no se anima nada")
	}
}

// TestLasSalidasYElEnfasisSeAnimanComoEnLaWeb: la caja que sale con «desvanecer» se
// sigue pintando ese paso y su efecto la lleva de opaca a invisible; el «latido» llega a
// 1,06 a la mitad y vuelve a 1, alrededor del centro de la caja.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `Saliendo`, la caja que sale no se
// pintaba en su paso —cero primitivas— y desaparecía de golpe.
func TestLasSalidasYElEnfasisSeAnimanComoEnLaWeb(t *testing.T) {
	m, err := Leer([]byte(`{"slides":[{"id":"a","boxes":[
		{"id":"s","x":0,"y":0,"w":40,"h":20,"html":"<p>sale</p>","anims":[{"type":"fade-out","order":1}]},
		{"id":"p","x":0,"y":40,"w":40,"h":20,"html":"<p>late</p>","anims":[{"type":"pulse","order":2,"trigger":"with"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := NuevaPresentacion(m, 0, traductor(t))
	w := test.NewTempWindow(t, p)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(960, 540))
	p.Avanzar(1)
	sale := p.lienzo.r.porCaja["s"]
	if len(sale) == 0 || len(p.efectos) != 2 {
		t.Fatalf("la caja que sale se pinta en su paso (%d primitivas) y hay dos efectos (%d)", len(sale), len(p.efectos))
	}
	_, tx, _ := textoDe(t, sale)
	p.efectos[0].Tick(0)
	if tx.Color.(color.NRGBA).A != 255 {
		t.Error("la salida empieza opaca")
	}
	p.efectos[0].Tick(1)
	if tx.Color.(color.NRGBA).A != 0 {
		t.Error("la salida acaba invisible")
	}
	late, lt, sitio := textoDe(t, p.lienzo.r.porCaja["p"])
	reposo := sitio()
	p.efectos[1].Tick(0.5)
	if s := lt.TextSize / late.letra; s < 1.059 || s > 1.061 {
		t.Errorf("el latido a la mitad escala %v y son 1,06", s)
	}
	p.efectos[1].Tick(1)
	if lt.TextSize != late.letra || sitio() != reposo {
		t.Error("el latido vuelve a su tamaño y su sitio")
	}
	p.Avanzar(-1)
	if p.lienzo.Saliendo != nil {
		t.Error("al retroceder no sale nada")
	}
}

// textoDe es la primera letra de una caja: su primitiva, su texto y dónde se ve. Una caja
// de texto va dentro de su recorte (`recortada`), y la posición de la letra es relativa a
// él: lo que se ve es la del recorte más la suya.
func textoDe(t *testing.T, ps []primitiva) (primitiva, *canvas.Text, func() fyne.Position) {
	t.Helper()
	var recorte fyne.CanvasObject
	for _, pr := range ps {
		if !pr.local {
			recorte = pr.o
		}
		if tx, ok := pr.o.(*canvas.Text); ok {
			r, local := recorte, pr.local
			return pr, tx, func() fyne.Position {
				if local && r != nil {
					return r.Position().Add(tx.Position())
				}
				return tx.Position()
			}
		}
	}
	t.Fatal("la caja no tiene texto")
	return primitiva{}, nil, nil
}
