// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

func TestCancelarElCalendarioNoCreaNiMueveEventos(t *testing.T) {
	for _, mover := range []bool{false, true} {
		p := semanaDeMuestra(t)
		c := cuerpoDe(t, p)
		_, _, ancho := c.geometria()
		desde := fyne.NewPos(AnchoEje+ancho*1.5, 600*PxPorMinuto)
		if mover {
			desde = fyne.NewPos(AnchoEje+ancho*2+10, 600*PxPorMinuto+5)
		}
		hasta := desde.AddXY(ancho, 120*PxPorMinuto)
		guardados := 0
		p.AlCrearRango = func(time.Time, time.Time) { guardados++ }
		p.AlMoverEvento = func(Ocurrencia, time.Time, time.Time) { guardados++ }
		ev := &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: hasta}, Dragged: fyne.NewDelta(hasta.X-desde.X, hasta.Y-desde.Y)}
		c.Dragged(ev)
		if c.arr == nil {
			t.Fatal("no comenzó el gesto")
		}
		c.CancelarArrastre()
		c.Dragged(ev)
		c.DragEnd()
		if guardados != 0 || c.arr != nil || p.pendiente != nil {
			t.Fatal("cancelar creó o movió un evento o dejó un fantasma")
		}
		c.Dragged(ev)
		c.DragEnd()
		if guardados != 1 {
			t.Fatal("el gesto nuevo no pudo confirmarse")
		}
	}
}
