// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los tres rectángulos de la rejilla horaria: `EventBlock` (el evento), `DragBlock` (el
// fantasma de lo que se arrastra o se está guardando) y `CreateDraft` (el rango que se
// pinta al crear). Los dos últimos no se pulsan: `pointer-events-none`.

// bloqueHorario es `EventBlock`:
//
//	border-border bg-bg-subtle text-text absolute rounded-sm border
//	hover:border-border-strong hover:shadow-[var(--shadow-xs)]
//	<span class="absolute inset-y-0 left-0 w-[3px]" style="background: color" />
//	<div class="py-0.5 pr-1 pl-2">título `text-xs leading-tight font-medium` ·
//	    franja `text-2xs leading-tight text-text-muted` (con 34 px de alto o más)
//
// El arrastre no es suyo sino del cuerpo, que sabe qué bloque hay bajo el puntero: así
// los tres gestos se deciden en un sitio.
type bloqueHorario struct {
	widget.BaseWidget
	p         *Pantalla
	bl        Bloque[Ocurrencia]
	dia       int
	sinTitulo string

	encima, enfocado bool
}

var (
	_ fyne.Tappable     = (*bloqueHorario)(nil)
	_ fyne.Focusable    = (*bloqueHorario)(nil)
	_ desktop.Hoverable = (*bloqueHorario)(nil)
)

func nuevoBloqueHorario(p *Pantalla, bl Bloque[Ocurrencia], dia int, sinTitulo string) *bloqueHorario {
	b := &bloqueHorario{p: p, bl: bl, dia: dia, sinTitulo: sinTitulo}
	b.ExtendBaseWidget(b)
	return b
}

// Tapped abre la vista rápida: es el «clic sin mover» de la web.
func (b *bloqueHorario) Tapped(*fyne.PointEvent) {
	if b.p.AlAbrirEvento != nil {
		b.p.AlAbrirEvento(b.bl.Item, b)
	}
}

func (b *bloqueHorario) FocusGained()                   { b.enfocado = true; b.Refresh() }
func (b *bloqueHorario) FocusLost()                     { b.enfocado = false; b.Refresh() }
func (b *bloqueHorario) TypedRune(rune)                 {}
func (b *bloqueHorario) TypedKey(e *fyne.KeyEvent)      { pulsarConTecla(b, e, func() { b.Tapped(nil) }) }
func (b *bloqueHorario) MouseIn(*desktop.MouseEvent)    { b.encima = true; b.Refresh() }
func (b *bloqueHorario) MouseMoved(*desktop.MouseEvent) {}
func (b *bloqueHorario) MouseOut()                      { b.encima = false; b.Refresh() }

// AccessibilityLabel es «título, hora» (`grid.blockAria`).
func (b *bloqueHorario) AccessibilityLabel() string {
	ini, _ := ocurrenciaInstantes(b.bl.Item, b.p.zona())
	return b.p.t("grid.blockAria", i18n.Vars{
		"title": b.titulo(), "time": i18n.FormatTimeDigits(b.p.locale(), ini),
	})
}
func (b *bloqueHorario) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (b *bloqueHorario) titulo() string { return resumenDe(b.bl.Item, b.sinTitulo) }

func (b *bloqueHorario) rango() string {
	ini, _ := ocurrenciaInstantes(b.bl.Item, b.p.zona())
	return rangoHoras(b.p, ini, b.bl.Desde, b.bl.Hasta, b.p.zona())
}

func (b *bloqueHorario) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &bloqueRenderer{
		b:      b,
		caja:   ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioSM),
		franja: canvas.NewRectangle(b.bl.Item.Color),
		titulo: ui.Texto(b.titulo(), tema.TextoXS, ui.Medio, tema.Color("text", v)),
		rango:  ui.Texto(b.rango(), tema.Texto2XS, ui.Normal, tema.Color("text-muted", v)),
		aro:    ui.NuevoAro(tema.RadioSM, 0, v),
	}
	// La franja va DENTRO del borde y la recorta el `overflow-hidden` con el radio de
	// dentro, uno menos que el de la caja.
	r.franja.TopLeftCornerRadius = tema.RadioSM - tema.Borde
	r.franja.BottomLeftCornerRadius = tema.RadioSM - tema.Borde
	r.objetos = r.aro.Antes(append(r.caja.Objetos(), r.franja, r.titulo, r.rango)...)
	r.Refresh()
	return r
}

type bloqueRenderer struct {
	b             *bloqueHorario
	caja          *ui.Caja
	franja        *canvas.Rectangle
	titulo, rango *canvas.Text
	aro           *ui.Aro
	objetos       []fyne.CanvasObject
}

func (r *bloqueRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *bloqueRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *bloqueRenderer) Destroy()                     {}

func (r *bloqueRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	colocarContenidoBloque(t, r.franja, r.titulo, r.rango, r.b.titulo(), r.b.rango(), t.Height >= AltoConRango)
	r.aro.Colocar(t)
}

// colocarContenidoBloque reparte lo de dentro de un bloque: la franja de color pegada al
// borde de la izquierda, y el título y la franja horaria dentro de `py-0.5 pr-1 pl-2`.
func colocarContenidoBloque(t fyne.Size, franja *canvas.Rectangle, titulo, rango *canvas.Text, tt, rt string, conRango bool) {
	franja.Move(fyne.NewPos(tema.Borde, tema.Borde))
	franja.Resize(fyne.NewSize(AnchoFranjaColor, max(t.Height-2*tema.Borde, 0)))
	x := tema.Borde + espacio2
	cabe := max(t.Width-x-tema.Espacio(1)-tema.Borde, 0)
	y := tema.Borde + espacio0p5
	ui.Truncar(titulo, tt, tema.TextoXS, ui.Medio, cabe)
	ui.ColocarTexto(titulo, escalonTight(tema.TextoXS), x, y)
	rango.Hidden = !conRango
	if conRango {
		ui.Truncar(rango, rt, tema.Texto2XS, ui.Normal, cabe)
		ui.ColocarTexto(rango, escalonTight(tema.Texto2XS), x, y+escalonTight(tema.TextoXS).Linea)
	}
}

