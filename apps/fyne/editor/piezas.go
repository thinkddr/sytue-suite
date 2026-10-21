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
)

// activable es la cáscara de los controles de esta cinta: `ui.Pulsable` —puntero,
// pulsación, teclado y rol de botón— con el `Rotulo` que estas piezas han llamado
// siempre así y que leen de fuera Calcus, Diapos y CertiPDF.
//
// Hasta el 2026-09-23 era una copia entera de la cáscara, escrita cuando la de `ui` era
// privada y con la condición de borrarla en cuanto se exportase. Se exportó y la copia
// siguió aquí, con un borde distinto: dejaba enfocar un control inerte, que en la web no
// recibe el foco.
type activable struct {
	ui.Pulsable

	Rotulo string // el nombre accesible, YA traducido
}

// AccessibilityLabel es el rótulo, leído cada vez: cambia con el idioma.
func (a *activable) AccessibilityLabel() string { return a.Rotulo }

// ---- BotonCinta ----

// MedidaBotonCinta es el `h-7 w-7` de RibbonButton: 28 × 28, y 44 × 44 con el dedo, que es
// lo que el `data-suelo-tactil` del editor web hace con todo lo que hay en la cinta. Sin
// el suelo, en un teléfono los treinta botones de Datos medían 28 (medido el 2026-10-16).
func MedidaBotonCinta() fyne.Size {
	lado := ui.SueloTactil(LadoBotonCinta)
	return fyne.NewSize(lado, lado)
}

// BotonCinta es el botón-icono de la cinta: cuadrado, con el glifo a 18 en
// `text-muted`, fondo `bg-subtle` al pasar por encima y `selected` cuando está pulsado
// (`aria-pressed`).
type BotonCinta struct {
	activable

	Icono iconos.Ed
	// Activo es `aria-pressed`: negrita encendida, alineación actual, panel abierto.
	Activo bool
	// Glifo sustituye al icono del catálogo `Ed` cuando el dibujo es de una app: la barra
	// de formato de Sytue Mail usa `BOTON_DE_BARRA` con SUS glifos a 16 (`LadoGlifo`).
	Glifo     *iconos.Dibujo
	LadoGlifo float32
}

var _ fyne.Widget = (*BotonCinta)(nil)

// NuevoBotonCinta crea el botón. `rotulo` es el nombre accesible ya traducido.
func NuevoBotonCinta(i iconos.Ed, rotulo string, alPulsar func()) *BotonCinta {
	b := &BotonCinta{Icono: i}
	b.Rotulo, b.AlPulsar = rotulo, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MinSize es la medida declarada.
func (b *BotonCinta) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return MedidaBotonCinta()
}

// CreateRenderer compone el aro de foco, la caja y el glifo.
func (b *BotonCinta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, trazo := b.colores(v)
	r := &botonCintaRenderer{
		b:     b,
		aro:   ui.NuevoAro(tema.RadioSM, ui.SinHueco, v).Abierto(),
		caja:  ui.NuevaCaja(fondo, color.NRGBA{}, tema.RadioSM),
		glifo: b.Glifo,
		lado:  IconoBotonCinta,
	}
	if r.glifo == nil {
		r.glifo = iconos.TrazoEdConTinta(b.Icono, IconoBotonCinta, trazo)
	} else if b.LadoGlifo > 0 {
		r.lado = b.LadoGlifo
	}
	r.glifo.TenirCon(trazo)
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.glifo.Objeto())
	return r
}

// colores son las tres reglas de `BOTON_DE_BARRA`: reposo sin fondo y tinta apagada,
// `hover:bg-bg-subtle hover:text-text`, `aria-pressed:bg-selected aria-pressed:text-text`
// y `disabled:opacity-40`.
func (b *BotonCinta) colores(v tema.Variante) (fondo, trazo color.NRGBA) {
	fondo, trazo = color.NRGBA{}, tema.Color("text-muted", v)
	switch {
	case b.Activo:
		fondo, trazo = tema.Color("selected", v), tema.Color("text", v)
	case b.Encima():
		fondo, trazo = tema.Color("bg-subtle", v), tema.Color("text", v)
	}
	if b.Inerte {
		fondo, trazo = atenuar40(fondo), atenuar40(trazo)
	}
	return fondo, trazo
}

