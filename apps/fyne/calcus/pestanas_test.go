// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLaBarraDeHojasSaleDeSheetCanvas vigila las clases de las que salen sus medidas.
// Si la web cambia `h-8` o el `px-2` de la pestaña, esto lo dice y la cuenta se rehace.
func TestLaBarraDeHojasSaleDeSheetCanvas(t *testing.T) {
	fuente := webref.Read(t, lienzoWeb)
	for _, c := range []struct{ trozo, porQue string }{
		{"border-border bg-surface flex h-8 shrink-0 items-center gap-1 border-t px-2 text-xs", "la barra: `h-8`, `gap-1`, `px-2`, `border-t` y `text-xs`"},
		{"flex h-full min-w-0 flex-1 items-center gap-1 overflow-x-auto", "la tira: se desplaza y se lleva lo que sobra"},
		{"relative h-full max-w-[12rem] shrink-0 truncate px-2", "la pestaña: `px-2` y el tope de 12rem"},
		{"'text-text font-medium' : 'text-text-muted hover:text-text'", "la activa en `font-medium`, las demás apagadas"},
		{"bg-indicator absolute inset-x-1 bottom-0 h-0.5 rounded-full", "el subrayado de la activa"},
		{"text-text-muted shrink-0 truncate tabular-nums", "el resumen del rango"},
		{`icon="plus"`, "el botón de añadir hoja"},
		{`icon="copy"`, "el de duplicar"},
		{`icon="trash"`, "el de eliminar"},
	} {
		if !strings.Contains(fuente, c.trozo) {
			t.Errorf("SheetCanvas.tsx ya no contiene %q (%s)", c.trozo, c.porQue)
		}
	}
	for _, c := range []struct {
		clase           string
		nuestra, quiero float32
	}{
		{"h-8", AltoPestanas, 32},
		{"px-2 de la barra", MargenPestanas, 8},
		{"gap-1", HuecoPestanas, 4},
		{"px-2 de la pestaña", MargenPestana, 8},
		{"max-w-[12rem]", AnchoMaxPestana, 192},
		{"inset-x-1", SangriaIndicador, 4},
		{"h-0.5", GrosorIndicador, 2},
	} {
		if c.nuestra != c.quiero {
			t.Errorf("`%s` con el rem de 16 son %v y aquí %v", c.clase, c.quiero, c.nuestra)
		}
	}
}

// lienzoTraducido es un lienzo con el catálogo de Docs en castellano: la barra de hojas
// pinta frases del catálogo y un guardián de píxeles con las claves mediría otra cosa.
func lienzoTraducido(t *testing.T) *Lienzo {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	c := NuevoLienzo(NuevoLibroCon([]string{"Hoja 1", "Ventas"}, 0))
	c.T = tr.T
	c.Resize(fyne.NewSize(560, 300))
	return c
}

// TestLaBarraDeHojasPintaDondeLaWeb es el guardián de píxeles: la línea de arriba, el
// subrayado de la activa (y solo de la activa) y los tres botones pegados a la derecha.
func TestLaBarraDeHojasPintaDondeLaWeb(t *testing.T) {
	c := lienzoTraducido(t)
	p := c.Pestanas
	p.Move(fyne.NewPos(0, 0)) // suelta, no en el pie del lienzo
	p.Resize(fyne.NewSize(560, AltoPestanas))
	p.Refresh()
	img := pinta(t, 560, AltoPestanas, p)
	guarda(t, img)
	v := tema.Claro

	if got := px(img, 200, 0); !cerca(got, tema.Color("border", v)) {
		t.Errorf("el `border-t` pinta %v y el token es %v", got, tema.Color("border", v))
	}
	activa := MedidaPestanaHoja("Hoja 1", true)
	// El subrayado ocupa los 2 px de abajo de la pestaña, con 4 px de sangría.
	yInd := AltoPestanas - GrosorIndicador/2
	if got := px(img, MargenPestanas+activa.Width/2, yInd); !cerca(got, tema.Color("indicator", v)) {
		t.Errorf("el subrayado de la activa pinta %v y `bg-indicator` es %v", got, tema.Color("indicator", v))
	}
	if got := px(img, MargenPestanas+SangriaIndicador/2, yInd); !cerca(got, tema.Color("surface", v)) {
		t.Errorf("la sangría del subrayado pinta %v: `inset-x-1` no se respeta", got)
	}
	otra := MargenPestanas + activa.Width + HuecoPestanas + MedidaPestanaHoja("Ventas", false).Width/2
	if got := px(img, otra, yInd); !cerca(got, tema.Color("surface", v)) {
		t.Errorf("la pestaña que no está activa lleva subrayado: %v", got)
	}

	// Los botones: el de eliminar, el último, acaba a `px-2` del borde; cada uno mide
	// 28 px (`h-7 w-7`) y los separa `gap-1` (4 px).
	borrar := 560 - MargenPestanas - editor.LadoBotonCinta
	if pos := p.borrar.Position(); pos.X != borrar {
		t.Errorf("el botón de eliminar hoja cae en x=%v y en la web en %v", pos.X, borrar)
	}
	if pos := p.mas.Position(); pos.X != borrar-2*(editor.LadoBotonCinta+HuecoPestanas) {
		t.Errorf("el botón de añadir hoja cae en x=%v", pos.X)
	}
	// `items-center` centra el botón en el contenido de la barra, bajo su borde.
	if y := p.borrar.Position().Y; y != tema.Borde+(AltoPestanas-tema.Borde-editor.LadoBotonCinta)/2 {
		t.Errorf("el botón cae en y=%v y debe estar centrado bajo el borde", y)
	}
}

