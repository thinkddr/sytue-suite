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

// El teléfono y la tablet conservan la selección y su navegación al girar.
// Abrir el menú desplaza las columnas, sin cambiar su ancho ni el retorno físico.
func TestElMenuTactilConservaColumnasEstadoYFocoAlGirar(t *testing.T) {
	enMarco(t)
	sinAnimacionDelMenu(t)
	t.Cleanup(TactilDePrueba(true))
	for _, ciclo := range []struct {
		nombre  string
		tamanos []fyne.Size
	}{
		{nombre: "telefono", tamanos: []fyne.Size{{Width: 320, Height: 700}, {Width: 844, Height: 390}, {Width: 320, Height: 700}}},
		{nombre: "tablet", tamanos: []fyne.Size{{Width: 768, Height: 1024}, {Width: 1024, Height: 768}, {Width: 768, Height: 1024}}},
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
					b.AlMenu = func() { a.CajonAbierto = !a.CajonAbierto; a.Refresh() }
					b.AlVolver = func() { vueltas++; a.DetalleAbierto, b.EnDetalle = false, false; a.Refresh() }
					a.BarraMovil = b
				}
				w := test.NewTempWindow(t, a)
				w.SetPadded(false)
				c := w.Canvas()
				for paso, tam := range ciclo.tamanos {
					w.Resize(tam)
					a.Resize(tam)
					a.Refresh()
					if paso > 0 && !a.CajonAbierto {
						t.Fatal("girar ha cerrado el cajón táctil")
					}
					b := a.barraMovil().(*BarraMovil)
					if !a.menuEnCarril() || !b.Visible() || !b.EnDetalle || b.AlVolver == nil || vueltas != 0 {
						t.Fatalf("%v: se perdió el mando o el estado de la selección", tam)
					}
					icono := iconos.Menu
					if tam.Width < Estrecho {
						icono = iconos.ChevronIzquierda
					}
					if b.boton.Icono != icono {
						t.Fatalf("%v: el mando no ofrece menú con ambas columnas, o volver con detalle exclusivo", tam)
					}
					separa := float32(0)
					if tam.Width >= Estrecho {
						separa = tema.Forma("sheet-gap")
					}
					util := tam.Width - AnchoCarrilMovil - separa
					anchoPanel, anchoDetalle := util, util
					xDetalle := float32(0)
					if tam.Width >= Estrecho {
						anchoPanel = min(600, util-AnchoContenidoMinimo)
						anchoDetalle, xDetalle = util-anchoPanel, anchoPanel
					}
					r := test.WidgetRenderer(a).(*armazonRenderer)
					comprobar := func(desplaza float32) {
						t.Helper()
						x := AnchoCarrilMovil + desplaza
						if a.Panel.Position().X != x || a.Panel.Size().Width != anchoPanel ||
							fuera.Position().X != x+xDetalle || fuera.Size().Width != anchoDetalle ||
							r.marco.hoja.Position().X != x || r.marco.hoja.Size().Width != util {
							t.Fatalf("%v: cambian los anchos al desplazar %g: panel=%v/%v detalle=%v/%v hoja=%v/%v",
								tam, desplaza, a.Panel.Position(), a.Panel.Size(), fuera.Position(), fuera.Size(), r.marco.hoja.Position(), r.marco.hoja.Size())
						}
						if a.Panel.Visible() != (tam.Width >= Estrecho) || !fuera.Visible() ||
							b.Size() != fyne.NewSize(AnchoCarrilMovil, tam.Height) ||
							!a.DetalleAbierto || a.AnchoPanelPersonalizado != 600 {
							t.Fatal("se perdió la selección, las columnas o su ancho manual")
						}
					}
					if paso == 0 {
						comprobar(0)
						a.AbrirCajon()
					}
					anchoMenu := a.Menu.Size().Width
					comprobar(anchoMenu)
					a.entrada = -anchoMenu / 2
					a.Refresh()
					comprobar(anchoMenu / 2)
					a.entrada = 0
					for range 6 {
						c.FocusNext()
						if c.Focused() == fyne.Focusable(fuera) {
							t.Fatal("el Tab sale del menú y alcanza el detalle bajo el velo")
						}
					}
					Descartar(c)
					if a.CajonAbierto || c.Focused() != fyne.Focusable(b.boton) {
						t.Fatal("Escape no cierra o no devuelve el foco al mando")
					}
					comprobar(0)
					if tam.Width >= Estrecho {
						b.boton.AlPulsar()
					} else {
						a.AbrirCajon()
					}
					if !a.CajonAbierto || !a.Menu.Visible() || vueltas != 0 {
						t.Fatal("la hamburguesa ancha ha vuelto a la lista en lugar de abrir el menú")
					}
					if c.Focused() != fyne.Focusable(a.Menu.(*Lateral)) {
						t.Fatal("abrir el cajón no lleva el foco al menú")
					}
				}
				a.CerrarCajon()
				if !a.volverAlPanel() || a.DetalleAbierto || vueltas != 1 {
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
				if a.menuEnCarril() || a.barra.Visible() || a.Menu.Visible() == conCarril ||
					a.Panel.Position().X != x || a.Contenido.Position().X != x+panel.Width ||
					detalle.Width != 1024-x-panel.Width-separa {
					t.Fatal("el menú táctil cambió la disposición del escritorio")
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