// atenuar40 es el `disabled:opacity-40` de la receta. Es 0,4 y no el 0,5 del resto de
// la suite porque el botón de la cinta lo declara así.
func atenuar40(c color.NRGBA) color.NRGBA {
	c.A = uint8(float32(c.A) * 0.4)
	return c
}

type botonCintaRenderer struct {
	b       *BotonCinta
	aro     *ui.Aro
	caja    *ui.Caja
	glifo   *iconos.Dibujo
	lado    float32
	objetos []fyne.CanvasObject
}

func (r *botonCintaRenderer) MinSize() fyne.Size           { return MedidaBotonCinta() }
func (r *botonCintaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonCintaRenderer) Destroy()                     {}

func (r *botonCintaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	r.glifo.Colocar((t.Width-r.lado)/2, (t.Height-r.lado)/2, r.lado)
}

func (r *botonCintaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, trazo := r.b.colores(v)
	r.caja.Pintar(fondo, color.NRGBA{})
	r.glifo.TenirCon(trazo)
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado())
}

// ---- PestanaCinta ----

// MedidaPestanaCinta es lo que ocupa una pestaña de la tira: el rótulo en `text-sm`
// con `px-3` a cada lado, y de alto la tira entera menos su línea de un píxel.
//
// Medido: «Inicio» 49 × 31, «Disposición» 81 × 31, «Vista» 47 × 31. Con el dedo la tira
// mide 44 y la pestaña, 43 de alto y 44 de ancho como poco.
func MedidaPestanaCinta(rotulo string, activa bool) fyne.Size {
	p := ui.Normal
	if activa {
		p = ui.Medio // `font-medium` en la activa; medido, peso 500
	}
	return fyne.NewSize(
		ui.SueloTactil(ui.Medir(rotulo, tema.TextoSM, p).Width+2*MargenPestana),
		AltoDeTira()-tema.Borde,
	)
}

// PestanaCinta es una pestaña de la tira de la cinta.
//
// No es `ui.Pestana`, y no por descuido: aquella es `Tabs.tsx` del sistema de diseño
// —versalitas, 35,25 de alto, subrayado de dos píxeles de cobre y una línea que recorre
// la tira—. Esta mide 31, no va en versalitas y su indicador lleva `inset-x-1` y
// esquinas redondeadas. Son dos componentes distintos en la web y aquí también.
type PestanaCinta struct {
	activable

	Activa bool
	// AlMover recibe −1 o +1 con las flechas y 0 o 1 con Inicio y Fin (patrón
	// `tablist` de la WAI), igual que `ui.Pestana`.
	AlMover func(delta int, extremo bool)
}

var _ fyne.Widget = (*PestanaCinta)(nil)

// NuevaPestanaCinta crea la pestaña con su rótulo YA traducido.
func NuevaPestanaCinta(rotulo string, alPulsar func()) *PestanaCinta {
	p := &PestanaCinta{}
	p.Rotulo, p.AlPulsar = rotulo, alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// AccessibilityRole la anuncia como botón.
//
// Fyne v2.8.1 no tiene `AccessibleRoleTab`: sus roles son los de `fyne.Accessible` y
// ahí no está el `role="tab"` de la WAI. El botón es lo más cercano y lo que hace el
// resto de la suite (`ui.Pestana`); lo que SÍ se conserva es el comportamiento —las
// flechas, Inicio y Fin—, que es lo que un lector de pantalla necesita para moverse.
func (p *PestanaCinta) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// TypedKey añade las flechas y los extremos al Intro de la cáscara.
func (p *PestanaCinta) TypedKey(e *fyne.KeyEvent) {
	if p.AlMover != nil && e != nil {
		switch e.Name {
		case fyne.KeyRight:
			p.AlMover(1, false)
			return
		case fyne.KeyLeft:
			p.AlMover(-1, false)
			return
		case fyne.KeyHome:
			p.AlMover(0, true)
			return
		case fyne.KeyEnd:
			p.AlMover(1, true)
			return
		default: // el resto las resuelve la cáscara: Intro y Espacio activan
		}
	}
	p.activable.TypedKey(e)
}

// MinSize es la medida declarada.
func (p *PestanaCinta) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return MedidaPestanaCinta(p.Rotulo, p.Activa)
}

