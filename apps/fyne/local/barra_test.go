// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// TestLaBarraDeLaEdicionUsaLosBotonesDeUI (§8.3, «los de la barra local»): la barra de
// las apps de Sytue Office (`Montar`) lleva los botones de `ui` y no los de Fyne, y se
// pintan como en la web: el fantasma atenuado (`text-text-muted hover:bg-bg-subtle
// hover:text-text`) de alto `sm` (h-8, 32 px), sin relleno en reposo. Y el Hub, que tiene
// su propia barra, tampoco crea botones de Fyne en ella.
//
// Y el menú «Nube» que abre su botón: el `Menu` de `ui` (Dropdown.tsx) colgado del botón,
// alineado con su borde derecho, con sus `ElementoMenu` (`text` en el rótulo,
// `text-muted` en el glifo, `menu-surface` en reposo y `menu-hover` con el puntero), y no
// un diálogo con botones de Fyne; «Guardar en la nube…» solo si la app exporta.
//
// VISTO EN ROJO (2026-11-25): con la barra de antes (widget.NewButtonWithIcon de
// importancia baja) dijo que había dos botones de Fyne y ninguno de `ui`; con el
// fantasma sin Atenuado, que el rótulo no estaba en `text-muted` ni el relleno con el
// puntero en `bg-subtle`. Con el menú de antes (un `ui.Dialogo` con tres
// widget.NewButtonWithIcon), que «Nube» no abría el menú de `ui` y que dentro había
// botones de Fyne.
func TestLaBarraDeLaEdicionUsaLosBotonesDeUI(t *testing.T) {
	t.Setenv("SYTUE_IDIOMA", "es")
	a := test.NewTempApp(t)
	a.Settings().SetTheme(tema.Sytue{})
	v := tema.De(a.Settings().ThemeVariant())
	l := &App{A: a, Espacio: &espacio.Espacio{}}
	w := a.NewWindow("barra")
	w.SetContent(l.Montar(w, iconos.Notes, layout.NewSpacer()))
	w.Resize(fyne.NewSize(900, 400))

	var botones []*ui.Boton
	for _, o := range test.LaidOutObjects(w.Content()) {
		switch b := o.(type) {
		case *widget.Button:
			t.Errorf("la barra lleva un botón de Fyne: %q", b.Text)
		case *ui.Boton:
			botones = append(botones, b)
		}
	}
	if len(botones) != 2 || botones[0].Texto != "Nube" || botones[1].Texto != "Ajustes" {
		t.Fatalf("la barra lleva %d botones de ui; quiero Nube y Ajustes", len(botones))
	}
	for _, b := range botones {
		if b.Variante != ui.Fantasma || b.Talla != ui.TallaSM || !b.Atenuado || b.Icono == 0 {
			t.Errorf("%s: variante %v, talla %v, atenuado %v, glifo %v; quiero el fantasma atenuado sm con glifo", b.Texto, b.Variante, b.Talla, b.Atenuado, b.Icono)
		}
		pos, tam := a.Driver().AbsolutePositionForObject(b), b.Size()
		if quiero := ui.MedidaBotonDe(b); tam.Height != quiero.Height || tam.Height != 32 {
			t.Errorf("%s: mide %v de alto; `h-8` son 32 y ui dice %v", b.Texto, tam.Height, quiero.Height)
		}
		// Dentro del botón, pasado el radio de la esquina y antes del glifo (`px-3`).
		x, y := int(pos.X+7), int(pos.Y+tam.Height/2)
		img := w.Canvas().Capture()
		fondo := px(img, int(pos.X-2), y)
		if got := px(img, x, y); !casi(got, fondo, 2) {
			t.Errorf("%s: en reposo pinta %v donde la barra es %v: el fantasma no tiene relleno", b.Texto, got, fondo)
		}
		if !hayTinta(img, pos, tam, tema.Color("text-muted", v)) {
			t.Errorf("%s: el rótulo no está en `text-muted` (%v)", b.Texto, tema.Color("text-muted", v))
		}
		b.MouseIn(nil)
		img = w.Canvas().Capture()
		if got, quiero := px(img, x, y), tema.Color("bg-subtle", v); !casi(got, quiero, 2) {
			t.Errorf("%s: con el puntero pinta %v; la web, `bg-subtle` %v", b.Texto, got, quiero)
		}
		if alto := altoDe(img, x, y, tema.Color("bg-subtle", v)); alto != 32 {
			t.Errorf("%s: el relleno con el puntero mide %d de alto; quiero 32", b.Texto, alto)
		}
		if !hayTinta(img, pos, tam, tema.Color("text", v)) {
			t.Errorf("%s: con el puntero el rótulo no pasa a `text`", b.Texto)
		}
		b.MouseOut()
	}

	// El menú «Nube», sin nada que exportar y con ello.
	comprobarMenuNube(t, w, botones[0], v, "Abrir de la nube…", "Ajustes…")
	l.AlExportar(func() (string, []byte, error) { return "nota.md", nil, nil })
	comprobarMenuNube(t, w, botones[0], v, "Abrir de la nube…", "Guardar en la nube…", "Ajustes…")

	// El Hub monta su propia barra (`cmd-local/hub`, func barra): con BotonDeBarra.
	ruta := filepath.Join("..", "cmd-local", "hub", "main.go")
	arbol, err := parser.ParseFile(token.NewFileSet(), ruta, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range arbol.Decls {
		f, ok := d.(*ast.FuncDecl)
		if !ok || f.Name.Name != "barra" {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && strings.HasPrefix(sel.Sel.Name, "NewButton") {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "widget" {
					t.Errorf("la barra del Hub crea un botón de Fyne (widget.%s); usa local.BotonDeBarra", sel.Sel.Name)
				}
			}
			return true
		})
	}
}