// TestLaBarraDeHojasResumeElRango: el `<output>` solo sale con un rango que tenga algún
// número, y dice lo mismo que la web con el catálogo en castellano.
func TestLaBarraDeHojasResumeElRango(t *testing.T) {
	c := lienzoTraducido(t)
	c.Libro.Escribir(0, 0, "1")
	c.Libro.Escribir(0, 1, "2")
	c.Libro.Escribir(0, 2, "texto")
	c.Rejilla.Seleccionar(0, 0, false)
	if got := c.Pestanas.Resumen(); got != "" {
		t.Errorf("con una celda suelta no hay resumen y sale %q", got)
	}
	c.Rejilla.Seleccionar(0, 2, true)
	if got, quiero := c.Pestanas.Resumen(), "Recuento: 3 · Suma: 3 · Promedio: 1.5"; got != quiero {
		t.Errorf("el resumen de A1:A3 es %q y la web dice %q", got, quiero)
	}
}

// TestLasPestanasCambianDeHojaYSeCierranEnSoloLectura: pulsar cambia de hoja y vuelve a
// A1; en solo lectura se puede mirar otra hoja, pero no añadir, duplicar ni borrar.
func TestLasPestanasCambianDeHojaYSeCierranEnSoloLectura(t *testing.T) {
	c := lienzoTraducido(t)
	avisos := 0
	c.AlCambiar = func() { avisos++ }
	c.Rejilla.Seleccionar(4, 4, false)
	c.Pestanas.Refresh()
	c.Pestanas.tira.pestanas[1].Tapped(nil)
	if c.Libro.Hoja() != "Ventas" || c.Rejilla.Seleccion() != (Celda{}) || avisos != 1 {
		t.Errorf("pulsar «Ventas» deja la hoja %q, la selección en %v y %d avisos al cromo",
			c.Libro.Hoja(), c.Rejilla.Seleccion(), avisos)
	}
	c.AnadirHoja()
	if got := c.Libro.Hoja(); got != "Hoja 3" {
		t.Errorf("la hoja añadida se llama %q y la web la llama «Hoja 3»", got)
	}

	c.SoloLectura(true)
	c.AnadirHoja()
	c.DuplicarHoja()
	c.BorrarHoja()
	c.RenombrarHoja(0, "otra")
	if n := len(c.Libro.Hojas()); n != 3 || c.Libro.Hojas()[0] != "Hoja 1" {
		t.Errorf("en solo lectura ha cambiado la estructura: %v", c.Libro.Hojas())
	}
	if !c.Pestanas.mas.Inerte || !c.Pestanas.borrar.Inerte {
		t.Error("en solo lectura los botones de la barra siguen encendidos")
	}
	c.CambiarHoja(0)
	if c.Libro.Activa() != 0 {
		t.Error("en solo lectura no se puede ni mirar otra hoja")
	}
}

