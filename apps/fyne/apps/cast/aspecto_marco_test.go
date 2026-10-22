// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El traslúcido hace visible el fallo: un `bg` en Pagina tapaba el papel que
// Armazon había compuesto sobre el marco. Se compara con la misma hoja vacía.
func TestElContenidoDeCastDejaVerElPapelDeMarco(t *testing.T) {
	tema.ElegirForma("marco")
	tema.Elegir("traslucido")
	fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	t.Cleanup(func() {
		tema.ElegirForma("")
		tema.Elegir("")
		fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	})
	w := test.NewWindow(ui.NuevoArmazon(canvas.NewRectangle(color.NRGBA{})))
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	vacio := w.Canvas().Capture()
	x, y := vacio.Bounds().Dx()/2, vacio.Bounds().Dy()/2
	papel := colorEn(vacio, x, y)
	if parecido(papel, tema.Color("bg", tema.Translucido), 2) {
		t.Fatal("la muestra no distingue el papel del fondo opaco de la página")
	}
	w.SetContent(ui.NuevoArmazon(Pagina(vistaDePrueba(t, tema.Translucido))))
	img := w.Canvas().Capture()
	guardar(t, "cast-papel-marco-traslucido", img)
	if got := colorEn(img, x, y); !parecido(got, papel, 1) {
		t.Errorf("Cast tapa el papel de Marco: pinta %v y la hoja pinta %v", got, papel)
	}
}

// El nombre del canal y la descripción pública pueden ocupar varias líneas. En
// Marco la letra crece; los controles deben conservar su sitio también a 360 px.
func TestLasCabecerasPublicasDeMarcoCabenEnEstrecho(t *testing.T) {
	tema.ElegirForma("marco")
	t.Cleanup(func() { tema.ElegirForma("") })
	for _, ancho := range []float32{360, 820} {
		v := vistaDePrueba(t, tema.Claro)
		v.Ventana = ancho
		buscar := ui.NuevaEntrada(v.Traduce("explore.search")).Con(ui.CampoSM)
		pantallas := []fyne.CanvasObject{
			PantallaExplorar(v, Explorar{Buscador: buscar, Videos: misVideos()[:1]}),
			PantallaCanal(v, Canal{Org: "acme", Nombre: strings.Repeat("Una organización con nombre largo ", 3), AlDenunciar: func() {}}),
		}
		for i, p := range pantallas {
			w := test.NewWindow(enMarco(v, 255, p))
			w.SetPadded(false)
			w.Resize(fyne.NewSize(ancho, 800))
			guardar(t, fmt.Sprintf("cast-publico-marco-%.0f-%d", ancho, i), w.Canvas().Capture())
			// El armazón pinta una sombra fuera de la hoja. Se inspecciona la página
			// ya colocada para medir su contenido, sin contar esa decoración.
			if fuera := desbordado(p, ancho); fuera != "" {
				t.Errorf("la cabecera se sale a %v px: %s", ancho, fuera)
			}
			w.Close()
		}
	}
}
