// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor/figuras"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los otros dos controles de `ribbon/primitives.tsx`: el selector compacto y la muestra
// de color con su paleta. Con BotonCinta, Grupo y SeparadorGrupos, son las cinco
// primitivas de la cinta.

// ---- paletas de CONTENIDO (§5) ----

// PaletaContenido es CONTENT_PALETTE: los colores que la persona aplica a SU documento.
// No son tokens de interfaz y por eso van en hex, y en hex se guardan en el modelo.
var PaletaContenido = figuras.Paleta

// PaletaResaltado es HIGHLIGHT_PALETTE. 'transparent' = quitar.
var PaletaResaltado = []string{
	"#FEF08A", "#FED7AA", "#BBF7D0", "#BFDBFE", "#E9D5FF", "#FBCFE8", "#E2E8F0", "transparent",
}

// Hex lee un color de contenido `#RRGGBB`. Devuelve false en 'transparent', en vacío y
// en cualquier cosa que no sepa leer: quien llama decide qué hacer con eso, que no es
// lo mismo en un relleno (sin relleno) que en un trazo (el de por defecto).
func Hex(s string) (color.NRGBA, bool) {
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, false
	}
	var v [3]uint8
	for i := range v {
		a, okA := nibble(s[1+2*i])
		b, okB := nibble(s[2+2*i])
		if !okA || !okB {
			return color.NRGBA{}, false
		}
		v[i] = a<<4 | b
	}
	return color.NRGBA{R: v[0], G: v[1], B: v[2], A: 255}, true
}

