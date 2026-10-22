// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// No basta con que el token sea opaco: el panel tiene que pintarlo. Un fondo
// saturado representa una foto o un documento situado detrás de las opciones.
func TestLosPanelesTraslucidosTapanElContenidoDeDebajo(t *testing.T) {
	base := os.Getenv("SYTUE_CAPTURA_CROMO")
	tema.Elegir("traslucido")
	fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	t.Cleanup(func() {
		tema.Elegir("")
		tema.ElegirColor("")
		tema.ElegirForma("")
		fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	})
	for _, forma := range []string{"marco", "carril"} {
		tema.ElegirForma(forma)
		for _, paleta := range []string{"petroleo", "cobalto"} {
			tema.ElegirColor(paleta)
			for _, caso := range []struct {
				nombre string
				crear  func() fyne.CanvasObject
			}{
				{"menú", func() fyne.CanvasObject {
					return NuevoMenu(NuevoElementoMenu("Abrir", iconos.Ninguno, nil))
				}},
				{"popover", func() fyne.CanvasObject {
					e := &Emergente{contenido: canvas.NewRectangle(color.NRGBA{}), relleno: RellenoPopover}
					e.ExtendBaseWidget(e)
					return e
				}},
				{"selector", func() fyne.CanvasObject {
					return NuevoSelector([]string{"Una opción", "Otra opción"}, nil).lista()
				}},
			} {
				panel := caso.crear()
				panel.Resize(fyne.NewSize(320, 100))
				for _, debajo := range []color.NRGBA{{R: 255, A: 255}, {B: 255, A: 255}} {
					img := pintaSobre(t, debajo, 320, 100, panel)
					got := px(img, int(280*escala), int(90*escala))
					if want := tema.Color("menu-surface", tema.Translucido); !cerca(got, want) {
						t.Errorf("%s/%s/%s sobre %v pinta %v; el papel opaco es %v", forma, paleta, caso.nombre, debajo, got, want)
					}
					// El CI deja una muestra representativa junto a los retratos del
					// cromo, con el mismo prefijo de TestRetratoDeLaBarraConElLateral.
					if base != "" && forma == "marco" && paleta == "petroleo" && caso.nombre == "selector" && debajo.R == 255 {
						nombre := base + "-menu-traslucido-marco.png"
						f, err := os.Create(nombre)
						if err != nil {
							t.Fatal(err)
						}
						if err := png.Encode(f, img); err != nil {
							_ = f.Close()
							t.Fatal(err)
						}
						if err := f.Close(); err != nil {
							t.Fatal(err)
						}
						t.Logf("pintor=software menú=selector modo=traslucido forma=marco escala=%g captura=%s", escala, nombre)
					}
				}
			}
		}
	}
}

// Una lista puede seguir abierta cuando otra app cambia las preferencias. Antes
// solo refrescaba el resaltado: la caja y la tinta conservaban el modo anterior.
func TestElSelectorAbiertoActualizaElPapelYLaTintaAlCambiarTema(t *testing.T) {
	t.Cleanup(func() {
		tema.Elegir("")
		fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	})
	tema.Elegir("oscuro")
	fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	panel := NuevoSelector([]string{"Una opción", "Otra opción"}, nil).lista()
	panel.Resize(fyne.NewSize(320, 100))
	// El mismo lienzo permanece montado en toda la prueba. Reconstruirlo entre la
	// mutación y la captura podría reparar el defecto que el control debe detectar.
	lienzo := software.NewCanvas()
	lienzo.SetPadded(false)
	lienzo.SetContent(container.NewWithoutLayout(panel))
	lienzo.SetScale(escala)
	lienzo.Resize(fyne.NewSize(320, 100))
	_ = lienzo.Capture()
	papelViejo := tema.Color("menu-surface", tema.Oscuro)

	for _, nombre := range []string{"traslucido", "claro", "oscuro"} {
		tema.Elegir(nombre)
		fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		if nombre == "traslucido" {
			// Control negativo acotado, también en el runner: reproducir la caja
			// oscura conservada tiene que hacer fallar la misma medida de píxel.
			panel.fondo.Pintar(papelViejo, tema.Color("border", v))
			roto := lienzo.Capture()
			if got := px(roto, int(280*escala), int(90*escala)); cerca(got, tema.Color("menu-surface", v)) {
				t.Fatal("el guardián no detecta que el selector conserva el papel oscuro")
			}
		}
		panel.Refresh()
		img := lienzo.Capture()
		if got, want := px(img, int(280*escala), int(90*escala)), tema.Color("menu-surface", v); !cerca(got, want) {
			t.Errorf("al pasar a %s el selector conserva el papel %v; toca %v", nombre, got, want)
		}
		for i, h := range panel.hijos {
			r := test.WidgetRenderer(h.(fyne.Widget)).(*opcionRenderer)
			if got, want := r.texto.Color, tema.Color("text", v); got != color.Color(want) {
				t.Errorf("al pasar a %s la opción conserva tinta %v; toca %v", nombre, got, want)
			}
			fondo := px(img, int(280*escala), int(90*escala))
			if i == panel.s.Elegida {
				fondo = px(img, int(280*escala), int(20*escala))
			}
			if razon := contraste(tema.Color("text", v), fondo); razon < 4.5 {
				t.Errorf("al pasar a %s la opción %d solo contrasta %.2f:1", nombre, i, razon)
			}
		}
	}
}
