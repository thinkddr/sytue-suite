// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El mini-calendario de la barra lateral (`MiniMonth` de CalendarApp.tsx), sobre el
// fondo de marca oscuro. Medido en `calendar/mes.ancho.json`:
//
//	columna        32 de ancho · siete, dentro de los 224 útiles (231 − 2 × px-1)   medido
//	inicial        21 de alto (`h-6`), `text-2xs` en versalitas                     medido
//	día            24,5 de alto (`h-7`), `text-xs tabular-nums`                     medido
//	cabecera       `mb-1.5`, rótulo `text-sm font-semibold pl-1.5`, dos botones 24  hoja
//
// Los 32 de columna NO se declaran: salen de repartir el ancho útil entre siete con la
// misma cuantización que el navegador (`ui.Rejilla`). Que dé 32 clavado es la
// comprobación de que el reparto es el mismo, y eso es lo que mira el guardián.
const (
	// RellenoMiniMes es el `px-1` del bloque.
	RellenoMiniMes = 1 * 0.25 * tema.Rem
	// AltoInicialMini es el `h-6` de la inicial del día de la semana.
	AltoInicialMini = 6 * 0.25 * tema.Rem
	// AltoDiaMini es el `h-7` de cada día.
	AltoDiaMini = 7 * 0.25 * tema.Rem
	// LadoChevronMini es el `h-6 w-6` de las dos flechas de la cabecera.
	LadoChevronMini = 6 * 0.25 * tema.Rem
	// MargenCabeceraMini es el `mb-1.5` entre la cabecera y la rejilla.
	MargenCabeceraMini = 1.5 * 0.25 * tema.Rem
	// MargenMiniMes es el `mb-4` de debajo del bloque entero.
	MargenMiniMes = 4 * 0.25 * tema.Rem
)

// cuadriculaMini reparte las siete columnas del mini-mes con la cuantización del
// navegador, igual que la rejilla grande.
var cuadriculaMini = ui.Rejilla{Fijas: 7}

// MiniMes es el mini-calendario: cabecera con mes y flechas, iniciales de los días y
// seis semanas de números.
type MiniMes struct {
	widget.BaseWidget

	Locale i18n.Locale
	Cursor time.Time
	// Zona es la de visualización; nil, la del sistema.
	Zona *time.Location
	// AlElegir fija el cursor en ese día; AlDesplazar cambia de mes.
	AlElegir    func(time.Time)
	AlDesplazar func(delta int)
	// RotuloAnterior y RotuloSiguiente son los `aria-label` de las flechas.
	RotuloAnterior, RotuloSiguiente string
	// Superficie lo pinta sobre una superficie clara —el desplegable de fecha del
	// formulario— en vez de sobre el cromo: los tokens de texto en lugar de los del
	// lateral, flechas de cromo corriente y el día ELEGIDO en tinta, que ahí es el valor
	// del campo y lo que hay que ver.
	Superficie bool
}

// tinta es el token de color de la pieza: el del lateral, o su equivalente sobre una
// superficie clara.
func (m *MiniMes) tinta(token string) string {
	if !m.Superficie {
		return token
	}
	switch token {
	case "sidebar-fg":
		return "text"
	case "sidebar-fg-muted":
		return "text-muted"
	case "sidebar-hover-bg":
		return "bg-subtle"
	case "sidebar-focus":
		return "focus-ring"
	}
	return token
}

var _ fyne.Widget = (*MiniMes)(nil)

// NuevoMiniMes crea el bloque.
func NuevoMiniMes(l i18n.Locale, cursor time.Time) *MiniMes {
	m := &MiniMes{Locale: l, Cursor: cursor}
	m.ExtendBaseWidget(m)
	return m
}

// AccessibilityRole: agrupa botones de día.
func (m *MiniMes) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AccessibilityLabel es el mes que enseña.
func (m *MiniMes) AccessibilityLabel() string { return i18n.FormatMonthYear(m.Locale, m.Cursor) }

func (m *MiniMes) zona() *time.Location {
	if m.Zona == nil {
		return time.Local
	}
	return m.Zona
}

// MedidaMiniMes es lo que ocupa: cabecera, fila de iniciales y seis filas de días, más
// el `mb-4`.
func MedidaMiniMes() fyne.Size {
	alto := max(tema.TextoSM.Linea, LadoChevronMini) + MargenCabeceraMini +
		AltoInicialMini + 6*AltoDiaMini + MargenMiniMes
	return fyne.NewSize(0, alto)
}