func nibble(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// ---- SelectorCinta ----

// Opcion es una fila de un SelectorCinta: el valor que se guarda y su rótulo YA
// traducido.
type Opcion struct{ Valor, Rotulo string }

// MedidaSelectorCinta es `h-7` y el `width ?? 120` de RibbonSelect.
func MedidaSelectorCinta(ancho float32) fyne.Size {
	if ancho <= 0 {
		ancho = AnchoSelectorPorDefecto
	}
	return fyne.NewSize(ui.SueloTactil(ancho), ui.SueloTactil(AltoSelectorCinta))
}

// SelectorCinta es RibbonSelect: el `<select>` compacto de la cinta.
//
//	border-border bg-surface text-text hover:bg-bg-subtle h-7 w-full appearance-none
//	rounded-sm border pr-6 pl-1.5 text-xs    + chevrón `sm` en `right-1.5` text-subtle
//
// No es `ui.Selector`, y por lo mismo que la pestaña de la cinta no es `ui.Pestana`: el
// `Select` de Form.tsx es un `h-9` con borde `border-strong` que cambia al pasar por
// encima; este mide 24,5, su borde es `border` y lo que cambia al pasar es el FONDO.
//
// La lista abierta es la del SISTEMA en la web, sin un solo número de CSS que copiar;
// aquí es el menú denso de la suite, como hace `ui.Selector`.
type SelectorCinta struct {
	activable

	Opciones []Opcion
	Valor    string
	Ancho    float32
	// Alto es el de la caja; cero es el `h-7` de la cinta. Los `<select>` de los paneles
	// de Diapos miden `h-6`.
	Alto     float32
	AlElegir func(valor string)

	menu *ui.Menu
}

var _ fyne.Widget = (*SelectorCinta)(nil)

// NuevoSelectorCinta crea el selector. `rotulo` es el `aria-label`, ya traducido.
func NuevoSelectorCinta(rotulo string, ancho float32, opciones []Opcion, valor string, alElegir func(string)) *SelectorCinta {
	s := &SelectorCinta{Opciones: opciones, Valor: valor, Ancho: ancho, AlElegir: alElegir}
	s.Rotulo = rotulo
	s.AlPulsar = s.abrir
	s.ExtendBaseWidget(s)
	return s
}

// MinSize es la medida declarada.
func (s *SelectorCinta) MinSize() fyne.Size {
	s.ExtendBaseWidget(s)
	m := MedidaSelectorCinta(s.Ancho)
	if s.Alto > 0 {
		m.Height = ui.SueloTactil(s.Alto)
	}
	return m
}

// rotuloActual es el de la opción con el valor actual; sin ninguna, vacío: un `<select>`
// controlado de Preact con un valor que no está entre sus opciones queda con
// `selectedIndex = -1`, en blanco, y no enseña la primera.
func (s *SelectorCinta) rotuloActual() string {
	for _, o := range s.Opciones {
		if o.Valor == s.Valor {
			return o.Rotulo
		}
	}
	return ""
}

func (s *SelectorCinta) elegir(v string) {
	if v == s.Valor {
		return
	}
	s.Valor = v
	s.Refresh()
	if s.AlElegir != nil {
		s.AlElegir(v)
	}
}

func (s *SelectorCinta) abrir() {
	if s.menu != nil && s.menu.Abierto() {
		s.menu.Cerrar()
		return
	}
	elems := make([]fyne.CanvasObject, len(s.Opciones))
	for i, o := range s.Opciones {
		e := ui.NuevoElementoMenu(o.Rotulo, iconos.Ninguno, func() { s.elegir(o.Valor) })
		e.Denso, e.Marcada = true, o.Valor == s.Valor
		elems[i] = e
	}
	s.menu = ui.NuevoMenu(elems...)
	s.menu.AlInicio, s.menu.Rotulo = true, s.Rotulo
	s.menu.Mostrar(s)
}

// TypedKey es el de un `<select>` cerrado: las flechas cambian el valor sin abrir, e
// Intro abre la lista.
func (s *SelectorCinta) TypedKey(e *fyne.KeyEvent) {
	if e == nil || s.Inerte {
		return
	}
	i := -1
	for k, o := range s.Opciones {
		if o.Valor == s.Valor {
			i = k
		}
	}
	switch e.Name {
	case fyne.KeyDown:
		if i+1 < len(s.Opciones) {
			s.elegir(s.Opciones[i+1].Valor)
		}
	case fyne.KeyUp:
		if i > 0 {
			s.elegir(s.Opciones[i-1].Valor)
		}
	default:
		s.activable.TypedKey(e)
	}
}

// CreateRenderer compone el aro, la caja, el valor y el chevrón.
func (s *SelectorCinta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &selectorCintaRenderer{
		s:       s,
		aro:     ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		caja:    ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioSM),
		valor:   ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text", v)),
		chevron: iconos.TrazoEd(iconos.EdChevronAbajo, IconoChevron, "text-subtle", v),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.valor, r.chevron.Objeto())
	r.Refresh()
	return r
}

type selectorCintaRenderer struct {
	s       *SelectorCinta
	aro     *ui.Aro
	caja    *ui.Caja
	valor   *canvas.Text
	chevron *iconos.Dibujo
	objetos []fyne.CanvasObject
}

func (r *selectorCintaRenderer) MinSize() fyne.Size           { return r.s.MinSize() }
func (r *selectorCintaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *selectorCintaRenderer) Destroy()                     {}

