// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las mediciones previas de Notes, Contactos, Bases, ERP y Drive deben coincidir
// con el espacio que el renderer acaba asignando a sus objetos reales.
func TestLasMedidasPreviasSiguenAlArmazonAlGirar(t *testing.T) {
	t.Cleanup(tema.FormaDeLosGuardianes)
	sinAnimacionDelMenu(t)
	for _, tactil := range []bool{false, true} {
		restaurar := TactilDePrueba(tactil)
		for _, forma := range []string{"marco", "carril"} {
			tema.ElegirForma(forma)
			for _, carril := range []float32{0, 72} {
				for _, conPanel := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/tactil=%v/carril=%g/panel=%v", forma, tactil, carril, conPanel), func(t *testing.T) {
						a := NuevoArmazon(contenidoDePrueba())
						a.Menu = NuevoLateral("Menú", nil)
						a.DetalleAbierto = true
						if carril > 0 {
							c := canvas.NewRectangle(tema.Color("sidebar-bg", tema.Claro))
							c.SetMinSize(fyne.NewSize(carril, 0))
							a.Carril = c
						}
						if conPanel {
							a.Panel = contenidoDePrueba()
						}
						for _, ancho := range []float32{320, 568, 844, 1024, 320} {
							a.Resize(fyne.NewSize(ancho, 320))
							_ = test.WidgetRenderer(a)
							a.Refresh()
							hoja := AnchoHoja(ancho, AnchoMenu(false), carril)
							quiero := hoja
							if conPanel && ancho >= Estrecho {
								quiero -= AcotarAnchoPanel(AnchoPanel(ancho), hoja)
							}
							if got := a.Contenido.Size().Width; got != quiero {
								t.Fatalf("a%g: la medición previa reserva%g, el renderer asigna%g", ancho, quiero, got)
							}
							if a.menuEnCarril() {
								a.AbrirCajon()
								if a.Contenido.Size().Width != quiero {
									t.Fatal("abrir el menú cambió el ancho disponible")
								}
								a.CerrarCajon()
							}
						}
					})
				}
			}
		}
		restaurar()
	}
}