// CreateRenderer compone el rótulo y el indicador.
func (p *PestanaCinta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pestanaCintaRenderer{
		p:         p,
		aro:       ui.NuevoAro(0, ui.SinHueco, v).Abierto(),
		indicador: canvas.NewRectangle(color.NRGBA{}),
		rotulo:    ui.Texto(p.Rotulo, tema.TextoSM, p.peso(), p.tinta(v)),
	}
	r.indicador.CornerRadius = GrosorIndicador / 2 // `rounded-full` sobre 1,75 de alto
	r.objetos = append(r.aro.Objetos(), r.indicador, r.rotulo)
	return r
}

func (p *PestanaCinta) peso() ui.Peso {
	if p.Activa {
		return ui.Medio
	}
	return ui.Normal
}

// tinta es `text-text font-medium` en la activa y `text-text-muted hover:text-text` en
// las demás. Medido: `--color-accent` en la activa y en la que tiene el puntero encima,
// `--color-text-muted` en el resto.
func (p *PestanaCinta) tinta(v tema.Variante) color.NRGBA {
	if p.Activa || p.Encima() {
		return tema.Color("text", v)
	}
	return tema.Color("text-muted", v)
}

type pestanaCintaRenderer struct {
	p         *PestanaCinta
	aro       *ui.Aro
	indicador *canvas.Rectangle
	rotulo    *canvas.Text
	objetos   []fyne.CanvasObject
}

func (r *pestanaCintaRenderer) MinSize() fyne.Size {
	return MedidaPestanaCinta(r.p.Rotulo, r.p.Activa)
}
func (r *pestanaCintaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pestanaCintaRenderer) Destroy()                     {}

func (r *pestanaCintaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	// El rótulo va centrado en los dos ejes: la pestaña es `h-full` y su contenido se
	// centra, que es lo que la tira hace con `items-end` sobre una caja de altura fija.
	caja := ui.Medir(r.p.Rotulo, tema.TextoSM, r.p.peso())
	ui.ColocarTexto(r.rotulo, tema.TextoSM, (t.Width-caja.Width)/2, (t.Height-tema.TextoSM.Linea)/2)
	// `absolute inset-x-1 bottom-0 h-0.5`.
	r.indicador.Move(fyne.NewPos(MargenIndicador, t.Height-GrosorIndicador))
	r.indicador.Resize(fyne.NewSize(max(t.Width-2*MargenIndicador, 0), GrosorIndicador))
}

func (r *pestanaCintaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.rotulo.Text = r.p.Rotulo
	r.rotulo.TextSize = tema.TextoSM.Tam
	r.rotulo.Color = r.p.tinta(v)
	r.rotulo.Refresh()
	if r.p.Activa {
		r.indicador.FillColor = tema.Color("indicator", v)
	} else {
		r.indicador.FillColor = color.NRGBA{}
	}
	r.indicador.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.p.Enfocado())
}

// ---- Grupo ----