func (r *selectorCintaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	// El `<select>` recorta su texto en el hueco que dejan `pl-1.5` y `pr-6`; con
	// puntos suspensivos, que es lo que hace `text-overflow` en los navegadores que lo
	// aplican al select cerrado.
	x := tema.Borde + IzqSelector
	ui.Truncar(r.valor, r.s.rotuloActual(), tema.TextoXS, ui.Normal, t.Width-x-DerSelector-tema.Borde)
	ui.ColocarTexto(r.valor, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
	// `right-1.5`, medido desde la caja de BORDE del envoltorio.
	r.chevron.Colocar(t.Width-IzqSelector-IconoChevron, (t.Height-IconoChevron)/2, IconoChevron)
}

func (r *selectorCintaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := tema.Color("surface", v), tema.Color("text", v)
	if r.s.Encima() {
		fondo = tema.Color("bg-subtle", v)
	}
	borde, chevron := tema.Color("border", v), tema.Color("text-subtle", v)
	if r.s.Inerte {
		fondo, borde, tinta, chevron = atenuar40(fondo), atenuar40(borde), atenuar40(tinta), atenuar40(chevron)
	}
	r.caja.Pintar(fondo, borde)
	r.valor.Color = tinta
	r.valor.Refresh()
	r.chevron.TenirCon(chevron)
	r.aro.Pintar(v)
	r.aro.Ver(r.s.Enfocado())
	r.Layout(r.s.Size())
}

// ---- MuestraCinta ----

// Medidas de RibbonColorSwatch: la barrita `bottom-1 h-[3px] w-4 rounded-[1px]` y, en
// la paleta, las muestras `h-6 w-6 rounded-[3px] border` en `grid-cols-6 gap-1 p-2`.
var (
	AnchoBarrita          = tema.Espacio(4)
	AltoBarrita   float32 = 3
	AbajoBarrita          = tema.Espacio(1)
	LadoMuestra           = tema.Espacio(6)
	HuecoPaleta           = tema.Espacio(1)
	RellenoPaleta         = tema.Espacio(2)
)

// ColumnasPaleta es el `grid-cols-6`.
const ColumnasPaleta = 6

// MuestraCinta es RibbonColorSwatch: el botón con el glifo y, debajo, la barrita del
// color actual; al pulsarlo, la paleta.
type MuestraCinta struct {
	activable

	Icono iconos.Ed
	// Color es el actual, en hex o 'transparent'.
	Color    string
	Paleta   []string
	AlElegir func(color string)

	ctx       *Contexto
	emergente *ui.Emergente
	paleta    *rejillaPaleta
}

var _ fyne.Widget = (*MuestraCinta)(nil)

// NuevaMuestraCinta crea la muestra. `rotulo` es el `aria-label`, ya traducido; con
// `paleta` nil, CONTENT_PALETTE.
func NuevaMuestraCinta(ctx *Contexto, i iconos.Ed, rotulo, actual string, paleta []string, alElegir func(string)) *MuestraCinta {
	if paleta == nil {
		paleta = PaletaContenido
	}
	m := &MuestraCinta{Icono: i, Color: actual, Paleta: paleta, AlElegir: alElegir, ctx: ctx}
	m.Rotulo = rotulo
	m.AlPulsar = m.abrir
	m.ExtendBaseWidget(m)
	return m
}

// MinSize es el `h-7 w-7` del botón.
func (m *MuestraCinta) MinSize() fyne.Size {
	m.ExtendBaseWidget(m)
	return MedidaBotonCinta()
}

func (m *MuestraCinta) abrir() {
	if m.emergente != nil && m.emergente.Abierto() {
		m.emergente.Cerrar()
		return
	}
	celdas := make([]fyne.CanvasObject, len(m.Paleta))
	for i, c := range m.Paleta {
		nombre := m.ctx.Traducir("palette.color", i18n.Vars{"hex": c})
		if c == "transparent" {
			nombre = m.ctx.Traducir("palette.none")
		}
		celdas[i] = nuevaCeldaPaleta(c, nombre, c == m.Color, func() {
			m.emergente.Cerrar()
			m.Color = c
			m.Refresh()
			if m.AlElegir != nil {
				m.AlElegir(c)
			}
		})
	}
	m.paleta = &rejillaPaleta{celdas: celdas}
	m.emergente = ui.MostrarEmergente(m, m.paleta, RellenoPaleta)
}

// CreateRenderer compone el aro, la caja, el glifo y la barrita.
func (m *MuestraCinta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &muestraCintaRenderer{
		m:       m,
		aro:     ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		caja:    ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM),
		glifo:   iconos.TrazoEd(m.Icono, IconoBotonCinta, "text-muted", v),
		barrita: canvas.NewRectangle(color.NRGBA{}),
	}
	r.barrita.CornerRadius = 1
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.glifo.Objeto(), r.barrita)
	r.Refresh()
	return r
}

type muestraCintaRenderer struct {
	m       *MuestraCinta
	aro     *ui.Aro
	caja    *ui.Caja
	glifo   *iconos.Dibujo
	barrita *canvas.Rectangle
	objetos []fyne.CanvasObject
}

