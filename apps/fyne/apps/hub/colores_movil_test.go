// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"fmt"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Mide el montaje real antes de capturarlo. La escala ×3 expresa el tamaño que
// pediría el Raster a Retina; no presupone el límite de la GPU del teléfono.
func TestElHubClaroMideElFondoRetina(t *testing.T) {
	t.Cleanup(func() {
		tema.Elegir("")
		tema.ElegirApp("")
		tema.FormaDeLosGuardianes()
	})
	tema.Elegir("claro")
	for _, forma := range []string{"marco", "carril"} {
		tema.ElegirForma(forma)
		for _, ancho := range []float32{320, 375, 390} {
			l := lanzadorDePrueba(t)
			desplaza := container.NewVScroll(l)
			barra := ui.NuevaBarraSuite(l.T, nil, iconos.One).ConSinSesionPropia()
			raiz := barra.Montar(desplaza)
			w := test.NewTempWindow(t, raiz)
			w.SetPadded(false)
			w.Resize(fyne.NewSize(ancho, 740))
			raiz.Refresh()
			r := test.WidgetRenderer(l).(*lanzadorRenderer)
			fondo := r.fondo.Size()
			if fondo.Height <= desplaza.Size().Height || fondo.Width != l.Size().Width {
				t.Fatalf("%s a %g no mide el fondo de una página desplazable: fondo=%v, ventana=%v", forma, ancho, fondo, desplaza.Size())
			}
			const retina = float32(3)
			t.Logf("%s ancho=%g: viewport=%v contenido=%v fondo=%v; textura de página ×3=%gx%g (%g MiB RGBA)",
				forma, ancho, desplaza.Size(), l.Size(), fondo, fondo.Width*retina, fondo.Height*retina,
				fondo.Width*fondo.Height*retina*retina*4/(1024*1024))
		}
	}
}

// El montaje es el de cmd/hub, con desplazador vertical y cromo de la suite. Las
// capturas anteriores pintaban la página suelta y no enseñaban el papel de Marco
// que queda debajo de sus superficies traslúcidas.
func TestElHubMovilConservaLaPaletaDeLaWeb(t *testing.T) {
	if os.Getenv("SYTUE_CAPTURAS") == "" {
		t.Skip("diagnóstico óptico: SYTUE_CAPTURAS guarda la matriz de apariencias")
	}
	web := webref.Read(t, "apps/frontend/apps/portal/src/styles/global.css")
	if !contiene(web, ".hub-product-card {\n  background: var(--sytue-surface);") {
		t.Fatal("la web ya no usa surface en las teselas: revisar el contrato de color")
	}
	t.Cleanup(func() {
		tema.Elegir("")
		tema.ElegirApp("")
		tema.FormaDeLosGuardianes()
	})
	for _, forma := range []string{"marco", "carril"} {
		tema.ElegirForma(forma)
		for _, ancho := range []float32{320, 375, 390} {
			l := lanzadorDePrueba(t)
			l.Nombre = "Ada"
			desplaza := container.NewVScroll(l)
			barra := ui.NuevaBarraSuite(l.T, nil, iconos.One).ConSinSesionPropia()
			raiz := barra.Montar(desplaza)
			w := test.NewWindow(raiz)
			w.SetPadded(false)
			w.Resize(fyne.NewSize(ancho, 740))
			for _, modo := range []string{"claro", "oscuro", "traslucido", "claro"} {
				tema.Elegir(modo)
				barra.Refresh()
				l.Refresh()
				raiz.Refresh()
				r := test.WidgetRenderer(l).(*lanzadorRenderer)
				v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
				if r.v != v || r.fondo.Variante != v {
					t.Errorf("%s/%s a %g conserva la apariencia anterior", forma, modo, ancho)
				}
				for _, tarjeta := range []*tarjeta{r.hoy, r.recientes} {
					if tarjeta.fondo() != "surface" {
						t.Errorf("%s/%s: la sección usa %s en vez de surface", forma, modo, tarjeta.fondo())
					}
				}
				for _, tile := range r.teselas {
					tr := test.WidgetRenderer(tile).(*teselaRenderer)
					if tile.fondo(v) != tema.Color("surface", v) || tr.nombre.Color != tema.Color("text", v) {
						t.Errorf("%s/%s: %s no sigue surface/text", forma, modo, tile.Producto.NombreCorto())
					}
					if len(tr.desc) > 0 && tr.desc[0].Color != tema.Color("text-muted", v) {
						t.Errorf("%s/%s: la descripción no sigue text-muted", forma, modo)
					}
				}
				desplaza.ScrollToTop()
				guardar(t, w.Canvas().Capture(), fmt.Sprintf("hub-paleta-%s-%s-%g-inicio", forma, modo, ancho))
				desplaza.ScrollToBottom()
				guardar(t, w.Canvas().Capture(), fmt.Sprintf("hub-paleta-%s-%s-%g-tarjetas", forma, modo, ancho))
				t.Logf("%s/%s ancho=%g: papel=%v superficie=%v texto=%v", forma, modo, ancho,
					tema.Color("bg", v), tema.Color("surface", v), tema.Color("text", v))
			}
			w.Close()
		}
	}
}