// Grupo es un grupo de la banda de la cinta: una fila de controles y, debajo, su
// rótulo en `text-2xs`.
//
// Es un contenedor y no un widget porque no se pulsa, no se enfoca y no se anuncia
// —`role="group"` con `aria-label`, que en Fyne no tiene equivalente—; la regla del
// paquete `ui` es elegir por comportamiento, y este no tiene ninguno.
type Grupo struct {
	widget.BaseWidget

	// Rotulo va debajo, YA traducido.
	Rotulo    string
	Controles []fyne.CanvasObject
	// AlPrincipio es `max-md:items-start max-md:justify-start`: por debajo de `md` la
	// fila, sus líneas y el rótulo van a la izquierda en vez de centrados. Lo pone quien
	// coloca los grupos, que es quien sabe cuánto mide la ventana.
	//
	// Y por debajo de `md` el rótulo NO se pinta (`max-md:hidden`) salvo con la banda
	// Desplegada: son 13 px de una banda de 53, y cada control lleva su nombre. El grupo
	// lo sigue diciendo a quien no ve (AccessibilityLabel).
	AlPrincipio bool
	// Desplegado es el grupo en la banda desplegada con «Más»: rótulo a la vista y una
	// fila que, si no cabe en el ancho que le dan, reparte sus controles en líneas.
	Desplegado bool
}

// conRotulo dice si el rótulo se pinta: siempre en escritorio y, por debajo de `md`,
// solo con la banda desplegada.
func (g *Grupo) conRotulo() bool { return !g.AlPrincipio || g.Desplegado }

var _ fyne.Widget = (*Grupo)(nil)

// HuecoLineasGrupo es el `gap-y-1` entre las líneas de controles cuando la fila envuelve.
var HuecoLineasGrupo = tema.Espacio(1)

// lineasGrupo reparte los controles en líneas que caben en `interior`, que es lo que hace
// `flex-wrap` con `gap-0.5`: un control que no cabe baja a la línea siguiente.
func lineasGrupo(ctl []fyne.CanvasObject, interior float32) [][]fyne.CanvasObject {
	var lineas [][]fyne.CanvasObject
	var ancho float32
	for _, c := range ctl {
		w := c.MinSize().Width
		if n := len(lineas); n > 0 && ancho+HuecoControl+w <= interior+0.01 {
			lineas[n-1] = append(lineas[n-1], c)
			ancho += HuecoControl + w
			continue
		}
		lineas = append(lineas, []fyne.CanvasObject{c})
		ancho = w
	}
	return lineas
}

// altoLinea es lo que mide una línea de controles: el más alto.
func altoLinea(l []fyne.CanvasObject) float32 {
	var h float32
	for _, c := range l {
		h = max(h, c.MinSize().Height)
	}
	return h
}

// AltoFilaGrupoEn es el alto de la fila de controles de un grupo que mide `ancho`: su
// `min-h-11`, o lo que sumen sus líneas con el `gap-y-1` si envuelve.
func AltoFilaGrupoEn(g *Grupo, ancho float32) float32 {
	lineas := lineasGrupo(g.Controles, ancho-2*MargenGrupo)
	var h float32
	for i, l := range lineas {
		if i > 0 {
			h += HuecoLineasGrupo
		}
		h += altoLinea(l)
	}
	return max(AltoFilaGrupo, h)
}

// AltoGrupoEn es el alto natural de un grupo que mide `ancho`: `pt-1.5`, la fila, el
// `gap-0.5`, el rótulo con `leading-none` y el `pb-1`; sin rótulo, `py-1` y la fila. En la
// banda del editor el grupo llena un alto fijo; en la cinta de CertiPDF, la banda mide lo
// que su grupo más alto.
func AltoGrupoEn(g *Grupo, ancho float32) float32 {
	if !g.conRotulo() {
		return 2*ArribaGrupoEstrecho + AltoFilaGrupoEn(g, ancho)
	}
	return ArribaGrupo + AltoFilaGrupoEn(g, ancho) + HuecoControl + AltoRotuloGrupo + AbajoGrupo
}

// NuevoGrupo crea el grupo con su rótulo traducido y sus controles.
func NuevoGrupo(rotulo string, controles ...fyne.CanvasObject) *Grupo {
	g := &Grupo{Rotulo: rotulo, Controles: controles}
	g.ExtendBaseWidget(g)
	return g
}

// AccessibilityRole lo anuncia como un contenedor con nombre.
func (g *Grupo) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AccessibilityLabel es el `aria-label` del `role="group"` de la web.
func (g *Grupo) AccessibilityLabel() string { return g.Rotulo }