func (r *muestraCintaRenderer) MinSize() fyne.Size           { return MedidaBotonCinta() }
func (r *muestraCintaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *muestraCintaRenderer) Destroy()                     {}

func (r *muestraCintaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	r.glifo.Colocar((t.Width-IconoBotonCinta)/2, (t.Height-IconoBotonCinta)/2, IconoBotonCinta)
	r.barrita.Resize(fyne.NewSize(AnchoBarrita, AltoBarrita))
	r.barrita.Move(fyne.NewPos((t.Width-AnchoBarrita)/2, t.Height-AbajoBarrita-AltoBarrita))
}

func (r *muestraCintaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := color.NRGBA{}, tema.Color("text-muted", v)
	if r.m.Encima() {
		fondo, tinta = tema.Color("bg-subtle", v), tema.Color("text", v)
	}
	// 'transparent' se pinta neutra para que la barrita no desaparezca.
	barra, ok := Hex(r.m.Color)
	if !ok {
		barra = tema.Color("border", v)
	}
	if r.m.Inerte {
		fondo, tinta, barra = atenuar40(fondo), atenuar40(tinta), atenuar40(barra)
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.glifo.TenirCon(tinta)
	r.barrita.FillColor = barra
	r.barrita.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.m.Enfocado())
	r.Layout(r.m.Size())
}

// rejillaPaleta es el `grid grid-cols-6 gap-1` de la paleta.
type rejillaPaleta struct {
	widget.BaseWidget
	celdas []fyne.CanvasObject
}

// MedidaPaleta es lo que ocupa la rejilla de `n` muestras.
func MedidaPaleta(n int) fyne.Size {
	cols := min(n, ColumnasPaleta)
	filas := (n + ColumnasPaleta - 1) / ColumnasPaleta
	lado := func(k int) float32 { return float32(k)*LadoMuestra + float32(max(k-1, 0))*HuecoPaleta }
	return fyne.NewSize(lado(cols), lado(filas))
}

func (g *rejillaPaleta) MinSize() fyne.Size {
	g.ExtendBaseWidget(g)
	return MedidaPaleta(len(g.celdas))
}

func (g *rejillaPaleta) CreateRenderer() fyne.WidgetRenderer { return &rejillaRenderer{g: g} }

type rejillaRenderer struct{ g *rejillaPaleta }

func (r *rejillaRenderer) MinSize() fyne.Size           { return r.g.MinSize() }
func (r *rejillaRenderer) Objects() []fyne.CanvasObject { return r.g.celdas }
func (r *rejillaRenderer) Destroy()                     {}
func (r *rejillaRenderer) Refresh()                     {}
func (r *rejillaRenderer) Layout(fyne.Size) {
	for i, c := range r.g.celdas {
		col, fila := i%ColumnasPaleta, i/ColumnasPaleta
		c.Move(fyne.NewPos(float32(col)*(LadoMuestra+HuecoPaleta), float32(fila)*(LadoMuestra+HuecoPaleta)))
		c.Resize(fyne.NewSize(LadoMuestra, LadoMuestra))
	}
}

// celdaPaleta es una muestra de la paleta: el color con su borde y, si es el actual, el
// tic encima; 'transparent' lleva la diagonal roja de «sin color».
type celdaPaleta struct {
	activable
	hex    string
	actual bool
}

func nuevaCeldaPaleta(hex, nombre string, actual bool, alPulsar func()) *celdaPaleta {
	c := &celdaPaleta{hex: hex, actual: actual}
	c.Rotulo, c.AlPulsar = nombre, alPulsar
	c.ExtendBaseWidget(c)
	return c
}

func (c *celdaPaleta) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(LadoMuestra, LadoMuestra)
}