// TestLaTiraEnsenaSuBarraAlPasarPorEncima: con pestañas que no caben, el puntero sobre la
// tira (o sobre una pestaña) enseña el pulgar de `scrollbar-width: thin` —6 px, a 2 del
// borde de abajo, del largo de lo que se ve—, y al salir se esconde.
func TestLaTiraEnsenaSuBarraAlPasarPorEncima(t *testing.T) {
	conBarraClasica(t, false) // la de macOS, que se superpone
	c := NuevoLienzo(NuevoLibroCon([]string{"Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio", "Julio", "Agosto"}, 0))
	c.Resize(fyne.NewSize(360, 300))
	pinta(t, 360, 300, c)
	tira := c.Pestanas.tira
	r := test.WidgetRenderer(tira).(*tiraRenderer)
	if !r.pulgar.Hidden {
		t.Fatal("sin el puntero encima la barra se ve")
	}
	tira.pestanas[0].MouseIn(nil)
	total, ancho := tira.anchoContenido(), tira.Size().Width
	if ancho >= total {
		t.Fatalf("las ocho pestañas caben en %v (miden %v): la prueba no prueba nada", ancho, total)
	}
	if r.pulgar.Hidden {
		t.Fatal("con el puntero sobre una pestaña la barra no sale")
	}
	quiero := fyne.NewSize(ancho*ancho/total, AltoPulgarTira)
	if got := r.pulgar.Size(); got != quiero {
		t.Errorf("el pulgar mide %v y lo que se ve de la tira es %v", got, quiero)
	}
	if y := r.pulgar.Position().Y; y != tira.Size().Height-MargenPulgarTira-AltoPulgarTira {
		t.Errorf("el pulgar cae en y=%v", y)
	}
	tira.pestanas[0].MouseOut()
	if !r.pulgar.Hidden {
		t.Error("al salir el puntero la barra sigue")
	}
}

// conBarraClasica pone las barras del sistema de un tipo durante la prueba.
func conBarraClasica(t *testing.T, si bool) {
	antes := barraClasica
	barraClasica = si
	t.Cleanup(func() { barraClasica = antes })
}

// TestLaTiraReservaLaBarraClasica: en Linux y Windows, con pestañas que no caben, la
// barra no se superpone: se lleva 10 px de abajo aunque el puntero no esté encima, las
// pestañas se quedan con lo de arriba, el pulgar corre por la pista entre los dos botones
// de 12 y cada flecha corre la tira 40 px. Con las que caben, nada cambia.
func TestLaTiraReservaLaBarraClasica(t *testing.T) {
	conBarraClasica(t, true)
	c := NuevoLienzo(NuevoLibroCon([]string{"Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio", "Julio", "Agosto"}, 0))
	c.Resize(fyne.NewSize(360, 300))
	guarda(t, pinta(t, 360, 300, c))
	tira := c.Pestanas.tira
	r := test.WidgetRenderer(tira).(*tiraRenderer)
	total, ancho, alto := tira.anchoContenido(), tira.Size().Width, tira.Size().Height
	if r.pulgar.Hidden || !r.flechas[0].Objeto().Visible() || !r.flechas[1].Objeto().Visible() {
		t.Fatal("la barra clásica no se ve sin el puntero encima")
	}
	if h := tira.pestanas[0].Size().Height; h != alto-AltoBarraClasica || tira.pestanas[0].Position().Y != 0 {
		t.Errorf("la pestaña mide %v de alto en y=%v; le quedan %v", h, tira.pestanas[0].Position().Y, alto-AltoBarraClasica)
	}
	pista := ancho - 2*LadoBotonClasico
	if got, quiero := r.pulgar.Size(), fyne.NewSize(pista*ancho/total, AltoPulgarTira); got != quiero {
		t.Errorf("el pulgar mide %v y debería %v", got, quiero)
	}
	if got := r.pulgar.Position(); got != fyne.NewPos(LadoBotonClasico, alto-AltoBarraClasica+MargenPulgarTira) {
		t.Errorf("el pulgar empieza en %v", got)
	}
	tira.Tapped(&fyne.PointEvent{Position: fyne.NewPos(ancho-3, alto-3)})
	if tira.desX != PasoFlechaClasica {
		t.Errorf("la flecha de la derecha corre la tira %v", tira.desX)
	}
	tira.Tapped(&fyne.PointEvent{Position: fyne.NewPos(3, alto-3)})
	if tira.desX != 0 {
		t.Errorf("la de la izquierda la deja en %v", tira.desX)
	}
	tira.Tapped(&fyne.PointEvent{Position: fyne.NewPos(ancho-3, 3)}) // encima de la barra: nada
	if tira.desX != 0 {
		t.Error("un toque fuera de la barra corre la tira")
	}

	pocas := NuevoLienzo(NuevoLibroCon([]string{"Enero"}, 0))
	pocas.Resize(fyne.NewSize(360, 300))
	pinta(t, 360, 300, pocas)
	rp := test.WidgetRenderer(pocas.Pestanas.tira).(*tiraRenderer)
	if !rp.pulgar.Hidden || rp.flechas[0].Objeto().Visible() {
		t.Error("con las pestañas que caben sale la barra clásica")
	}
}

