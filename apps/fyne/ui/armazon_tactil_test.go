// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// El teléfono y la tablet conservan la selección, el ancho manual del panel y el
// retorno físico al girar entre retrato (< 768) y paisaje (≥ 768).
func TestElMenuTactilConservaColumnasEstadoYFocoAlGirar(t *testing.T) {
	enMarco(t)
	sinAnimacionDelMenu(t)
	t.Cleanup(TactilDePrueba(true))
	for _, ciclo := range []struct {
		nombre  string
		tamanos []fyne.Size
	}{
		{nombre: "telefono", tamanos: []fyne.Size{{Width: 320, Height: 700}, {Width: 844, Height: 390}, {Width: 320, Height: 700}}},
		{nombre: "tablet", tamanos: []fyne.Size{{Width: 600, Height: 900}, {Width: 768, Height: 1024}, {Width: 1024, Height: 768}, {Width: 600, Height: 900}}},
	} {
		for _, propia := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/barraPropia=%v", ciclo.nombre, propia), func(t *testing.T) {
				fuera := NuevoBoton("Detalle seleccionado", nil)
				uno := NuevaFilaLateral("Recibidos", nil, nil)
				a := NuevoArmazon(fuera)
				a.Menu = NuevoLateral("Navegación", container.New(Columna{}, uno))
				a.Panel = contenidoDePrueba()
				a.AnchoPanelPersonalizado = 600
				a.DetalleAbierto = true
				vueltas := 0
				a.AlVolver = func() { vueltas++ }
				if propia {
					b := NuevaBarraMovil()
					b.EnDetalle = true
					b.AlMenu = func() {
						if a.CajonAbierto {
							a.CerrarCajon()
						} else {
							a.AbrirCajon()
						}
					}
					b.AlVolver = func() { vueltas++; a.DetalleAbierto, b.EnDetalle = false, false; a.Refresh() }
					a.BarraMovil = b
				}
				w := test.NewTempWindow(t, a)
				w.SetPadded(false)
				c := w.Canvas()

				for _, tam := range ciclo.tamanos {
					w.Resize(tam)
					a.Resize(tam)
					a.Refresh()
					b := a.barraMovil().(*BarraMovil)
					r := test.WidgetRenderer(a).(*armazonRenderer)

					if tam.Width < Estrecho {
						if !b.Visible() || !b.EnDetalle || b.AlVolver == nil || b.boton.Icono != iconos.ChevronIzquierda {
							t.Fatalf("%v: la barra móvil perdió el estado de detalle o el botón de volver", tam)
						}
						if b.Size() != fyne.NewSize(tam.Width, AltoBarraMovil) {
							t.Fatalf("%v: barra móvil=%v, quiero %g×%g", tam, b.Size(), tam.Width, AltoBarraMovil)
						}
						alto := tam.Height - AltoBarraMovil
						if a.Panel.Visible() || !fuera.Visible() ||
							fuera.Position() != fyne.NewPos(0, AltoBarraMovil) || fuera.Size() != fyne.NewSize(tam.Width, alto) ||
							r.marco.hoja.Position() != fyne.NewPos(0, AltoBarraMovil) || r.marco.hoja.Size() != fyne.NewSize(tam.Width, alto) {
							t.Fatalf("%v: el detalle móvil no ocupa la hoja completa bajo la barra", tam)
						}

						a.AbrirCajon()
						if !a.CajonAbierto || !a.Menu.Visible() || fuera.Size().Width != tam.Width {
							t.Fatalf("%v: abrir el cajón encogió o desplazó el detalle", tam)
						}
						for range 6 {
							c.FocusNext()
							if c.Focused() == fyne.Focusable(fuera) {
								t.Fatal("el Tab sale del menú y alcanza el detalle bajo el velo")
							}
						}
						Descartar(c)
						if a.CajonAbierto || c.Focused() != fyne.Focusable(b.boton) {
							t.Fatal("Escape no cierra el cajón o no devuelve el foco al mando")
						}
						// Dejarlo abierto antes de girar a paisaje: al cruzar ≥ 768 debe cerrarse solo.
						a.AbrirCajon()
					} else {
						if a.CajonAbierto {
							t.Fatalf("%v: cruzar a ≥ 768 no cerró el cajón móvil", tam)
						}
						if b.Visible() {
							t.Fatalf("%v: la barra móvil sigue visible en ancho", tam)
						}
						separa := tema.Forma("sheet-gap")
						hoja := tam.Width - tema.AnchoLateral - separa
						anchoPanel := AcotarAnchoPanel(600, hoja)
						anchoDetalle := hoja - anchoPanel
						if !a.Menu.Visible() || a.Menu.Position() != fyne.NewPos(0, 0) || a.Menu.Size() != fyne.NewSize(tema.AnchoLateral, tam.Height) {
							t.Fatalf("%v: menú lateral fijo mal colocado en ancho: %v/%v", tam, a.Menu.Position(), a.Menu.Size())
						}
						if !a.Panel.Visible() || a.Panel.Position() != fyne.NewPos(tema.AnchoLateral, 0) || a.Panel.Size().Width != anchoPanel {
							t.Fatalf("%v: panel mal colocado en ancho: %v/%v (quiero ancho %g)", tam, a.Panel.Position(), a.Panel.Size(), anchoPanel)
						}
						if !fuera.Visible() || fuera.Position() != fyne.NewPos(tema.AnchoLateral+anchoPanel, 0) || fuera.Size().Width != anchoDetalle {
							t.Fatalf("%v: detalle mal colocado en ancho: %v/%v (quiero ancho %g)", tam, fuera.Position(), fuera.Size(), anchoDetalle)
						}
						if !a.DetalleAbierto || a.AnchoPanelPersonalizado != 600 || vueltas != 0 {
							t.Fatalf("%v: girar alteró la selección o el ancho personalizado", tam)
						}
					}
				}

				a.CerrarCajon()
				if !a.volverAlPanel() || a.DetalleAbierto || vueltas != 1 || !a.Panel.Visible() {
					t.Fatal("el retorno físico perdió AlVolver después de girar")
				}
			})
		}
	}
}

