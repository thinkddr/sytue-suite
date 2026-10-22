// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La barra de control de la vista de Calendar (el `<header>` de CalendarApp.tsx).
//
// NO es `ui.CabeceraApp`: aquella es `SectionHeader` —icono, título y acciones a la
// derecha— y esta mete controles TAMBIÉN a la izquierda del título («Hoy», ‹, ›). Es
// bespoke en la web igual que aquí: no vive en `packages/ui`, sino dentro de la app.
// Lo que sí se comparte es la REGLA del alto, que es la misma:
//
//	max(min-h-12, contenido + 2 × py-2.5 + border-b)
//
// Con botones h-8=32, py-2.5=10 y borde de1 sale53, medido el2026-10-04
// en CalendarApp con raíz de16. Los48 del `min-h-12` viven en
// `ui.MinCabeceraApp` y se importan de allí para que no haya dos copias del número.
const (
	// AltoBotonCabecera es el `h-8` de los controles de esta barra.
	AltoBotonCabecera float32 = 8 * 0.25 * tema.Rem
	// LadoBotonIcono es el `h-8 w-8` de los botones de solo icono.
	LadoBotonIcono = AltoBotonCabecera
	// AltoGrupoVista es el `h-7` de cada botón del conmutador de vista, y
	// RellenoGrupoVista su `p-0.5`.
	AltoGrupoVista    float32 = 7 * 0.25 * tema.Rem
	RellenoGrupoVista float32 = 0.5 * 0.25 * tema.Rem
)

// transparente es «sin relleno»: una caja que no pinta fondo.
var transparente = color.NRGBA{}

var (
	cabeceraPX   = tema.Espacio(4)   // px-4
	cabeceraPY   = tema.Espacio(2.5) // py-2.5
	huecoX       = tema.Espacio(3)   // gap-x-3
	huecoBotones = tema.Espacio(1)   // gap-1 dentro de cada grupo
)

// AltoCabeceraVista es la medida declarada, y la única cuenta: el renderer la llama y
// el guardián compara contra ella y contra los píxeles pintados.
func AltoCabeceraVista() float32 {
	return max(ui.MinCabeceraApp, max(AltoBotonCabecera, escalonTitulo().Linea)+2*cabeceraPY+tema.Borde)
}

// escalonTitulo es el del `<h1 class="sytue-h3">` de la cabecera, con la escala de la
// forma (y su peso, `ui.PesoTitulo(3)`). MEDIDO en la web el 2026-10-05, raíz de 16:
// 22 / 30 en 600. Fue `Texto2XL` (28 / 36) un día, sin medir: el título salía seis
// píxeles más grande que en la web.
func escalonTitulo() tema.Escalon { return ui.EscalonTitulo(3) }

// CabeceraVista es la barra: Hoy ‹ › · título · buscar · invitaciones · vista.
type CabeceraVista struct {
	widget.BaseWidget

	// Titulo es `titleFor(mode, cursor, locale)`, ya compuesto por la pantalla.
	Titulo string
	// Izquierda son «Hoy» y las dos flechas; Derecha, lo que va tras el título.
	Izquierda []fyne.CanvasObject
	Derecha   []fyne.CanvasObject
}

var _ fyne.Widget = (*CabeceraVista)(nil)

// NuevaCabeceraVista arma la barra.
func NuevaCabeceraVista(titulo string, izquierda, derecha []fyne.CanvasObject) *CabeceraVista {
	c := &CabeceraVista{Titulo: titulo, Izquierda: izquierda, Derecha: derecha}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el `<h1>` de la vista.
func (c *CabeceraVista) AccessibilityLabel() string { return c.Titulo }

// AccessibilityRole: encabezado, no control.
func (c *CabeceraVista) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// CreateRenderer compone fondo, filete y título.
func (c *CabeceraVista) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &cabeceraVistaRenderer{
		c:      c,
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
		// `.sytue-h3`: `--text-xl` en semi-negrita, con la escala y el peso de la forma.
		titulo: ui.Texto(c.Titulo, escalonTitulo(), ui.PesoTitulo(3), tema.Color("text", v)),
	}
	r.objetos = []fyne.CanvasObject{r.fondo, r.filete, r.titulo}
	r.objetos = append(r.objetos, c.Izquierda...)
	r.objetos = append(r.objetos, c.Derecha...)
	return r
}