func (r *bloqueRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	borde := tema.Color("border", v)
	if r.b.encima {
		borde = tema.Color("border-strong", v)
	}
	r.caja.Pintar(tema.Color("bg-subtle", v), borde)
	r.franja.FillColor = r.b.bl.Item.Color
	r.franja.Refresh()
	r.titulo.Color = tema.Color("text", v)
	r.rango.Color = tema.Color("text-muted", v)
	r.titulo.Refresh()
	r.rango.Refresh()
	r.aro.Ver(r.b.enfocado)
}

// bloqueFantasma es `DragBlock`: `bg-surface border shadow-sm`, con la franja en el
// acento por defecto y siempre con su rango. Al guardar va al 70 % (`opacity-70`).
//
// ponytail: Fyne no tiene opacidad de grupo, así que el 70 % se aplana contra la
// superficie; sobre una línea de hora el píxel de debajo no se ve a través. Es el mismo
// aplanado que la cortina de recarga y no se nota fuera de ese píxel.
type bloqueFantasma struct {
	widget.BaseWidget
	titulo, rango string
	guardando     bool
}

func nuevoBloqueFantasma() *bloqueFantasma {
	f := &bloqueFantasma{}
	f.ExtendBaseWidget(f)
	f.Hide()
	return f
}

func (f *bloqueFantasma) poner(titulo, rango string, guardando bool) {
	f.titulo, f.rango, f.guardando = titulo, rango, guardando
	f.Refresh()
}

func (f *bloqueFantasma) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &fantasmaRenderer{
		f:      f,
		sombra: ui.NuevaSombra(tema.SombraSM, tema.RadioSM),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioSM),
		franja: canvas.NewRectangle(AcentoPorDefecto),
		titulo: ui.Texto("", tema.TextoXS, ui.Medio, tema.Color("text", v)),
		rango:  ui.Texto("", tema.Texto2XS, ui.Normal, tema.Color("text-muted", v)),
	}
	r.franja.TopLeftCornerRadius = tema.RadioSM - tema.Borde
	r.franja.BottomLeftCornerRadius = tema.RadioSM - tema.Borde
	r.objetos = append([]fyne.CanvasObject{r.sombra.Objeto()}, r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.franja, r.titulo, r.rango)
	r.Refresh()
	return r
}

type fantasmaRenderer struct {
	f             *bloqueFantasma
	sombra        *ui.Sombra
	caja          *ui.Caja
	franja        *canvas.Rectangle
	titulo, rango *canvas.Text
	objetos       []fyne.CanvasObject
}

func (r *fantasmaRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *fantasmaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *fantasmaRenderer) Destroy()                     {}

func (r *fantasmaRenderer) Layout(t fyne.Size) {
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.caja.Redimensionar(t)
	colocarContenidoBloque(t, r.franja, r.titulo, r.rango, r.f.titulo, r.f.rango, true)
}

func (r *fantasmaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := tema.Color("surface", v)
	tinta := func(c color.NRGBA) color.NRGBA {
		if r.f.guardando {
			return mezclar(fondo, c, 0.7)
		}
		return c
	}
	r.caja.Pintar(fondo, tinta(tema.Color("border", v)))
	r.franja.FillColor = tinta(AcentoPorDefecto)
	r.titulo.Text, r.rango.Text = r.f.titulo, r.f.rango
	r.titulo.Color, r.rango.Color = tinta(tema.Color("text", v)), tinta(tema.Color("text-muted", v))
	r.Layout(r.f.Size())
	for _, o := range []fyne.CanvasObject{r.franja, r.titulo, r.rango} {
		o.Refresh()
	}
}

// borrador es `CreateDraft`: el rango que se pinta al crear arrastrando.
//
//	border-primary bg-primary/10 absolute inset-x-0.5 rounded-sm border
//	<div class="text-primary-text text-2xs px-2 py-0.5 font-medium tabular-nums">
type borrador struct {
	widget.BaseWidget
	rotulo string
}

func nuevoBorrador() *borrador {
	b := &borrador{}
	b.ExtendBaseWidget(b)
	b.Hide()
	return b
}

func (b *borrador) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &borradorRenderer{
		b:     b,
		caja:  ui.NuevaCaja(color.NRGBA{}, tema.Color("primary", v), tema.RadioSM),
		texto: ui.Texto("", tema.Texto2XS, ui.Medio, tema.Color("primary-text", v)),
	}
	r.objetos = append(r.caja.Objetos(), r.texto)
	r.Refresh()
	return r
}

type borradorRenderer struct {
	b       *borrador
	caja    *ui.Caja
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *borradorRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *borradorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *borradorRenderer) Destroy()                     {}

func (r *borradorRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	ui.ColocarTexto(r.texto, tema.Texto2XS, tema.Borde+espacio2, tema.Borde+espacio0p5)
}

// Refresh: `bg-primary/10` es el token al 10 % de su alfa, y como la caja pinta el borde
// como un rectángulo lleno por debajo del relleno, el relleno se compone YA sobre la
// superficie para que el borde no asome por dentro.
func (r *borradorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(mezclar(tema.Color("surface", v), tema.Color("primary", v), 0.1), tema.Color("primary", v))
	r.texto.Text = r.b.rotulo
	r.texto.Color = tema.Color("primary-text", v)
	r.texto.Refresh()
	r.Layout(r.b.Size())
}