func TestElMenuDeEscritorioConservaMarcoYCarril(t *testing.T) {
	sinAnimacionDelMenu(t)
	t.Cleanup(TactilDePrueba(false))
	t.Cleanup(tema.FormaDeLosGuardianes)
	for _, forma := range []string{"marco", "carril"} {
		for _, conCarril := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/carrilIconos=%v", forma, conCarril), func(t *testing.T) {
				tema.ElegirForma(forma)
				a := NuevoArmazon(contenidoDePrueba())
				a.Menu = NuevoLateral("Navegación", nil)
				a.Panel = contenidoDePrueba()
				x := a.Menu.MinSize().Width
				if conCarril {
					carril := contenidoDePrueba()
					carril.SetMinSize(fyne.NewSize(64, 0))
					a.Carril, x = carril, 64
				}
				a.Resize(fyne.NewSize(1024, 768))
				a.Refresh()
				separa := a.separacionHoja(false)
				panel, detalle := a.Panel.Size(), a.Contenido.Size()
				if a.barra.Visible() || a.Menu.Visible() == conCarril ||
					a.Panel.Position().X != x || a.Contenido.Position().X != x+panel.Width ||
					detalle.Width != 1024-x-panel.Width-separa {
					t.Fatal("el menú móvil cambió la disposición del escritorio")
				}
				if conCarril {
					a.AbrirCajon()
					if !a.Menu.Visible() || a.Panel.Size() != panel || a.Contenido.Size() != detalle || a.Panel.Position().X != x {
						t.Fatal("el cajón del carril de iconos ha desplazado o encogido las columnas")
					}
					a.CerrarCajon()
				}
			})
		}
	}
}