type cabeceraVistaRenderer struct {
	c             *CabeceraVista
	fondo, filete *canvas.Rectangle
	titulo        *canvas.Text
	objetos       []fyne.CanvasObject
	tira          *ui.TiraDeslizable
	tituloMovil   *ui.Rotulo
}

func (r *cabeceraVistaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.c.AltoPara(r.c.Size().Width))
}
func (r *cabeceraVistaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *cabeceraVistaRenderer) Destroy()                     {}

// huecoFilas es el `gap-y-2` entre las filas cuando la barra se parte.
var huecoFilas = tema.Espacio(2)

// pieza es un elemento `flex` de la barra: el grupo de la izquierda cuenta como uno, el
// título como otro y cada control de la derecha, suelto.
type pieza struct {
	objs         []fyne.CanvasObject // el grupo de la izquierda; nil para el título
	ancho        float32
	titulo, auto bool // `auto` es el `ml-auto` del primero de la derecha
}

// piezas son los elementos en su orden, con su ancho de contenido.
func (c *CabeceraVista) piezas() []pieza {
	var out []pieza
	if len(c.Izquierda) > 0 {
		p := pieza{objs: c.Izquierda}
		for i, o := range c.Izquierda {
			if i > 0 {
				p.ancho += huecoBotones
			}
			p.ancho += o.MinSize().Width
		}
		out = append(out, p)
	}
	out = append(out, pieza{titulo: true, ancho: ui.Medir(c.Titulo, escalonTitulo(), ui.PesoTitulo(3)).Width})
	for i, o := range c.Derecha {
		out = append(out, pieza{objs: []fyne.CanvasObject{o}, ancho: o.MinSize().Width, auto: i == 0})
	}
	return out
}

// filas es el `flex-wrap`: cada elemento va en la fila actual si cabe a su tamaño de
// contenido y, si no, abre una nueva. El título puede encoger (`min-w-0 truncate`), pero
// el reparto en filas se decide con su ancho entero, como hace el navegador.
func (c *CabeceraVista) filas(util float32) [][]pieza {
	piezas := c.piezas()
	var out [][]pieza
	fila := make([]pieza, 0, len(piezas))
	x := float32(0)
	for _, p := range piezas {
		if len(fila) > 0 && x+huecoX+p.ancho > util {
			out, fila, x = append(out, fila), nil, 0
		}
		if len(fila) > 0 {
			x += huecoX
		}
		fila = append(fila, p)
		x += p.ancho
	}
	if len(fila) > 0 {
		out = append(out, fila)
	}
	return out
}

// altoFila cuenta el tamaño real de los controles. En iOS los botones compartidos
// crecen hasta el objetivo táctil de 44: una tira fija de 32 los centraba en Y=-6 y
// recortaba también «Hoy», que comparte su grupo con las flechas.
func (c *CabeceraVista) altoFila() float32 {
	alto := max(AltoBotonCabecera, escalonTitulo().Linea)
	for _, grupo := range [][]fyne.CanvasObject{c.Izquierda, c.Derecha} {
		for _, o := range grupo {
			if o.Visible() {
				alto = max(alto, o.MinSize().Height)
			}
		}
	}
	return alto
}