// TestElPulgarEncimaEsElDeChromium fija el borde de reposo desde tokens/colors.css:
// claro rgba(15,29,32,.6) sobre #fff; oscuro rgba(231,239,239,.42) sobre #131c1e.
// Con alfa de 8 bits (153 y 107), source-over da #6f7779 y #6c7576. La barra clásica
// de Chromium en Linux (2026-10-04, sin `--hide-scrollbars`) mide esos mismos colores
// en reposo, y #515b5d/#adb3b4 al arrastrar.
func TestElPulgarEncimaEsElDeChromium(t *testing.T) {
	if got := tema.Color("border-strong", tema.Claro); got != (color.NRGBA{R: 0x6f, G: 0x77, B: 0x79, A: 255}) {
		t.Errorf("border-strong claro es %v: el CSS compuesto sobre surface da #6f7779", got)
	}
	if got := tema.Color("border-strong", tema.Oscuro); got != (color.NRGBA{R: 0x6c, G: 0x75, B: 0x76, A: 255}) {
		t.Errorf("border-strong oscuro es %v: el CSS compuesto sobre surface da #6c7576", got)
	}
	for _, c := range []struct {
		variante            tema.Variante
		encima, arrastrando color.NRGBA
	}{
		{tema.Claro, color.NRGBA{R: 111, G: 119, B: 121, A: 255}, color.NRGBA{R: 81, G: 91, B: 93, A: 255}},
		{tema.Oscuro, color.NRGBA{R: 108, G: 117, B: 118, A: 255}, color.NRGBA{R: 173, G: 179, B: 180, A: 255}},
	} {
		if got := PulgarEncima(c.variante); got != c.encima {
			t.Errorf("pulgar con el puntero encima, variante %v: %v; Chromium %v", c.variante, got, c.encima)
		}
		if got := PulgarArrastrando(c.variante); got != c.arrastrando {
			t.Errorf("pulgar arrastrándose, variante %v: %v; Chromium %v", c.variante, got, c.arrastrando)
		}
	}
}

// TestElPulgarClasicoSeCogeYSeEnciende: en la barra clásica el pulgar conserva el color
// con el puntero encima y cambia al arrastrarlo, arrastrarlo corre la tira a escala, y un clic
// en la pista corre una página hacia ese lado.
func TestElPulgarClasicoSeCogeYSeEnciende(t *testing.T) {
	conBarraClasica(t, true)
	c := NuevoLienzo(NuevoLibroCon([]string{"Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio", "Julio", "Agosto"}, 0))
	c.Resize(fyne.NewSize(360, 300))
	pinta(t, 360, 300, c)
	tira := c.Pestanas.tira
	r := test.WidgetRenderer(tira).(*tiraRenderer)
	alto, ancho, total := tira.Size().Height, tira.Size().Width, tira.anchoContenido()
	x, _, largo, _ := tira.pulgar()
	medio := fyne.NewPos(x+largo/2, alto-AltoBarraClasica/2)
	reposo := color.NRGBA{R: 111, G: 119, B: 121, A: 255}
	if r.pulgar.FillColor != reposo {
		t.Errorf("en reposo el pulgar es %v", r.pulgar.FillColor)
	}
	tira.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: medio}})
	if r.pulgar.FillColor != reposo {
		t.Errorf("con el puntero encima el pulgar es %v y en Chromium %v", r.pulgar.FillColor, reposo)
	}
	tira.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: medio.AddXY(10, 0)}, Dragged: fyne.Delta{DX: 10}})
	if quiero := (color.NRGBA{R: 81, G: 91, B: 93, A: 255}); r.pulgar.FillColor != quiero {
		t.Errorf("arrastrando el pulgar es %v y en Chromium %v", r.pulgar.FillColor, quiero)
	}
	pista := ancho - 2*LadoBotonClasico
	if d := tira.desX - 10*total/pista; d > 0.01 || d < -0.01 {
		t.Errorf("arrastrar el pulgar 10 px corre la tira %v y debería %v", tira.desX, 10*total/pista)
	}
	tira.DragEnd()
	if r.pulgar.FillColor != reposo {
		t.Errorf("al soltar el pulgar es %v y en Chromium %v", r.pulgar.FillColor, reposo)
	}
	tira.desX = 0
	tira.Refresh()
	tira.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(ancho-20, alto-3)}, Dragged: fyne.Delta{DX: 10}})
	if tira.desX != 0 {
		t.Error("un arrastre que no empieza en el pulgar corre la tira")
	}
	tira.DragEnd()
	tira.Tapped(&fyne.PointEvent{Position: fyne.NewPos(ancho-LadoBotonClasico-2, alto-3)})
	if quiero := min(FraccionPagina*ancho, total-ancho); tira.desX != quiero {
		t.Errorf("un clic en la pista corre %v y en Chromium %v", tira.desX, quiero)
	}
}
