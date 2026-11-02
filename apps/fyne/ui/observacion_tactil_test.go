// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
)

type canvasConObservacion struct {
	fyne.Canvas
	filtro     func(mobile.FilterEvent) mobile.FilterResult
	observador func(mobile.FilterEvent, mobile.FilterResult)
}

func (c *canvasConObservacion) SetTouchFilter(f func(mobile.FilterEvent) mobile.FilterResult) {
	c.filtro = f
}

func (c *canvasConObservacion) SetTouchObserver(f func(mobile.FilterEvent, mobile.FilterResult)) {
	c.observador = f
}

// La entrega real tras Pass y Cancel se prueba en el fork del driver. Aquí se
// comprueba que la suite instala ambos contratos, sin observar desde el filtro.
func TestLaSuiteInstalaObservacionSeparadaDelFiltro(t *testing.T) {
	boton := NuevoBoton("Guardar", func() { t.Fatal("observar no debe ejecutar acciones") })
	w := test.NewWindow(boton)
	t.Cleanup(w.Close)
	c := &canvasConObservacion{Canvas: w.Canvas()}
	g := &gestosMoviles{canvas: c}
	if !instalarGestosMoviles(c, g) || c.filtro == nil || c.observador == nil {
		t.Fatal("falta uno de los contratos móviles")
	}
	inicio := eventoGesto(mobile.FilterBegin, 40, 40, boton)
	if c.filtro(inicio) != mobile.FilterPass || boton.pulsado {
		t.Fatal("el filtro mezcla observación o retiene la pulsación")
	}
	c.observador(inicio, mobile.FilterPass)
	if !boton.pulsado {
		t.Fatal("el observador no refleja el inicio")
	}
	c.observador(eventoGesto(mobile.FilterEnd, 40, 40, boton), mobile.FilterPass)
	if boton.pulsado {
		t.Fatal("la respuesta visual queda retenida después de Pass")
	}
}

func TestObservarCancelImpideGuardarAntesDeDragEnd(t *testing.T) {
	asa := NuevaAsaArrastre("Mover")
	w := test.NewWindow(asa)
	t.Cleanup(w.Close)
	g := &gestosMoviles{}
	guardados := 0
	asa.AlSoltar = func(fyne.Position) { guardados++ }
	g.observarPulsacion(eventoGesto(mobile.FilterBegin, 40, 40, asa), mobile.FilterPass)
	asa.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(50, 60)}})
	g.observarPulsacion(eventoGesto(mobile.FilterCancel, 50, 60, asa), mobile.FilterPass)
	asa.DragEnd()
	if guardados != 0 || asa.pulsado {
		t.Fatal("una cancelación del sistema terminó guardando el gesto")
	}
}