// AltoPara es lo que mide la barra con ese ancho: `max(min-h-12, filas + py-2.5 +
// border-b)`. CalendarApp conserva una fila desplazable en móvil.
func (c *CabeceraVista) AltoPara(ancho float32) float32 {
	n := float32(1)
	if ancho >= ui.Estrecho {
		n = float32(len(c.filas(ancho - 2*cabeceraPX)))
	}
	alto := n*c.altoFila() + max(n-1, 0)*huecoFilas + 2*cabeceraPY + tema.Borde
	return max(ui.MinCabeceraApp, alto)
}

// Layout coloca cada fila centrada en su alto de 28; el `ml-auto` empuja al margen
// derecho el primer control de la derecha y todo lo que le sigue en su fila.
func (r *cabeceraVistaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	r.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	util := t.Width - 2*cabeceraPX
	if t.Width < ui.Estrecho {
		r.colocarMovil(util)
		return
	}
	r.colocarFilas(t, util)
}

func (r *cabeceraVistaRenderer) crearTiraMovil() {
	c := r.c
	r.tituloMovil = ui.NuevoRotulo(c.Titulo, escalonTitulo(), ui.PesoTitulo(3), "text")
	objetos := []fyne.CanvasObject{}
	if len(c.Izquierda) > 0 {
		objetos = append(objetos, container.New(ui.FilaAuto{Hueco: huecoBotones, Derecha: len(c.Izquierda)}, c.Izquierda...))
	}
	objetos = append(objetos, r.tituloMovil)
	objetos = append(objetos, c.Derecha...)
	r.tira = ui.NuevaTiraDeslizable(c.altoFila(), huecoX, objetos...)
	r.tira.AnchoPropio = true
	for _, objeto := range objetos {
		mostrar := func() {
			if r.c.Size().Width < ui.Estrecho {
				r.tira.Mostrar(objeto)
			}
		}
		switch o := objeto.(type) {
		case *ui.Boton:
			o.AlEnfocar = mostrar
		case *fyne.Container:
			for _, hijo := range o.Objects {
				switch b := hijo.(type) {
				case *ui.Boton:
					b.AlEnfocar = mostrar
				case *botonInvitaciones:
					b.alEnfocar = mostrar
				}
			}
		case *botonInvitaciones:
			o.alEnfocar = mostrar
		case *GrupoVista:
			o.AlEnfocar = mostrar
		}
	}
}

func (r *cabeceraVistaRenderer) colocarMovil(util float32) {
	if r.tira == nil {
		r.crearTiraMovil()
	}
	// FILA_UNICA_EN_MOVIL de CalendarApp.tsx: conservar una fila y alcanzar
	// todos los controles con la tira compartida, sin consumir más alto.
	r.tituloMovil.Texto = r.c.Titulo
	r.tituloMovil.Refresh()
	r.objetos = []fyne.CanvasObject{r.fondo, r.filete, r.tira}
	r.tira.Move(fyne.NewPos(cabeceraPX, cabeceraPY))
	r.tira.Lado = r.c.altoFila()
	r.tira.Resize(fyne.NewSize(max(util, 0), r.tira.Lado))
}

func (r *cabeceraVistaRenderer) colocarFilas(t fyne.Size, util float32) {
	c := r.c
	r.objetos = []fyne.CanvasObject{r.fondo, r.filete, r.titulo}
	r.objetos = append(r.objetos, c.Izquierda...)
	r.objetos = append(r.objetos, c.Derecha...)
	filas := c.filas(util)
	altoFila := c.altoFila()
	// Con una sola fila, la barra se centra en su alto (el `min-h-12` puede darle más).
	y := cabeceraPY
	if len(filas) == 1 {
		y = (t.Height - tema.Borde - altoFila) / 2
	}
	for _, fila := range filas {
		usado := float32(0)
		for i, p := range fila {
			if i > 0 {
				usado += huecoX
			}
			usado += p.ancho
		}
		libre := max(util-usado, 0)
		centro := y + altoFila/2
		x := cabeceraPX
		for i, p := range fila {
			if i > 0 {
				x += huecoX
			}
			if p.auto {
				x += libre
			}
			if p.titulo {
				// Lo que sobre de la fila, sin pasarse de su contenido.
				ancho := min(p.ancho, max(p.ancho-(usado-util), 0))
				ui.Truncar(r.titulo, c.Titulo, escalonTitulo(), ui.PesoTitulo(3), ancho)
				ui.ColocarTexto(r.titulo, escalonTitulo(), x, centro-escalonTitulo().Linea/2)
				x += ancho
				continue
			}
			for j, o := range p.objs {
				if j > 0 {
					x += huecoBotones
				}
				m := o.MinSize()
				o.Move(fyne.NewPos(x, centro-m.Height/2))
				o.Resize(m)
				x += m.Width
			}
		}
		y += altoFila + huecoFilas
	}
}