// comprobarMenuNube pulsa Nube y mira el menú que cuelga del botón: el de `ui`, con
// esos elementos y pintado como el de la web; y lo cierra.
func comprobarMenuNube(t *testing.T, w fyne.Window, nube *ui.Boton, v tema.Variante, quiero ...string) {
	t.Helper()
	test.Tap(nube)
	capas := w.Canvas().Overlays().List()
	var menu *ui.Menu
	var elementos []*ui.ElementoMenu
	var textos []string
	for _, c := range capas {
		for _, o := range test.LaidOutObjects(c) {
			switch e := o.(type) {
			case *ui.Menu:
				menu = e
			case *ui.ElementoMenu:
				elementos, textos = append(elementos, e), append(textos, e.Texto)
			case *widget.Button:
				t.Errorf("el menú «Nube» lleva un botón de Fyne: %q", e.Text)
			case *ui.Dialogo:
				t.Error("«Nube» abre un diálogo; es el menú de ui colgado del botón")
			}
		}
	}
	if menu == nil {
		t.Fatalf("al pulsar «Nube» no se abre el menú de ui (capas: %d)", len(capas))
	}
	if !slices.Equal(textos, quiero) {
		t.Errorf("el menú «Nube» lleva %q; quiero %q", textos, quiero)
	}
	abs := fyne.CurrentApp().Driver().AbsolutePositionForObject
	pm, pn := abs(menu), abs(nube)
	// `align='end'`, debajo del botón con el hueco de Dropdown.tsx.
	if d := (pm.X + menu.Size().Width) - (pn.X + nube.Size().Width); d < -1 || d > 1 || pm.Y < pn.Y+nube.Size().Height {
		t.Errorf("el menú está en %v (%v) y el botón en %v (%v): cuelga de él, alineado con su borde derecho", pm, menu.Size(), pn, nube.Size())
	}
	img := w.Canvas().Capture()
	if !hayTinta(img, pm, menu.Size(), tema.Color("text", v)) || !hayTinta(img, pm, menu.Size(), tema.Color("text-muted", v)) {
		t.Errorf("el menú no pinta sus rótulos en `text` y sus glifos en `text-muted`")
	}
	for _, e := range elementos {
		if e.Icono == 0 || e.AlPulsar == nil {
			t.Errorf("%s: glifo %v; quiero su glifo del catálogo y su acción", e.Texto, e.Icono)
		}
		// En el `px-3` del elemento, antes del glifo.
		pe := abs(e)
		x, y := int(pe.X+5), int(pe.Y+e.Size().Height/2)
		if got, quiero := px(img, x, y), tema.Color("menu-surface", v); !casi(got, quiero, 2) {
			t.Errorf("%s: en reposo pinta %v; la web, `menu-surface` %v", e.Texto, got, quiero)
		}
		e.MouseIn(nil)
		if got, quiero := px(w.Canvas().Capture(), x, y), tema.Color("menu-hover", v); !casi(got, quiero, 2) {
			t.Errorf("%s: con el puntero pinta %v; la web, `menu-hover` %v", e.Texto, got, quiero)
		}
		e.MouseOut()
	}
	menu.Cerrar()
	if n := len(w.Canvas().Overlays().List()); n != 0 {
		t.Errorf("cerrado el menú quedan %d capas", n)
	}
}

func px(img image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

func casi(a, b color.NRGBA, tol int) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

// hayTinta dice si algún píxel del botón es del color c (el rótulo y el glifo, con su
// suavizado: basta uno pleno).
func hayTinta(img image.Image, pos fyne.Position, tam fyne.Size, c color.NRGBA) bool {
	for y := int(pos.Y); y < int(pos.Y+tam.Height); y++ {
		for x := int(pos.X); x < int(pos.X+tam.Width); x++ {
			if casi(px(img, x, y), c, 6) {
				return true
			}
		}
	}
	return false
}

// altoDe es cuántas filas seguidas, por encima y por debajo de (x, y), son del color c.
func altoDe(img image.Image, x, y int, c color.NRGBA) int {
	arriba, abajo := y, y
	for arriba > img.Bounds().Min.Y && casi(px(img, x, arriba-1), c, 2) {
		arriba--
	}
	for abajo < img.Bounds().Max.Y-1 && casi(px(img, x, abajo+1), c, 2) {
		abajo++
	}
	return abajo - arriba + 1
}
