// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestElZoomMovilRedibujaElFolioSinRedimensionar(t *testing.T) {
	tema.Elegir("claro")
	t.Cleanup(func() { tema.Elegir("") })
	for _, ancho := range []float32{320, 375, 390} {
		f := folioCon(t, "<h1>Propuesta de reforma</h1><p>Documento de prueba del zoom.</p>")
		f.ctx.Estado = editor.Listo
		m := editor.NuevoMarco(f.ctx, registroDocus("es"), nil)
		w := test.NewTempWindow(t, m)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(ancho, 740))
		f.ctx.PonerZoom(100)
		_ = w.Canvas().Capture()
		caja := f.Size()
		f.ctx.PonerZoom(25)
		r := test.WidgetRenderer(f.pag).(*paginaRenderer)
		if got, want := r.papel.Size().Width, AnchoPagina/4; math.Abs(float64(got-want)) > .02 {
			t.Errorf("a%g el zoom dice25%% pero el papel pinta%gpx; debe pintar%g", ancho, got, want)
		}
		if f.Size() != caja {
			t.Errorf("a%g cambiar el zoom ha redimensionado la caja del folio", ancho)
		}
		img := w.Canvas().Capture()
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(f)
		x, y := int(pos.X+caja.Width-5), int(pos.Y+70)
		got := img.At(x, y)
		gr, gg, gb, _ := got.RGBA()
		fondo := f.lienzo()
		if gr>>8 != uint32(fondo.R) || gg>>8 != uint32(fondo.G) || gb>>8 != uint32(fondo.B) {
			t.Errorf("a%g el papel sigue invadiendo el margen derecho: pixel%v frente%v", ancho, got, fondo)
		}
		if dir := os.Getenv("SYTUE_CAPTURAS"); dir != "" {
			fh, err := os.Create(filepath.Join(dir, fmt.Sprintf("docus-zoom-movil-%g.png", ancho)))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(fh, img)
			_ = fh.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