// Refresh repinta con los tokens del tema.
func (r *cabeceraVistaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.filete.FillColor = tema.Color("border", v)
	r.titulo.Color = tema.Color("text", v)
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- El conmutador de vista ----

// Vista es la vista activa: mes, semana, día o agenda.
type Vista uint8

// Las cuatro vistas, en el orden en que la web las pinta.
const (
	VistaMes Vista = iota
	VistaSemana
	VistaDia
	VistaAgenda
)

// GrupoVista es el `role="group"` de la cabecera: una píldora sobre `bg-subtle` con
// cuatro botones de `h-7`, y el activo sobre `surface` con su sombra.
//
// El activo va en `primary-text` y NO en `primary`: el relleno no se reasigna en
// oscuro y sobre `surface` oscuro daba 2,96:1. El rol de texto sí invierte. Es el
// comentario que la web lleva en esa misma clase, y por eso está aquí.
type GrupoVista struct {
	widget.BaseWidget

	// Rotulos son los cuatro textos, ya traducidos.
	Rotulos [4]string
	// Activa es la vista pintada como seleccionada.
	Activa Vista
	// AlElegir cambia de vista.
	AlElegir func(Vista)
	// AlEnfocar enseña el grupo cuando sus botones ganan foco en una tira móvil.
	AlEnfocar func()
}

var _ fyne.Widget = (*GrupoVista)(nil)

// NuevoGrupoVista crea el conmutador.
func NuevoGrupoVista(rotulos [4]string, activa Vista, alElegir func(Vista)) *GrupoVista {
	g := &GrupoVista{Rotulos: rotulos, Activa: activa, AlElegir: alElegir}
	g.ExtendBaseWidget(g)
	return g
}

// AccessibilityRole: agrupa cuatro botones.
func (g *GrupoVista) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MedidaGrupoVista es lo que ocupa: cuatro rótulos con su `px-2.5`, el `gap-0.5` entre
// ellos y el `p-0.5` de la píldora.
func MedidaGrupoVista(rotulos [4]string) fyne.Size {
	ancho := 2 * RellenoGrupoVista
	for i, r := range rotulos {
		if i > 0 {
			ancho += RellenoGrupoVista
		}
		ancho += ui.Medir(r, tema.TextoSM, ui.SemiNegrita).Width + 2*espacio2p5
	}
	return fyne.NewSize(ancho, AltoGrupoVista+2*RellenoGrupoVista)
}

// CreateRenderer construye la píldora y sus cuatro botones.
func (g *GrupoVista) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &grupoVistaRenderer{g: g, fondo: canvas.NewRectangle(tema.Color("bg-subtle", v))}
	r.fondo.CornerRadius = tema.RadioMD
	r.objetos = []fyne.CanvasObject{r.fondo}
	for i := range g.Rotulos {
		b := nuevoBotonVista(g, Vista(i))
		r.botones = append(r.botones, b)
		r.objetos = append(r.objetos, b)
	}
	return r
}

type grupoVistaRenderer struct {
	g       *GrupoVista
	fondo   *canvas.Rectangle
	botones []*botonVista
	objetos []fyne.CanvasObject
}