// MedidaGrupo es lo que ocupa: el ancho de la fila de controles o del rótulo, el que
// sea mayor, con `px-1.5`; y de alto, la banda entera menos su línea. Sin rótulo, la fila
// sola y la banda estrecha.
func MedidaGrupo(g *Grupo) fyne.Size {
	var fila float32
	for i, c := range g.Controles {
		if i > 0 {
			fila += HuecoControl
		}
		fila += c.MinSize().Width
	}
	if !g.conRotulo() {
		return fyne.NewSize(fila+2*MargenGrupo, AltoBandaEstrecha-tema.Borde)
	}
	rot := ui.Medir(g.Rotulo, tema.Texto2XS, ui.Normal).Width
	return fyne.NewSize(max(fila, rot)+2*MargenGrupo, AltoBanda-tema.Borde)
}

// MinSize es la medida declarada.
func (g *Grupo) MinSize() fyne.Size {
	g.ExtendBaseWidget(g)
	return MedidaGrupo(g)
}

// CreateRenderer compone los controles y el rótulo.
func (g *Grupo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &grupoRenderer{
		g:      g,
		rotulo: ui.Texto(g.Rotulo, tema.Texto2XS, ui.Normal, tema.Color("neutral-500", v)),
	}
	r.objetos = append(append([]fyne.CanvasObject{}, g.Controles...), r.rotulo)
	return r
}

type grupoRenderer struct {
	g       *Grupo
	rotulo  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *grupoRenderer) MinSize() fyne.Size           { return MedidaGrupo(r.g) }
func (r *grupoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *grupoRenderer) Destroy()                     {}

func (r *grupoRenderer) Layout(t fyne.Size) {
	// `pt-1.5` y la fila de `min-h-11`: cada control va centrado en ella. Medido: con
	// la banda en y=136 y su `py-1`, la fila sale en 144,75 y un botón de 24,5 en
	// 151,75 — que es lo que devuelve el DOM.
	//
	// Y en horizontal, `items-center`: la fila y el rótulo van CENTRADOS en el grupo.
	// Medido en la cinta de CertiPDF, que es este mismo `RibbonGroup`: «Herramientas»
	// cae en x=53,31 de un grupo de 157,5 que empieza en 3,5, o sea centrado bajo sus
	// cinco botones, y no pegado al `px-1.5`. Cuando la fila es lo más ancho, centrarla
	// es dejarla donde estaba.
	// Por debajo de `md` van al principio (`max-md:items-start`), y eso lo dice
	// AlPrincipio, porque el grupo no sabe cuánto mide la ventana.
	//
	// Con menos ancho que el natural, la fila ENVUELVE (`flex-wrap`, `gap-y-1`), y el
	// alto que sobra en su `min-h-11` se reparte entre las líneas, que es el
	// `align-content: normal` de una caja flexible que envuelve. Medido en la cinta de
	// CertiPDF a 360: cuatro botones y el quinto debajo, 31,5 más abajo.
	interior := t.Width - 2*MargenGrupo
	lineas := lineasGrupo(r.g.Controles, interior)
	var suma float32
	for i, l := range lineas {
		if i > 0 {
			suma += HuecoLineasGrupo
		}
		suma += altoLinea(l)
	}
	fila := max(AltoFilaGrupo, suma)
	extra := (fila - suma) / float32(max(len(lineas), 1))
	y := ArribaGrupo
	if !r.g.conRotulo() {
		y = ArribaGrupoEstrecho
	}
	for _, l := range lineas {
		alto := altoLinea(l) + extra
		var ancho float32
		for i, c := range l {
			if i > 0 {
				ancho += HuecoControl
			}
			ancho += c.MinSize().Width
		}
		x := MargenGrupo
		if !r.g.AlPrincipio {
			x += (interior - ancho) / 2
		}
		for _, c := range l {
			m := c.MinSize()
			c.Move(fyne.NewPos(x, y+(alto-m.Height)/2))
			c.Resize(m)
			x += m.Width + HuecoControl
		}
		y += alto + HuecoLineasGrupo
	}
	// `mt-auto` + `pb-1`: el rótulo se pega abajo, con `leading-none`.
	r.rotulo.Hidden = !r.g.conRotulo()
	x := MargenGrupo
	if !r.g.AlPrincipio {
		x = (t.Width - ui.Medir(r.g.Rotulo, tema.Texto2XS, ui.Normal).Width) / 2
	}
	ui.ColocarTexto(r.rotulo, tema.Escalon{Tam: tema.Texto2XS.Tam, Linea: AltoRotuloGrupo},
		x, t.Height-AbajoGrupo-AltoRotuloGrupo)
}

