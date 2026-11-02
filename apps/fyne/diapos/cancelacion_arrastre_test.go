// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"testing"

	"fyne.io/fyne/v2"
)

func TestCancelarRestauraLasEdicionesProvisionalesDelEscenario(t *testing.T) {
	type cancelable interface {
		fyne.Draggable
		CancelarArrastre()
	}
	for _, caso := range []struct {
		nombre, fuente string
		elegir         func(*testing.T, *Editor) cancelable
	}{
		{"caja", conFormas, func(t *testing.T, e *Editor) cancelable {
			e.SeleccionarSolo("f")
			return marcoDe(t, e, "f")
		}},
		{"tamaño", conFormas, func(t *testing.T, e *Editor) cancelable {
			e.SeleccionarSolo("t")
			return asaDe(t, e, true)
		}},
		{"extremo", `{"slides":[{"id":"s","boxes":[],"connectors":[{"id":"c","kind":"straight","from":{"x":10,"y":10},"to":{"x":50,"y":50}}]}]}`, func(t *testing.T, e *Editor) cancelable {
			e.SeleccionarConector("c")
			return buscar[*asaRedonda](e.capa)[1]
		}},
		{"trayectoria", `{"slides":[{"id":"s","boxes":[{"id":"c","x":10,"y":20,"w":20,"h":10,"html":"<p>c</p>","anims":[{"type":"motion-right","order":1}]}]}]}`, func(t *testing.T, e *Editor) cancelable {
			e.SeleccionarSolo("c")
			return buscar[*asaRedonda](e.capa)[0]
		}},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			e, ctx, _ := montar(t, caso.fuente)
			g := caso.elegir(t, e)
			antes, estado := e.Serializar(), ctx.Guardado
			ev := &fyne.DragEvent{Dragged: fyne.NewDelta(40*e.escenario.Escala, 20*e.escenario.Escala)}
			g.Dragged(ev)
			if e.Serializar() == antes {
				t.Fatal("el gesto no alteró la previsualización")
			}
			g.CancelarArrastre()
			g.Dragged(ev)
			g.DragEnd()
			if e.Serializar() != antes || ctx.Guardado != estado {
				t.Fatal("cancelar dejó cambios o marcó el documento para guardar")
			}
			g.Dragged(ev)
			g.DragEnd()
			if e.Serializar() == antes || ctx.Guardado == estado {
				t.Fatal("el gesto posterior no pudo confirmarse")
			}
		})
	}
}
