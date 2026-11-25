// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// elegirTemaEnCaliente cambia de tema con la ventana abierta, como el menú de cuenta
// (`sesion.repintar`): la variante de la suite y el SetTheme que hace a Fyne recorrer y
// refrescar lo que hay.
func elegirTemaEnCaliente(nombre string) {
	tema.Elegir(nombre)
	fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
}

// TestLaCintaYElPanelSiguenAlTemaEnCaliente: con un PDF abierto, cambiar de tema
// repinta la cinta —la fila de pestañas y la banda de grupos—, el panel de páginas con
// su tira y el fondo del visor con el tema NUEVO, y la ventana queda igual, píxel a
// píxel, que una que nace en él.
//
// Abierto directamente en oscuro salía bien; al cambiar en caliente esas piezas se
// quedaban en el tema de antes, porque eran rectángulos (y el degradado de la máscara)
// con el color resuelto al componer, que el refresco de Fyne repinta tal cual. VISTO EN
// ROJO antes de `ui.Liso` y de `fundidoMascara`, en las dos direcciones.
func TestLaCintaYElPanelSiguenAlTemaEnCaliente(t *testing.T) {
	t.Cleanup(func() { elegirTemaEnCaliente("") })
	for _, c := range []struct{ desde, hasta string }{{"claro", "oscuro"}, {"oscuro", "claro"}} {
		t.Run(c.desde+"→"+c.hasta, func(t *testing.T) {
			elegirTemaEnCaliente(c.desde)
			p, w := documentoDeRecorrido(t)
			elegirTemaEnCaliente(c.hasta)
			cambiado := w.Canvas().Capture()
			_, w2 := documentoDeRecorrido(t)
			directo := w2.Canvas().Capture()

			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			tam := w.Canvas().Size()
			fila, cinta := AltoFilaBarra(), p.barra.Alto()
			// Un punto de cada pieza donde no hay más que su fondo: a la derecha de las
			// pestañas, al final de la banda, al pie del panel y en la esquina del visor.
			for _, m := range []struct {
				pieza string
				x, y  float32
				token string
			}{
				{"la fila de pestañas", tam.Width - 4, fila / 2, "surface"},
				{"la banda de la cinta", tam.Width - 4, fila + 4, "bg-subtle"},
				{"el filete de la cinta", tam.Width - 4, cinta - tema.Borde/2, "border"},
				{"el panel de páginas", AnchoPanel - 8, tam.Height - 8, "menu-surface"},
				{"el filete del panel", AnchoPanel - tema.Borde/2, tam.Height - 8, "border"},
				{"el fondo del visor", AnchoPanel + 4, cinta + 4, "bg-subtle"},
			} {
				got := color.NRGBAModel.Convert(cambiado.At(int(m.x), int(m.y))).(color.NRGBA) //nolint:forcetypeassert // el modelo devuelve eso
				if quiero := tema.Color(m.token, v); got != quiero {
					t.Errorf("%s pinta %v tras pasar a %s; su `%s` es %v", m.pieza, got, c.hasta, m.token, quiero)
				}
			}
			for _, r := range []struct {
				pieza string
				caja  image.Rectangle
			}{
				{"la cinta", image.Rect(0, 0, int(tam.Width), int(cinta))},
				{"el panel de páginas", image.Rect(0, int(cinta), int(AnchoPanel), int(tam.Height))},
				{"el visor", image.Rect(int(AnchoPanel), int(cinta), int(tam.Width), int(tam.Height))},
			} {
				if n, donde := pixelesDistintos(cambiado, directo, r.caja); n > 0 {
					t.Errorf("%s: %d píxeles distintos de los de una ventana que nace en %s (el primero en %v)",
						r.pieza, n, c.hasta, donde)
				}
			}
		})
	}
}

// pixelesDistintos cuenta los píxeles de `caja` que no son iguales en las dos imágenes,
// y dice dónde está el primero.
func pixelesDistintos(a, b image.Image, caja image.Rectangle) (n int, primero image.Point) {
	caja = caja.Intersect(a.Bounds()).Intersect(b.Bounds())
	for y := caja.Min.Y; y < caja.Max.Y; y++ {
		for x := caja.Min.X; x < caja.Max.X; x++ {
			if color.NRGBAModel.Convert(a.At(x, y)) != color.NRGBAModel.Convert(b.At(x, y)) {
				if n == 0 {
					primero = image.Pt(x, y)
				}
				n++
			}
		}
	}
	return n, primero
}