// CreateRenderer arma cabecera, iniciales y 42 días.
func (m *MiniMes) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &miniMesRenderer{
		m:      m,
		titulo: ui.Texto(capitalizar(i18n.FormatMonthYear(m.Locale, m.Cursor)), tema.TextoSM, ui.SemiNegrita, tema.Color(m.tinta("sidebar-fg"), v)),
	}
	r.objetos = []fyne.CanvasObject{r.titulo}
	// Las flechas del mini-mes son `h-6 w-6` sobre el cromo, con su aro BLANCO: el
	// azul global no contrasta sobre la tinta, que es lo que dice SidebarItem. Por eso
	// van con `ui.NuevaAccionLateral`, que es justo esa pieza del catálogo.
	// El glifo va a 16 (`<Icon size={16} />`), centrado en la caja de 24.
	if m.Superficie {
		antes := ui.NuevoBotonIcono(iconos.ChevronIzquierda, m.RotuloAnterior, func() { m.desplazar(-1) })
		despues := ui.NuevoBotonIcono(iconos.ChevronDerecha, m.RotuloSiguiente, func() { m.desplazar(1) })
		antes.Lado, despues.Lado = LadoChevronMini, LadoChevronMini
		r.antes, r.despues = antes, despues
	} else {
		izq := iconos.Trazo(iconos.ChevronIzquierda, 16, "sidebar-fg-muted", v)
		der := iconos.Trazo(iconos.ChevronDerecha, 16, "sidebar-fg-muted", v)
		r.antes = ui.NuevaAccionLateralCon(m.RotuloAnterior, izq, func() { m.desplazar(-1) })
		r.despues = ui.NuevaAccionLateralCon(m.RotuloSiguiente, der, func() { m.desplazar(1) })
	}
	r.objetos = append(r.objetos, r.antes, r.despues)

	// `text-2xs font-semibold uppercase` y SIN tracking: no es `sytue-overline`. Con
	// `ui.Versal` cada inicial ganaba 0,06 em a su derecha y salía descentrada.
	for _, ini := range i18n.WeekdaysNarrow(m.Locale) {
		s := ui.Texto(ui.Versalitas(ini), tema.Texto2XS, ui.SemiNegrita, tema.Color(m.tinta("sidebar-fg-muted"), v))
		r.iniciales = append(r.iniciales, s)
		r.objetos = append(r.objetos, s)
	}
	for _, d := range Dias(m.Cursor, m.zona()) {
		b := nuevoDiaMini(m, d)
		r.dias = append(r.dias, b)
		r.objetos = append(r.objetos, b)
	}
	return r
}

func (m *MiniMes) desplazar(delta int) {
	if m.AlDesplazar != nil {
		m.AlDesplazar(delta)
	}
}

type miniMesRenderer struct {
	m              *MiniMes
	titulo         *canvas.Text
	antes, despues fyne.CanvasObject
	iniciales      []*canvas.Text
	dias           []*diaMini
	objetos        []fyne.CanvasObject
}

func (r *miniMesRenderer) MinSize() fyne.Size           { return MedidaMiniMes() }
func (r *miniMesRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *miniMesRenderer) Destroy()                     {}

// Layout coloca cabecera, iniciales y días. El ancho de columna sale de `Rejilla` con
// el ancho ÚTIL (el del bloque menos su `px-1` a cada lado).
func (r *miniMesRenderer) Layout(t fyne.Size) {
	util := max(t.Width-2*RellenoMiniMes, 0)
	altoCab := max(tema.TextoSM.Linea, LadoChevronMini)
	// `pl-1.5` sobre el rótulo, y las flechas pegadas al margen derecho con `gap-0.5`.
	ui.ColocarTexto(r.titulo, tema.TextoSM, RellenoMiniMes+espacio1p5, (altoCab-tema.TextoSM.Linea)/2)
	x := RellenoMiniMes + util
	for _, b := range [...]fyne.CanvasObject{r.despues, r.antes} {
		m := b.MinSize()
		x -= m.Width
		b.Move(fyne.NewPos(x, (altoCab-m.Height)/2))
		b.Resize(m)
		x -= espacio0p5
	}

	y := altoCab + MargenCabeceraMini
	for c, s := range r.iniciales {
		// `grid h-6 place-items-center`: la inicial va centrada en su columna.
		col := RellenoMiniMes + cuadriculaMini.X(util, c)
		ancho := ui.Medir(s.Text, tema.Texto2XS, ui.SemiNegrita).Width
		ui.ColocarTexto(s, tema.Texto2XS, col+(cuadriculaMini.AnchoColumna(util)-ancho)/2, y+(AltoInicialMini-tema.Texto2XS.Linea)/2)
	}
	y += AltoInicialMini
	ancho := cuadriculaMini.AnchoColumna(util)
	for i, b := range r.dias {
		fila, col := i/7, i%7
		b.Move(fyne.NewPos(RellenoMiniMes+cuadriculaMini.X(util, col), y+float32(fila)*AltoDiaMini))
		b.Resize(fyne.NewSize(ancho, AltoDiaMini))
	}
}

