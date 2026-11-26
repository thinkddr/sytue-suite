// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

// TestPulsarCierraLaPistaDeTodoControl es el `onPointerDown` de Tooltip.tsx en todos los
// controles que llevan pista, no solo en `BotonIcono`: ya se ha elegido, y la burbuja
// abierta al pasar se quedaba encima del menú o de la vista que la pulsación abría, hasta
// que el puntero saliera. En la web el foco del clic la volvía a abrir (la campana dejaba
// «Notificaciones» sobre su panel); aquí no había quien la cerrara. VISTO EN ROJO en los
// tres casos quitando la llamada a `alPulsar` de cada `Tapped`.
func TestPulsarCierraLaPistaDeTodoControl(t *testing.T) {
	casos := []struct {
		nombre string
		montar func(alPulsar func()) (control fyne.Tappable, pista func() *Pista, raiz fyne.CanvasObject)
	}{
		{"Boton", func(alPulsar func()) (fyne.Tappable, func() *Pista, fyne.CanvasObject) {
			b := NuevoBoton("Guardar", alPulsar)
			p := NuevaPista("Guardar el documento", b)
			return b, func() *Pista { return p }, p
		}},
		{"pulsable (FilaPulsable)", func(alPulsar func()) (fyne.Tappable, func() *Pista, fyne.CanvasObject) {
			f := NuevaFilaPulsable("Informe", canvas.NewRectangle(color.NRGBA{A: 255}), alPulsar)
			p := NuevaPista("Abrir el informe", f)
			return f, func() *Pista { return p }, p
		}},
		{"FilaLateral plegada", func(alPulsar func()) (fyne.Tappable, func() *Pista, fyne.CanvasObject) {
			f := NuevaFilaLateral("Recibidos", canvas.NewRectangle(color.NRGBA{A: 255}), alPulsar)
			f.Plegada = true
			return f, func() *Pista { return f.titulo }, f
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			ultimoCierrePista = time.Time{}
			pulsado := 0
			control, pista, raiz := c.montar(func() { pulsado++ })
			w := test.NewTempWindow(t, container.NewWithoutLayout(raiz))
			w.Resize(fyne.NewSize(400, 200))
			raiz.Resize(raiz.MinSize().Max(fyne.NewSize(64, 36)))
			raiz.Move(fyne.NewPos(40, 40))
			p := pista()
			if p == nil {
				t.Fatal("el control no tiene pista que probar")
			}
			// Abierta como la deja el puntero tras su retardo.
			p.Abrir()
			if !p.Abierta() {
				t.Fatal("la pista no se abre")
			}
			test.Tap(control)
			if pulsado != 1 {
				t.Fatalf("la pulsación llegó %d veces", pulsado)
			}
			if p.Abierta() {
				t.Error("tras pulsar el control su pista sigue abierta encima de lo que la pulsación abre")
			}
		})
	}
}
