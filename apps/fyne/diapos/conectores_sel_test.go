// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// TestLosConectoresSeEligenComoEnLaWeb: una pulsación a menos de 7 px del trazo elige
// el conector (y suelta las cajas); lleva el halo del acento, un tirador en cada
// extremo LIBRE y, en la mini-barra, tipo, puntas, color y eliminar. El tirador mueve el
// extremo en % con una décima; Supr borra el conector y pulsar el fondo lo suelta.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin mirar los conectores en la pulsación
// del fondo, junto al trazo no se elegía nada.
func TestLosConectoresSeEligenComoEnLaWeb(t *testing.T) {
	e, ctx, w := montar(t, `{"slides":[{"id":"a","boxes":[
		{"id":"p","x":5,"y":5,"w":10,"h":10,"html":"<p>p</p>"}],
		"connectors":[{"id":"libre","kind":"straight","from":{"x":28,"y":42},"to":{"x":64,"y":62},"endArrow":true}]}]}`)
	k := e.escenario.Escala
	sh := e.m.AltoEscenario()
	medio := fyne.NewPos((28+64)/2.0/100*AnchoEscenario*k, (42+62)/2.0/100*sh*k)
	origen := fyne.CurrentApp().Driver().AbsolutePositionForObject(e.capa)
	halo := func() color.NRGBA {
		img := w.Canvas().Capture()
		esc := w.Canvas().Scale()
		p := origen.Add(medio).AddXY(0, 3*k)
		return color.NRGBAModel.Convert(img.At(int(p.X*esc), int(p.Y*esc))).(color.NRGBA)
	}
	antes := halo()

	e.SeleccionarSolo("p")
	e.capa.Tapped(&fyne.PointEvent{Position: medio.AddXY(0, 6*k)})
	if e.ConectorElegido() != "libre" || e.sel != "" || e.escenario.Conector != "libre" {
		t.Fatalf("la pulsación junto al trazo elige el conector: %q, cajas %q", e.ConectorElegido(), e.sel)
	}
	if despues := halo(); despues == antes {
		t.Errorf("el conector elegido no lleva halo: %v antes y después", antes)
	}
	var rotulos []string
	for _, o := range e.miniBarra.Objects {
		if b, ok := o.(*editor.BotonCinta); ok {
			rotulos = append(rotulos, b.Rotulo)
		}
	}
	if n := len(rotulos); n < 3 || rotulos[n-3] != "Punta de flecha al inicio" ||
		rotulos[n-2] != "Punta de flecha al final" || rotulos[n-1] != "Eliminar conector" {
		t.Errorf("la mini-barra del conector acaba en %v", rotulos)
	}
	asas := buscar[*asaRedonda](e.capa)
	if len(asas) != 2 {
		t.Fatalf("un conector libre lleva %d tiradores y son dos", len(asas))
	}
	final := asas[1]
	final.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(AnchoEscenario*k/10, 0)})
	final.DragEnd()
	if c := e.m.conector(0, "libre"); *c.Hasta.X != 74 || *c.Hasta.Y != 62 || ctx.Guardado != editor.Sucio {
		t.Errorf("el tirador del final lleva el extremo a (%v, %v)", *c.Hasta.X, *c.Hasta.Y)
	}
	e.m.AlternarPunta(0, "libre", false)
	if c := e.m.conector(0, "libre"); c.PuntaFin != nil {
		t.Error("apagar la punta la quita del JSON, como `patchConnector`")
	}
	guarda(t, "conector.png", w.Canvas().Capture())

	e.capa.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDelete})
	if e.m.conector(0, "libre") != nil || e.ConectorElegido() != "" {
		t.Error("Supr borra el conector elegido")
	}
	e.NuevoConector("elbow")
	if e.ConectorElegido() == "" {
		t.Fatal("el conector insertado queda elegido")
	}
	e.capa.Tapped(&fyne.PointEvent{Position: fyne.NewPos(2*k, 2*k)})
	if e.ConectorElegido() != "" {
		t.Error("pulsar el fondo suelta el conector")
	}
}
