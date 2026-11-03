// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

func TestLaRespuestaTactilNoCapturaElScrollNiDisparaLaAccion(t *testing.T) {
	boton := NuevoBoton("Guardar", func() { t.Fatal("el filtro ha ejecutado la acción") })
	icono := NuevoBotonIcono(iconos.Mas, "Añadir", func() { t.Fatal("el filtro ha ejecutado la acción") })
	fila := NuevaFilaLateral("Carpeta", nil, func() { t.Fatal("el filtro ha ejecutado la acción") })
	for _, tc := range []struct {
		nombre string
		obj    pulsableMovil
		estado func() bool
	}{
		{"botón", boton, func() bool { return boton.pulsado }},
		{"icono", icono, func() bool { return icono.pulsado }},
		{"navegación", fila, func() bool { return fila.pulsado }},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			if _, captura := tc.obj.(mobile.Touchable); captura {
				t.Fatal("el estado visual intercepta el desplazamiento táctil")
			}
			for _, fin := range []mobile.FilterPhase{mobile.FilterEnd, mobile.FilterCancel, mobile.FilterMove} {
				g := &gestosMoviles{}
				g.observarPulsacion(eventoGesto(mobile.FilterBegin, 40, 40, tc.obj), mobile.FilterPass)
				if !tc.estado() {
					t.Fatal("no hay respuesta visual al apoyar el dedo")
				}
				g.observarPulsacion(eventoGesto(fin, 40, 70, tc.obj), mobile.FilterPass)
				if tc.estado() {
					t.Fatal("la pulsación queda encendida al soltar, cancelar o desplazar")
				}
			}
		})
	}
	boton.Inerte = true
	g := &gestosMoviles{}
	g.observarPulsacion(eventoGesto(mobile.FilterBegin, 40, 40, boton), mobile.FilterPass)
	if boton.pulsado {
		t.Fatal("el botón deshabilitado se pinta como pulsado")
	}
}

func TestElAsaCancelaSinGuardarYRevalidaAntesDeSoltar(t *testing.T) {
	asa := NuevaAsaArrastre("Mover")
	w := test.NewWindow(asa)
	t.Cleanup(w.Close)
	permitido, guardados, cancelados := true, 0, 0
	asa.Puede = func() bool { return permitido }
	asa.AlSoltar = func(fyne.Position) { guardados++ }
	asa.AlCancelar = func() { cancelados++ }
	e := &fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: fyne.NewPos(40, 50)}, Dragged: fyne.NewDelta(0, 10)}
	asa.Dragged(e)
	asa.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	// El dedo sigue apoyado: cancelar no puede rearmar el mismo gesto.
	asa.Dragged(e)
	asa.DragEnd()
	if guardados != 0 || cancelados != 1 {
		t.Fatalf("Escape guardó el movimiento: guardados=%d cancelados=%d", guardados, cancelados)
	}
	asa.Dragged(e)
	permitido = false
	asa.DragEnd()
	if guardados != 0 || cancelados != 2 {
		t.Fatal("guardó después de perder la capacidad de mover")
	}
	permitido = true
	asa.Dragged(e)
	asa.DragEnd()
	asa.DragEnd()
	if guardados != 1 {
		t.Fatalf("debe guardar una sola vez al soltar, no %d", guardados)
	}
}

func TestElBordeSigueDesplazandoSinMoverElDedoYSeDetieneAlCancelar(t *testing.T) {
	zona := canvas.NewRectangle(color.Transparent)
	w := test.NewWindow(zona)
	w.Resize(fyne.NewSize(200, 200))
	t.Cleanup(w.Close)
	var auto AutoScrollArrastre
	t.Cleanup(auto.Parar)
	pasos := 0
	abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(zona)
	pos := abs.AddXY(zona.Size().Width/2, zona.Size().Height-1)
	auto.Actualizar(zona, pos, func(_, dy float32) {
		if dy <= 0 {
			t.Error("el borde inferior desplaza en el sentido incorrecto")
		}
		pasos++
	})
	limite := time.Now().Add(time.Second)
	for pasos < 2 && time.Now().Before(limite) {
		time.Sleep(10 * time.Millisecond)
		fyne.DoAndWait(func() {})
	}
	if pasos < 2 {
		t.Fatal("el desplazamiento exige mover el dedo otra vez")
	}
	auto.Parar()
	antes := pasos
	time.Sleep(60 * time.Millisecond)
	fyne.DoAndWait(func() {})
	if pasos != antes {
		t.Fatal("la lista sigue desplazándose tras cancelar")
	}
}
