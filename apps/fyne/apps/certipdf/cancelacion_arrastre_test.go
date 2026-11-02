// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestCancelarLaHerramientaNoEntregaUnaAnotacionFinal(t *testing.T) {
	test.NewTempApp(t)
	h := NuevaHoja(Pagina{Numero: 1, Ancho: 595, Alto: 842}, 1, HojaDibujada, "", nil)
	h.PonerHerramienta(Lapiz, colorCensura, 2)
	finales := 0
	h.AlGesto = func(_ *Hoja, _ Gesto, fin bool) {
		if fin {
			finales++
		}
	}
	tapa := nuevaTapa(h)
	ev := &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 100)}, Dragged: fyne.NewDelta(10, 10)}
	tapa.Dragged(ev)
	tapa.CancelarArrastre()
	tapa.Dragged(ev)
	tapa.DragEnd()
	if finales != 0 || h.gesto != nil {
		t.Fatal("cancelar entregó un gesto final al motor de anotaciones")
	}
	tapa.Dragged(ev)
	tapa.DragEnd()
	if finales != 1 {
		t.Fatal("el gesto posterior no se entregó una sola vez")
	}
}

func TestCancelarLaFirmaEliminaSoloElTrazoActual(t *testing.T) {
	test.NewTempApp(t)
	cambios := 0
	l := nuevoLienzoFirma(func() { cambios++ })
	l.Resize(fyne.NewSize(480, 148))
	ev := &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 70)}, Dragged: fyne.NewDelta(20, 0)}
	l.Dragged(ev)
	l.DragEnd()
	l.Dragged(ev)
	l.CancelarArrastre()
	l.Dragged(ev)
	l.DragEnd()
	if cambios != 1 || len(l.trazos) != 1 || l.dibujando {
		t.Fatalf("cancelar cambió la firma anterior: trazos=%d, avisos=%d", len(l.trazos), cambios)
	}
	l.Dragged(ev)
	l.DragEnd()
	if cambios != 2 || len(l.trazos) != 2 {
		t.Fatal("no se puede firmar después de cancelar un trazo")
	}
}