func (r *miniMesRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.titulo.Text = capitalizar(i18n.FormatMonthYear(r.m.Locale, r.m.Cursor)) // `titleFor('month')`, con su mayúscula
	r.titulo.Color = tema.Color(r.m.tinta("sidebar-fg"), v)
	for _, s := range r.iniciales {
		s.Color = tema.Color(r.m.tinta("sidebar-fg-muted"), v)
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// diaMini es un día del mini-calendario: `grid h-7 place-items-center rounded-md
// text-xs tabular-nums`, con tres estados (hoy, seleccionado, del mes o de fuera).
//
// Puntero, pulsación, teclado y rol de botón son los de `ui.Pulsable`.
type diaMini struct {
	ui.Pulsable
	m   *MiniMes
	dia time.Time
}

var _ fyne.Widget = (*diaMini)(nil)

func nuevoDiaMini(m *MiniMes, d time.Time) *diaMini {
	b := &diaMini{m: m, dia: d}
	b.AlPulsar = func() {
		if m.AlElegir != nil {
			m.AlElegir(d)
		}
	}
	b.ExtendBaseWidget(b)
	return b
}

// AccessibilityLabel es el `aria-label` de la web: el día con su mes y su año.
func (b *diaMini) AccessibilityLabel() string { return i18n.FormatDate(b.m.Locale, b.dia) }

// El rol es el botón de `ui.Pulsable`. El `aria-current="date"` de hoy NO llega al
// lector: Fyne 2.8.1 solo conoce cuatro roles y ninguno lleva estado.

func (b *diaMini) MinSize() fyne.Size { return fyne.NewSize(0, AltoDiaMini) }

// peso es el de la web: `font-semibold` hoy y `font-medium` el elegido. Se fija al
// construir el texto porque medir y pintar tienen que usar la MISMA cara (regla 2 de
// `ui`): antes se medía con la semi-negrita y se pintaba con la regular, y el número de
// hoy salía descentrado en su casilla.
func (b *diaMini) peso() ui.Peso {
	switch {
	case esHoy(b.dia):
		return ui.SemiNegrita
	case mismoDia(b.dia, b.m.Cursor):
		return ui.Medio
	}
	return ui.Normal
}

func (b *diaMini) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &diaMiniRenderer{
		b:     b,
		fondo: canvas.NewRectangle(color.NRGBA{}),
		texto: ui.Texto(strconv.Itoa(b.dia.Day()), tema.TextoXS, b.peso(), tema.Color(b.m.tinta("sidebar-fg"), v)),
		aro:   ui.NuevoAroCon(b.m.tinta("sidebar-focus"), tema.RadioMD, 0, v).Abierto(),
	}
	r.fondo.CornerRadius = tema.RadioMD
	r.objetos = []fyne.CanvasObject{r.fondo, r.texto}
	r.objetos = append(r.objetos, r.aro.Objetos()...)
	r.aro.Ver(false)
	r.Refresh() // regla 7 de `ui`: hoy y el elegido nacen ya pintados
	return r
}

type diaMiniRenderer struct {
	b       *diaMini
	fondo   *canvas.Rectangle
	texto   *canvas.Text
	aro     *ui.Aro
	objetos []fyne.CanvasObject
}

func (r *diaMiniRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *diaMiniRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *diaMiniRenderer) Destroy()                     {}

func (r *diaMiniRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	ancho := ui.Medir(r.texto.Text, tema.TextoXS, r.b.peso()).Width
	ui.ColocarTexto(r.texto, tema.TextoXS, (t.Width-ancho)/2, (t.Height-tema.TextoXS.Linea)/2)
	r.aro.Colocar(t)
}

func (r *diaMiniRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := r.b
	delMes := b.dia.Month() == b.m.Cursor.Month() && b.dia.Year() == b.m.Cursor.Year()
	elegido := mismoDia(b.dia, b.m.Cursor)
	t := b.m.tinta
	switch {
	case b.m.Superficie && elegido:
		// En el desplegable del formulario el elegido es el VALOR del campo: en tinta.
		// Hoy se distingue solo por su peso.
		r.fondo.FillColor = tema.Color("primary", v)
		r.texto.Color = tema.Color("on-primary", v)
	case b.m.Superficie && esHoy(b.dia):
		r.fondo.FillColor = color.NRGBA{}
		if b.Encima() {
			r.fondo.FillColor = tema.Color("bg-subtle", v)
		}
		r.texto.Color = tema.Color("text", v)
	case esHoy(b.dia):
		// `bg-primary text-on-primary`, pero DENTRO del carril, donde el primario es el
		// suyo (appearance.css): blanco con la cifra del color del carril en claro.
		r.fondo.FillColor = tema.Color("sidebar-primary", v)
		r.texto.Color = tema.Color("sidebar-on-primary", v)
	case elegido, b.Encima():
		r.fondo.FillColor = tema.Color(t("sidebar-hover-bg"), v)
		r.texto.Color = tema.Color(t("sidebar-fg"), v)
	case delMes:
		r.fondo.FillColor = color.NRGBA{}
		r.texto.Color = tema.Color(t("sidebar-fg"), v)
	default:
		r.fondo.FillColor = color.NRGBA{}
		r.texto.Color = tema.Color(t("sidebar-fg-muted"), v)
	}
	r.fondo.Refresh()
	r.texto.Refresh()
	r.aro.Ver(b.Enfocado())
}

// mismoDia compara por día natural, como `isSameDay` de dates.ts.
func mismoDia(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}