func (c *celdaPaleta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, hay := Hex(c.hex)
	r := &celdaRenderer{
		c:    c,
		aro:  ui.NuevoAro(3, ui.SinHueco, v),
		caja: ui.NuevaCaja(relleno, tema.Color("border", v), 3),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	if !hay {
		// `linear-gradient(to top right, transparent 44%, danger 44% 56%, transparent
		// 56%)` sobre `inset-0.5`: una banda de un 12 % del recorrido, de esquina a
		// esquina. Una línea de ese grosor es la misma banda.
		r.diagonal = canvas.NewLine(tema.Color("danger", v))
		r.objetos = append(r.objetos, r.diagonal)
	}
	if c.actual {
		// ponytail: la web lo pinta blanco con `mix-blend-difference`, que se ve
		// oscuro sobre los claros y claro sobre los oscuros. Aquí se elige el blanco o
		// la tinta por la luminancia del color: el mismo contraste sin fundido.
		tinta := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		if luz(relleno) > 0.5 || !hay {
			tinta = color.NRGBA{A: 255}
		}
		r.tic = iconos.TrazoEdConTinta(iconos.EdTic, 14, tinta)
		r.objetos = append(r.objetos, r.tic.Objeto())
	}
	r.Refresh()
	return r
}

// luz es la luminancia relativa aproximada (0..1) de un color sRGB.
func luz(c color.NRGBA) float32 {
	return (0.2126*float32(c.R) + 0.7152*float32(c.G) + 0.0722*float32(c.B)) / 255
}

type celdaRenderer struct {
	c        *celdaPaleta
	aro      *ui.Aro
	caja     *ui.Caja
	diagonal *canvas.Line
	tic      *iconos.Dibujo
	objetos  []fyne.CanvasObject
}

func (r *celdaRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *celdaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *celdaRenderer) Destroy()                     {}

func (r *celdaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	if r.diagonal != nil {
		d := tema.Borde + tema.Espacio(0.5)
		r.diagonal.Position1 = fyne.NewPos(d, t.Height-d)
		r.diagonal.Position2 = fyne.NewPos(t.Width-d, d)
		lado := t.Width - 2*d
		r.diagonal.StrokeWidth = 0.12 * lado * 1.41421356
	}
	if r.tic != nil {
		r.tic.Colocar((t.Width-14)/2, (t.Height-14)/2, 14)
	}
}

func (r *celdaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, _ := Hex(r.c.hex)
	r.caja.Pintar(relleno, tema.Color("border", v))
	r.aro.Pintar(v)
	r.aro.Ver(r.c.Enfocado())
	r.Layout(r.c.Size())
}

// ---- Divisor ----

// Divisor es el `bg-border mx-1 h-5 w-px` que separa grupos de controles en una barra
// de UNA línea —la mini-barra del escenario de Diapos—. No es SeparadorGrupos: aquel
// mide 28 y va colgado de la fila de la cinta; este mide 17,5 y va centrado.
func Divisor() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &divisor{linea: canvas.NewRectangle(tema.Color("border", v))}
}

// MedidaDivisor es el ancho de la línea con su `mx-1` y el alto de su `h-5`.
func MedidaDivisor() fyne.Size {
	return fyne.NewSize(tema.Borde+2*tema.Espacio(1), tema.Espacio(5))
}

type divisor struct {
	widget.BaseWidget
	linea *canvas.Rectangle
}

func (d *divisor) MinSize() fyne.Size {
	d.ExtendBaseWidget(d)
	return MedidaDivisor()
}

func (d *divisor) CreateRenderer() fyne.WidgetRenderer {
	d.ExtendBaseWidget(d)
	return &divisorRenderer{d: d}
}

type divisorRenderer struct{ d *divisor }

func (r *divisorRenderer) MinSize() fyne.Size           { return MedidaDivisor() }
func (r *divisorRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.d.linea} }
func (r *divisorRenderer) Destroy()                     {}
func (r *divisorRenderer) Layout(t fyne.Size) {
	r.d.linea.Move(fyne.NewPos(tema.Espacio(1), (t.Height-tema.Espacio(5))/2))
	r.d.linea.Resize(fyne.NewSize(tema.Borde, tema.Espacio(5)))
}

func (r *divisorRenderer) Refresh() {
	r.d.linea.FillColor = tema.Color("border", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.d.linea.Refresh()
}