func (r *grupoVistaRenderer) MinSize() fyne.Size           { return MedidaGrupoVista(r.g.Rotulos) }
func (r *grupoVistaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *grupoVistaRenderer) Destroy()                     {}

func (r *grupoVistaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	x := RellenoGrupoVista
	for i, b := range r.botones {
		m := b.MinSize()
		b.Move(fyne.NewPos(x, RellenoGrupoVista))
		b.Resize(fyne.NewSize(m.Width, AltoGrupoVista))
		x += m.Width
		if i < len(r.botones)-1 {
			x += RellenoGrupoVista
		}
	}
}

func (r *grupoVistaRenderer) Refresh() {
	r.fondo.FillColor = tema.Color("bg-subtle", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// botonVista es uno de los cuatro. Es un widget aparte porque cada uno se enfoca y se
// anuncia con su `aria-pressed`.
//
// Puntero, pulsación, teclado y rol de botón son los de `ui.Pulsable`. Fyne 2.8.1 no
// sabe decir «pulsado», así que el `aria-pressed` de la web NO llega al lector: es uno
// de los cuatro roles del techo del toolkit y está en `docs/roadmap.md`.
type botonVista struct {
	ui.Pulsable
	g *GrupoVista
	v Vista
}

var _ fyne.Widget = (*botonVista)(nil)

func nuevoBotonVista(g *GrupoVista, v Vista) *botonVista {
	b := &botonVista{g: g, v: v}
	b.AlPulsar = func() {
		if g.AlElegir != nil {
			g.AlElegir(v)
		}
	}
	b.ExtendBaseWidget(b)
	return b
}

// AccessibilityLabel es el rótulo de la vista, leído cada vez: el grupo puede cambiarlos.
func (b *botonVista) AccessibilityLabel() string { return b.g.Rotulos[b.v] }

func (b *botonVista) FocusGained() {
	b.Pulsable.FocusGained()
	if b.g.AlEnfocar != nil {
		b.g.AlEnfocar()
	}
}

func (b *botonVista) MinSize() fyne.Size {
	ancho := ui.Medir(b.g.Rotulos[b.v], tema.TextoSM, ui.SemiNegrita).Width + 2*espacio2p5
	return fyne.NewSize(ancho, AltoGrupoVista)
}

func (b *botonVista) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonVistaRenderer{
		b:     b,
		caja:  ui.NuevaCaja(transparente, transparente, tema.RadioSM),
		texto: ui.Texto(b.g.Rotulos[b.v], tema.TextoSM, ui.SemiNegrita, tema.Color("text-muted", v)),
		aro:   ui.NuevoAro(tema.RadioSM, 0, v).Abierto(),
	}
	r.objetos = append(r.caja.Objetos(), r.texto)
	r.objetos = append(r.objetos, r.aro.Objetos()...)
	r.aro.Ver(false)
	r.Refresh() // regla 7 de `ui`: la vista activa nace ya marcada
	return r
}

type botonVistaRenderer struct {
	b       *botonVista
	caja    *ui.Caja
	texto   *canvas.Text
	aro     *ui.Aro
	objetos []fyne.CanvasObject
}

func (r *botonVistaRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonVistaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonVistaRenderer) Destroy()                     {}

func (r *botonVistaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	ancho := ui.Medir(r.texto.Text, tema.TextoSM, ui.SemiNegrita).Width
	ui.ColocarTexto(r.texto, tema.TextoSM, (t.Width-ancho)/2, (t.Height-tema.TextoSM.Linea)/2)
	r.aro.Colocar(t)
}

func (r *botonVistaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	switch {
	case r.b.g.Activa == r.b.v:
		r.caja.Pintar(tema.Color("surface", v), transparente)
		r.texto.Color = tema.Color("primary-text", v)
	case r.b.Encima():
		r.caja.Pintar(tema.Color("surface", v), transparente)
		r.texto.Color = tema.Color("text", v)
	default:
		r.caja.Pintar(transparente, transparente)
		r.texto.Color = tema.Color("text-muted", v)
	}
	r.texto.Refresh()
	r.aro.Ver(r.b.Enfocado())
}

// BotonIconoCabecera es el `HEADER_ICON_BTN` de CalendarApp.tsx:
//
//	grid h-8 w-8 place-items-center rounded-md text-text
//	hover:bg-surface-sunken active:bg-surface-sunken
//
// En la web no es `Button.tsx` sino una clase escrita a mano dentro de esta app, pero
// la GEOMETRÍA es exactamente la de un `<Button variant="ghost" size="sm" iconOnly>`
// —28×28, radio 6, sin relleno—, así que se importa del catálogo en vez de escribir
// aquí noventa líneas de widget que solo cambian en un token.
//
// ponytail: ese token es el HOVER, y solo se separa en OSCURO. La clase de la web usa
// `surface-sunken` (neutral-950 en oscuro) y el botón fantasma usa `bg-subtle`
// (neutral-900): en claro los dos son neutral-100 y no hay diferencia; en oscuro, al
// pasar el ratón, este botón queda un escalón más claro que la web. Se cierra el día
// que `ui.Boton` admita un token de hover propio, que es de la familia de los botones.
func BotonIconoCabecera(i iconos.Icono, nombre string, alPulsar func()) *ui.Boton {
	b := ui.NuevoBoton(nombre, alPulsar).Con(ui.Fantasma, ui.TallaSM)
	b.SoloIcono = true
	b.Icono = i
	b.TamIcono = TamGlifoCabecera
	return b
}

// TamGlifoCabecera es el `size` de los iconos de esta barra: las flechas van al 20 por
// defecto de Icon.tsx y la lupa a 18 (`<Icon name="search" size={18} />`).
const TamGlifoCabecera float32 = 20

// botonInvitaciones es el `<button>` de «Invitaciones» de la cabecera, escrito a mano en
// CalendarApp.tsx y no con `Button`: el contador de pendientes va DENTRO del botón, tras
// el rótulo, y no suelto a su lado.
//
//	border-border-strong text-text inline-flex h-8 items-center gap-1.5 rounded-md
//	border px-3 text-sm font-semibold hover:bg-surface-sunken
//	<Icon users 16/> {rótulo} <Badge count class="ml-1">{n}</Badge>
type botonInvitaciones struct {
	widget.BaseWidget
	rotulo, pendientes string
	n                  int
	alPulsar           func()
	alEnfocar          func()
	sinIcono           bool

	encima, enfocado bool
}

var (
	_ fyne.Tappable     = (*botonInvitaciones)(nil)
	_ fyne.Focusable    = (*botonInvitaciones)(nil)
	_ desktop.Hoverable = (*botonInvitaciones)(nil)
)

func nuevoBotonInvitaciones(rotulo string, n int, pendientes string, alPulsar func()) *botonInvitaciones {
	b := &botonInvitaciones{rotulo: rotulo, n: n, pendientes: pendientes, alPulsar: alPulsar}
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonInvitaciones) Tapped(*fyne.PointEvent) {
	if b.alPulsar != nil {
		b.alPulsar()
	}
}

func (b *botonInvitaciones) FocusGained() {
	b.enfocado = true
	b.Refresh()
	if b.alEnfocar != nil {
		b.alEnfocar()
	}
}
func (b *botonInvitaciones) FocusLost()     { b.enfocado = false; b.Refresh() }
func (b *botonInvitaciones) TypedRune(rune) {}
func (b *botonInvitaciones) TypedKey(e *fyne.KeyEvent) {
	pulsarConTecla(b, e, func() { b.Tapped(nil) })
}
func (b *botonInvitaciones) MouseIn(*desktop.MouseEvent)    { b.encima = true; b.Refresh() }
func (b *botonInvitaciones) MouseMoved(*desktop.MouseEvent) {}
func (b *botonInvitaciones) MouseOut()                      { b.encima = false; b.Refresh() }

// AccessibilityLabel: el rótulo y, con pendientes, el «2 pendientes» de la insignia.
func (b *botonInvitaciones) AccessibilityLabel() string {
	if b.n > 0 {
		return b.rotulo + ", " + b.pendientes
	}
	return b.rotulo
}

func (b *botonInvitaciones) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

// MedidaBotonInvitaciones es la ÚNICA cuenta del ancho: borde, `px-3`, el icono, el
// `gap-1.5`, el rótulo y, con pendientes, otro `gap-1.5`, el `ml-1` y la insignia.
func MedidaBotonInvitaciones(rotulo string, n int) fyne.Size {
	ancho := 2*tema.Borde + 2*tema.Espacio(3) + 16 + espacio1p5 + ui.Medir(rotulo, tema.TextoSM, ui.SemiNegrita).Width
	if n > 0 {
		ancho += espacio1p5 + tema.Espacio(1) + ui.NuevoContador(contar(n)).MinSize().Width
	}
	return fyne.NewSize(ancho, AltoBotonCabecera)
}

func (b *botonInvitaciones) MinSize() fyne.Size {
	m := MedidaBotonInvitaciones(b.rotulo, b.n)
	if b.sinIcono {
		m.Width -= 16 + espacio1p5
	}
	return m
}

func (b *botonInvitaciones) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &invitacionesRenderer{
		b:     b,
		caja:  ui.NuevaCaja(tema.Color("surface", v), tema.Color("border-strong", v), tema.Forma("radius-control")),
		icono: iconos.Trazo(iconos.Usuarios, 16, "text", v),
		texto: ui.Texto(b.rotulo, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v)),
		aro:   ui.NuevoAro(tema.Forma("radius-control"), 0, v),
	}
	r.objetos = append(r.caja.Objetos(), r.texto)
	if !b.sinIcono {
		r.objetos = append(r.objetos, r.icono.Objeto())
	}
	r.objetos = r.aro.Antes(r.objetos...)
	if b.n > 0 {
		r.contador = ui.NuevoContador(contar(b.n))
		r.objetos = append(r.objetos, r.contador)
	}
	r.Refresh()
	return r
}

