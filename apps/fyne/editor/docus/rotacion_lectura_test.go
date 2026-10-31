// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func montarLecturaDocus(t *testing.T) (*Folio, fyne.Window) {
	t.Helper()
	// Secciones del documento REAL de la demo, leídas de su única fuente. Repetir
	// sus secciones permite probar varias páginas, no sólo un párrafo de relleno.
	f := folioCon(t, strings.Repeat(muestraReal(t), 4))
	f.ctx.Estado = editor.Listo
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	f.ctx.T = tr.T
	m := editor.NuevoMarco(f.ctx, registroDocus("es"), nil)
	barra, parar := ui.BarraSuiteDePrueba(tr, iconos.Docs)
	t.Cleanup(parar)
	w := test.NewTempWindow(t, barra.Montar(m))
	w.SetPadded(false)
	return f, w
}

func TestLaRotacionDeDocusConPocaAlturaConservaLecturaYDocumento(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	for _, forma := range []string{"marco", "carril"} {
		t.Run(forma, func(t *testing.T) {
			tema.ElegirForma(forma)
			t.Cleanup(tema.FormaDeLosGuardianes)
			f, w := montarLecturaDocus(t)
			pagina, original := f.Pagina(), f.Serializar()
			for _, tam := range []fyne.Size{{Width: 568, Height: 320}, {Width: 568, Height: 272}} {
				w.Resize(tam)
				if f.ctx.Zoom < 75 {
					t.Errorf("%s %v: zoom automático %d reduce el cuerpo16 por debajo de12dp", forma, tam, f.ctx.Zoom)
				}
				f.scroll.ScrollToTop()
				d := fyne.CurrentApp().Driver()
				origen := d.AbsolutePositionForObject(f.scroll)
				r := test.WidgetRenderer(f.pag).(*paginaRenderer)
				var cuerpo *canvas.Text
				for _, o := range r.dyn {
					if texto, ok := o.(*canvas.Text); ok && strings.Contains(texto.Text, "Migración") {
						cuerpo = texto
						break
					}
				}
				if cuerpo == nil {
					t.Fatal("el pintor no enseña el párrafo real de migración")
				}
				p := d.AbsolutePositionForObject(cuerpo)
				test.Drag(w.Canvas(), origen.AddXY(30, 30), 0, origen.Y+20-p.Y)
				if f.scroll.Offset.Y <= 0 {
					t.Fatal("el dedo no puede pasar de los márgenes al contenido del documento")
				}
				p = d.AbsolutePositionForObject(cuerpo)
				if cuerpo.TextSize < 12 || p.Y < origen.Y || p.Y+cuerpo.MinSize().Height > origen.Y+f.scroll.Size().Height {
					t.Errorf("%s %v: el gesto no deja cuerpo real a12dp dentro del viewport", forma, tam)
				}
				if f.Pagina() != pagina || f.Serializar() != original {
					t.Fatal("encuadrar/girar alteró A4, márgenes o contenido del documento")
				}
				capturarLecturaDocus(t, w, fmt.Sprintf("%s-%gx%g", forma, tam.Width, tam.Height))
			}
		})
	}
}

func TestLaRotacionDeDocusConservaZoomExplicitoYCursorFueraDelInicio(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	for _, zoom := range []int{75, 50, 125} {
		t.Run(fmt.Sprint(zoom), func(t *testing.T) {
			f, w := montarLecturaDocus(t)
			w.Resize(fyne.NewSize(568, 320))
			f.ctx.PonerZoom(zoom)
			pagina := f.Pagina()
			f.Enfocar()
			f.AlFinal()
			cursor := f.cur
			for _, tam := range []fyne.Size{{Width: 1024, Height: 768}, {Width: 568, Height: 320}, {Width: 568, Height: 272}} {
				w.Resize(tam)
				if f.ctx.Zoom != zoom || f.Pagina() != pagina || f.cur != cursor {
					t.Fatalf("girar cambia zoom explícito%d, página o cursor: zoom%d %v %v", zoom, f.ctx.Zoom, f.Pagina(), f.cur)
				}
				x, y, alto, ok := f.caretEnPagina()
				off, vista := f.scroll.Offset, f.scroll.Size()
				if !ok || x < off.X || x > off.X+vista.Width || y < off.Y || y+alto > off.Y+vista.Height {
					t.Errorf("%v a%d%%: cursor fuera de la vista tras girar: (%g,%g,%g), offset%v, viewport%v", tam, zoom, x, y, alto, off, vista)
				}
			}
			test.Type(f.pag, " Verificado tras girar")
			if !strings.Contains(f.Serializar(), "Verificado tras girar") || f.Pagina() != pagina {
				t.Fatal("la inserción al final perdió el texto o alteró la página")
			}
			capturarLecturaDocus(t, w, fmt.Sprintf("cursor-%d", zoom))
		})
	}
}

func capturarLecturaDocus(t *testing.T, w fyne.Window, nombre string) {
	t.Helper()
	dir := os.Getenv("SYTUE_ROTACION_CAPTURAS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "native-rotacion-docus-lectura-"+nombre+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}

func TestLaRotacionDeDocusConservaLaLecturaLejosDelCursor(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	f, w := montarLecturaDocus(t)
	w.Resize(fyne.NewSize(568, 320))
	f.ctx.PonerZoom(75)
	f.Enfocar()
	f.mover(punto{}, false)
	cursor, pagina, original := f.cur, f.Pagina(), f.Serializar()
	d := fyne.CurrentApp().Driver()
	origen := d.AbsolutePositionForObject(f.scroll)
	test.Drag(w.Canvas(), origen.AddXY(30, 30), 0, -600)
	_, y, alto, ok := f.caretEnPagina()
	antes := f.scroll.Offset
	if !f.enfocado || !ok || y+alto >= antes.Y || antes.Y < 500 {
		t.Fatal("el fixture no se ha desplazado lejos del cursor manteniendo el foco")
	}
	w.Resize(fyne.NewSize(568, 272))
	f.Refresh()
	if f.scroll.Offset != antes || f.cur != cursor || f.Pagina() != pagina || f.Serializar() != original {
		t.Fatalf("girar/repintar trae la lectura al cursor oculto o modifica el documento: offset%v→%v", antes, f.scroll.Offset)
	}
	capturarLecturaDocus(t, w, "lectura-lejos-del-cursor")
}
