// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"

	"fyne.io/fyne/v2"
)

func TestCancelarElRecorteRecuperaElMarcoAnterior(t *testing.T) {
	for _, conMarco := range []bool{false, true} {
		l := NuevoLienzoEditor()
		l.Recortando, l.tamPx = true, fyne.NewSize(400, 300)
		anterior := Marco{X: 0.1, Y: 0.1, W: 0.5, H: 0.5}
		if conMarco {
			l.Marco = &anterior
		}
		ev := &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(300, 200)}, Dragged: fyne.NewDelta(100, 100)}
		l.Dragged(ev)
		l.CancelarArrastre()
		l.Dragged(ev)
		l.DragEnd()
		if (l.Marco != nil) != conMarco || (conMarco && *l.Marco != anterior) {
			t.Fatalf("cancelar no recuperó el marco anterior: %v", l.Marco)
		}
		l.Dragged(ev)
		if !l.activo {
			t.Fatal("el gesto nuevo quedó bloqueado")
		}
		l.DragEnd()
	}
}

func TestCancelarLaFichaNoGuardaSuMedida(t *testing.T) {
	var medida float32
	guardados := 0
	a := NuevaAsaFicha(vistaDePrueba(t, nil), Visor{AnchoFicha: 340, AlAnchoFicha: func(m float32, guardar bool) {
		medida = m
		if guardar {
			guardados++
		}
	}})
	ev := &fyne.DragEvent{Dragged: fyne.NewDelta(-80, 0)}
	a.Dragged(ev)
	a.CancelarArrastre()
	a.Dragged(ev)
	a.DragEnd()
	if medida != 340 || guardados != 0 {
		t.Fatalf("cancelar la ficha dejó medida=%g y guardados=%d", medida, guardados)
	}
	a.Dragged(ev)
	a.DragEnd()
	if medida != 420 || guardados != 1 {
		t.Fatal("el gesto nuevo no pudo guardar la medida")
	}
}

func TestCancelarUnDeslizamientoNoPasaDeFoto(t *testing.T) {
	conTelefono(t)
	pasos := 0
	l := &LienzoVisor{}
	l.ExtendBaseWidget(l)
	d := NuevoDeslizaFoto(l, nil, func() { pasos++ }, nil)
	d.Resize(fyne.NewSize(390, 600))
	ev := &fyne.DragEvent{Dragged: fyne.NewDelta(-120, 0)}
	d.Dragged(ev)
	d.CancelarArrastre()
	d.Dragged(ev)
	d.DragEnd()
	if pasos != 0 || l.Position().X != 0 {
		t.Fatal("cancelar pasó de foto o dejó la foto desplazada")
	}
	d.Dragged(ev)
	d.DragEnd()
	if pasos != 1 {
		t.Fatal("el deslizamiento posterior quedó bloqueado")
	}
}