func (r *grupoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.rotulo.Text = r.g.Rotulo
	r.rotulo.Color = tema.Color("neutral-500", v)
	r.rotulo.Refresh()
}

// SeparadorGrupos es el `w-px h-8 mx-1.5 mt-3 self-start` entre grupos.
//
// Va CENTRADO EN LA FILA DE CONTROLES y no en la banda entera: centrado en los 68 px
// se metía en la línea del rótulo (el comentario de `primitives.tsx` lo cuenta con sus
// números). Aquí eso está resuelto por construcción — el `mt-3` es desde arriba de la
// banda, no desde el centro.
func SeparadorGrupos() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	linea := canvas.NewRectangle(tema.Color("border", v))
	return &separador{linea: linea}
}

type separador struct {
	widget.BaseWidget
	linea *canvas.Rectangle
	// estrecho es `max-md:mt-2.5`: baja con la fila del grupo estrecho. Lo pone la cinta,
	// que es quien sabe cuánto mide la ventana, igual que el AlPrincipio del grupo.
	estrecho bool
}

var _ fyne.Widget = (*separador)(nil)

func (s *separador) MinSize() fyne.Size {
	s.ExtendBaseWidget(s)
	return fyne.NewSize(tema.Borde+2*MargenSeparador, AltoBanda-tema.Borde)
}

func (s *separador) CreateRenderer() fyne.WidgetRenderer {
	s.ExtendBaseWidget(s)
	return &separadorRenderer{s: s}
}

type separadorRenderer struct{ s *separador }

func (r *separadorRenderer) MinSize() fyne.Size           { return r.s.MinSize() }
func (r *separadorRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.s.linea} }
func (r *separadorRenderer) Destroy()                     {}
func (r *separadorRenderer) Layout(fyne.Size) {
	arriba := ArribaSeparador
	if r.s.estrecho {
		arriba = ArribaSeparadorEstrecho
	}
	r.s.linea.Move(fyne.NewPos(MargenSeparador, arriba))
	r.s.linea.Resize(fyne.NewSize(tema.Borde, AltoSeparadorGrupos))
}

