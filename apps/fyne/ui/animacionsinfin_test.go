// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLasAnimacionesDeCargaSeDuermenFueraDelArbol: una rueda o un esqueleto que se quita
// del árbol, o se esconde, deja de ticar en dos fotogramas como mucho, y vuelve a latir
// —en la fase en que se quedó— cuando se pinta otra vez. Un Parar explícito no lo
// despierta nada.
//
// El driver de pruebas no tica animaciones, así que cada fotograma se hace a mano como
// lo hace el de verdad: primero tica la animación y después se pinta la ventana.
//
// VISTO EN ROJO (2026-11-24): con `pintada` vacía —el aviso del pintor que no llega—, las
// cuatro se duermen aunque estén en la ventana; y sin la cuenta de fotogramasSinPintar,
// la huérfana sigue ticando los veinte fotogramas.
func TestLasAnimacionesDeCargaSeDuermenFueraDelArbol(t *testing.T) {
	casos := []struct {
		nombre string
		nuevo  func() (fyne.CanvasObject, *animacionSinFin, func() float32)
	}{
		{"esqueleto", func() (fyne.CanvasObject, *animacionSinFin, func() float32) {
			e := NuevoEsqueleto(80, 0)
			return e, &e.latido, func() float32 { e.mu.Lock(); defer e.mu.Unlock(); return e.fase }
		}},
		{"rueda", func() (fyne.CanvasObject, *animacionSinFin, func() float32) {
			r := NuevaRueda(0, "primary", tema.Claro)
			return r, &r.giro, func() float32 { r.mu.Lock(); defer r.mu.Unlock(); return float32(r.angulo) }
		}},
		{"filas", func() (fyne.CanvasObject, *animacionSinFin, func() float32) {
			f := NuevasFilasEsqueleto(3, DelanteroAvatar, 2)
			return f, &f.animacionSinFin, func() float32 { f.mu.Lock(); defer f.mu.Unlock(); return f.fase }
		}},
		{"bloques", func() (fyne.CanvasObject, *animacionSinFin, func() float32) {
			b := NuevosBloquesEsqueleto()
			b.Con(container.NewStack(b.Bloque(0, 0)))
			return b, &b.animacionSinFin, func() float32 { b.mu.Lock(); defer b.mu.Unlock(); return b.fase }
		}},
		// La rueda de sincronización del lateral (el «Actualizar» de Mail): un menú
		// rehecho la deja fuera del árbol mientras gira. VISTO EN ROJO con la
		// `fyne.Animation` de antes, que solo paraba en Destroy.
		{"accion lateral", func() (fyne.CanvasObject, *animacionSinFin, func() float32) {
			a := NuevaAccionLateralCon("Actualizar", iconos.Trazo(iconos.Rotar, 16, "sidebar-fg-muted", tema.Claro), nil)
			a.Girando = true
			return a, &a.giro, func() float32 { a.giro.mu.Lock(); defer a.giro.mu.Unlock(); return a.giro.ultima }
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			obj, a, fase := c.nuevo()
			w := test.NewWindow(container.NewStack(obj))
			defer w.Close()
			w.Resize(fyne.NewSize(240, 160))
			p := float32(0)
			ticar := func() {
				a.mu.Lock()
				anim := a.anim
				a.mu.Unlock()
				if anim != nil {
					p += 0.05
					anim.Tick(p - float32(int(p)))
				}
			}
			fotograma := func() {
				ticar()
				w.Canvas().Capture()
			}

			w.Canvas().Capture()
			for range 20 {
				fotograma()
			}
			if !a.andando() {
				t.Fatal("en pantalla se ha dormido: el pintor no avisa de que la pinta")
			}

			// Fuera del árbol: tica dos fotogramas como mucho (el primero aún lleva el aviso
			// del último pintado en el árbol) y al siguiente se duerme sin mover nada.
			w.SetContent(canvas.NewRectangle(color.Transparent))
			ticados := 0
			for i := 0; a.andando() && i < 20; i++ {
				antes := fase()
				fotograma()
				if fase() != antes {
					ticados++
				}
			}
			if a.andando() || ticados > fotogramasSinPintar {
				t.Fatalf("fuera del árbol sigue ticando: %d fotogramas (andando %v)", ticados, a.andando())
			}
			quieta := fase()
			for range 5 {
				fotograma()
			}
			if fase() != quieta {
				t.Error("dormida, la animación sigue moviendo su fase")
			}

			// De vuelta: se despierta al pintarse, donde se quedó.
			w.SetContent(container.NewStack(obj))
			w.Canvas().Capture()
			if !a.andando() {
				t.Fatal("de vuelta en el árbol no se despierta")
			}
			a.mu.Lock()
			antes := a.ultima
			anim := a.anim
			a.mu.Unlock()
			anim.Tick(0)
			if a.ultima != antes {
				t.Errorf("al despertar salta de la fase %g a la %g", antes, a.ultima)
			}

			// Escondida: igual que fuera del árbol.
			obj.Hide()
			for range fotogramasSinPintar + 1 {
				fotograma()
			}
			if a.andando() {
				t.Error("escondida sigue ticando")
			}
			obj.Show()
			w.Canvas().Capture()
			if !a.andando() {
				t.Error("enseñada otra vez no se despierta")
			}

			// Parada a propósito, no la despierta ningún pintado.
			a.parar()
			w.Canvas().Capture()
			if a.andando() {
				t.Error("tras parar, volver a pintarse la ha despertado")
			}
		})
	}
}