type invitacionesRenderer struct {
	b        *botonInvitaciones
	caja     *ui.Caja
	icono    *iconos.Dibujo
	texto    *canvas.Text
	contador *ui.Contador
	aro      *ui.Aro
	objetos  []fyne.CanvasObject
}

func (r *invitacionesRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *invitacionesRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *invitacionesRenderer) Destroy()                     {}

func (r *invitacionesRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	x := tema.Borde + tema.Espacio(3)
	if !r.b.sinIcono {
		r.icono.Colocar(x, (t.Height-16)/2, 16)
		x += 16 + espacio1p5
	}
	ui.ColocarTexto(r.texto, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
	x += ui.Medir(r.b.rotulo, tema.TextoSM, ui.SemiNegrita).Width
	if r.contador != nil {
		x += espacio1p5 + tema.Espacio(1)
		m := r.contador.MinSize()
		r.contador.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		r.contador.Resize(m)
	}
	r.aro.Colocar(t)
}

// Refresh: el `hover:bg-surface-sunken` de la cabecera.
func (r *invitacionesRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := tema.Color("surface", v)
	if r.b.encima {
		fondo = tema.Color("surface-sunken", v)
	}
	r.caja.Pintar(fondo, tema.Color("border-strong", v))
	r.icono.Tenir("text", v) // el modo puede cambiar con la app abierta
	r.texto.Color = tema.Color("text", v)
	r.texto.Refresh()
	r.aro.Ver(r.b.enfocado)
}