func (r *separadorRenderer) Refresh() {
	r.s.linea.FillColor = tema.Color("border", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.s.linea.Refresh()
}

// ascenso es el `ascender` de Figtree (950 sobre 1000): de lo alto de la caja de la
// fuente a la línea base. Hace falta para poner un rótulo donde lo pondría un `<text>`
// de SVG, que se ancla por la línea base.
const ascenso = 0.95

// RotuloEnBase coloca un rótulo del SVG, escalado por `k`, con su línea base donde la
// pone el SVG y el anclaje horizontal que declara.
func RotuloEnBase(r figuras.Rotulo, pos fyne.Position, k float32, tinta color.NRGBA, peso ui.Peso) *canvas.Text {
	tam := float32(r.Tam) * k
	e := tema.Escalon{Tam: tam, Linea: tam * 1.2}
	t := ui.Texto(r.Texto, e, peso, tinta)
	x := pos.X + float32(r.X)*k
	switch r.Ancla {
	case figuras.AnclaMedio:
		x -= ui.Medir(r.Texto, e, peso).Width / 2
	case figuras.AnclaFin:
		x -= ui.Medir(r.Texto, e, peso).Width
	case figuras.AnclaInicio:
	}
	t.Move(fyne.NewPos(x, pos.Y+float32(r.Y)*k-ascenso*tam))
	t.Resize(t.MinSize())
	return t
}

// FormasUI son SLIDE_SHAPES (y DOC_SHAPES, que es el mismo juego): la forma, su rótulo y su icono (el id es el nombre del
// icono en la web), en el orden de «Insertar».
var FormasUI = []struct {
	ID, Clave string
	Icono     iconos.Ed
}{
	{"rect", "slides.shape.rect", iconos.EdRect},
	{"ellipse", "slides.shape.ellipse", iconos.EdElipse},
	{"line", "slides.shape.line", iconos.EdLinea},
	{"arrow", "slides.shape.arrow", iconos.EdFlecha},
	{"triangle", "slides.shape.triangle", iconos.EdTriangulo},
	{"diamond", "slides.shape.diamond", iconos.EdRombo},
	{"pentagon", "slides.shape.pentagon", iconos.EdPentagono},
	{"hexagon", "slides.shape.hexagon", iconos.EdHexagono},
	{"star", "slides.shape.star", iconos.EdEstrella},
	{"cylinder", "slides.shape.cylinder", iconos.EdCilindro},
	{"callout", "slides.shape.callout", iconos.EdBocadillo},
}

// TextoNodoSmart es el `foreignObject` de un nodo: `flex items-center`, el relleno del nodo,
// `font-weight: 500`, `line-height: 1.2` y `overflow: hidden`.
func TextoNodoSmart(nd figuras.NodoSmart, texto string, vineta bool, pos fyne.Position, k float32, tinta color.NRGBA) []fyne.CanvasObject {
	fs := float32(figuras.LetraNodo(nd)) * k
	e := tema.Escalon{Tam: fs, Linea: fs * 1.2}
	py, px := figuras.RellenoNodo(nd)
	x0, y0 := pos.X+float32(nd.X)*k, pos.Y+float32(nd.Y)*k
	w, h := float32(nd.W)*k, float32(nd.H)*k
	izq, cabe := x0+float32(px)*k, w-2*float32(px)*k
	var fuera []fyne.CanvasObject
	if vineta {
		// La viñeta va delante con `opacity: 0.55` y `gap: 0.4em`.
		b := tinta
		b.A = uint8(float32(b.A) * 0.55)
		punto := ui.Texto("•", e, ui.Medio, b)
		anchoPunto := ui.Medir("•", e, ui.Medio).Width + 0.4*fs
		ui.ColocarTexto(punto, e, izq, y0+(h-e.Linea)/2)
		fuera = append(fuera, punto)
		izq, cabe = izq+anchoPunto, cabe-anchoPunto
	}
	lineas := ui.Lineas(texto, e, ui.Medio, cabe)
	alto := h - 2*float32(py)*k
	if caben := int(alto / e.Linea); len(lineas) > max(caben, 1) {
		lineas = lineas[:max(caben, 1)]
	}
	y := y0 + (h-float32(len(lineas))*e.Linea)/2
	for _, ln := range lineas {
		x := izq
		if !vineta {
			x += (cabe - ui.Medir(ln, e, ui.Medio).Width) / 2
		}
		t := ui.Texto(ln, e, ui.Medio, tinta)
		ui.ColocarTexto(t, e, x, y)
		fuera = append(fuera, t)
		y += e.Linea
	}
	return fuera
}

// DisposicionesSmartUI son SMARTART_LAYOUTS, en su orden: las de la mini-barra y las de
// «Insertar», en las diapositivas y en los documentos.
var DisposicionesSmartUI = []struct {
	ID, Clave string
	Icono     iconos.Ed
}{
	{"list", "smartart.list", iconos.EdSmartLista},
	{"process", "smartart.process", iconos.EdSmartProceso},
	{"cycle", "smartart.cycle", iconos.EdSmartCiclo},
	{"hierarchy", "smartart.hierarchy", iconos.EdSmartJerarquia},
}
