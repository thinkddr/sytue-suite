// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

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
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Contrapartida de ThreadPanel.tsx dentro de la pantalla de canal declarada en
// pantalla.go.
//
// El panel del HILO: el mensaje raíz, sus respuestas y una caja para contestar, a la
// derecha del canal. Es el primero de los paneles de la derecha (`--rooms-rhs-width`);
// los demás —búsqueda, miembros, fijados, guardados, programados, reuniones— usan el
// mismo hueco.
//
// Medidas de la HOJA (`ThreadPanel.tsx`, rem 14):
//
//	panel      `w-[var(--rooms-rhs-width)]` = 22rem = **308** · `border-l` · `bg-surface`
//	cabecera   `h-14` (49) `bg-header` `px-4`, título `text-md` semi y «#canal» `text-xs` al 80 %
//	seguir     `h-8 px-2.5 gap-1.5 rounded-md text-sm font-medium`, glifo de 16
//	cerrar     `IconButton size="lg" tone="header"`: 28, aspa de 18
//	cuerpo     `py-2`; la raíz con `border-b pb-1`; el separador `my-2 px-4 gap-3`
//	fila       `flex gap-3 px-4 py-2`
var (
	// AnchoPanelDerecho es `--rooms-rhs-width`: 22rem.
	AnchoPanelDerecho float32 = 22 * tema.Rem
	// ArribaCuerpoHilo es el `py-2` del cuerpo, y AbajoRaiz el `pb-1` de la raíz.
	ArribaCuerpoHilo = tema.Espacio(2)
	AbajoRaiz        = tema.Espacio(1)
	// AireSeparadorHilo es el `my-2` del separador de «N respuestas».
	AireSeparadorHilo = tema.Espacio(2)
	// CanalonSeguir es el `px-2.5` del botón de seguir y HuecoSeguir su `gap-1.5`.
	CanalonSeguir = tema.Espacio(2.5)
	HuecoSeguir   = tema.Espacio(1.5)
	// GlifoSeguir es su `size={16}` y GlifoCerrarPanel el `size={18}` del aspa.
	GlifoSeguir      = iconos.TallaSM
	GlifoCerrarPanel = iconos.TallaMD
)

// PanelHilo es el hilo abierto.
type PanelHilo struct {
	widget.BaseWidget

	T      *i18n.Translator
	Locale i18n.Locale

	Raiz       Mensaje
	Respuestas []Mensaje
	// Estado es Cargando, Listo o Error: el hilo no tiene más estados que esos tres.
	Estado Estado
	// NombreCanal es la segunda línea de la cabecera: «#equipo».
	NombreCanal string
	Siguiendo   bool
	// Compositor es la caja de contestar, sin prioridad ni programar (en la web, sin
	// `advanced`) y con su propia marca de agua.
	Compositor *Compositor

	AlSeguir     func()
	AlCerrar     func()
	AlReintentar func()

	fondo, filete *canvas.Rectangle
	cabecera      *CabeceraPanel
	seguir        *BotonBanda
	cuerpo        fyne.CanvasObject
	objetos       []fyne.CanvasObject
}

var _ fyne.Widget = (*PanelHilo)(nil)

// NuevoPanelHilo crea el panel con su caja de contestar.
func NuevoPanelHilo(t *i18n.Translator, l i18n.Locale) *PanelHilo {
	c := NuevoCompositor(t, l)
	c.Sencillo = true
	c.MarcaDeAgua, c.Rotulo = t.T("thread.replyPlaceholder", nil), t.T("thread.replyAria", nil)
	p := &PanelHilo{T: t, Locale: l, Compositor: c, Estado: Cargando}
	p.ExtendBaseWidget(p)
	c.AlCambiarAlto = p.Refresh
	return p
}

// AccessibilityLabel es el `aria-label` del `<aside>`.
func (p *PanelHilo) AccessibilityLabel() string { return p.T.T("thread.title", nil) }

// AccessibilityRole implementa fyne.Accessible.
func (p *PanelHilo) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// CreateRenderer compone el panel.
func (p *PanelHilo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.fondo = canvas.NewRectangle(tema.Color("surface", v))
	p.filete = canvas.NewRectangle(tema.Color("border", v))
	r := &panelHiloRenderer{p: p}
	r.flujo = columnaConversacion{minimo: r.altoMinimo, colocar: r.colocarContenido}
	r.Refresh()
	return r
}

// construir rehace la cabecera y el cuerpo con los datos de ahora.
func (p *PanelHilo) construir(v tema.Variante) {
	t := p.T
	rotulo, pista := t.T("thread.followShort", nil), t.T("thread.followHint", nil)
	nombre := t.T("thread.follow", nil)
	if p.Siguiendo {
		rotulo, pista = t.T("thread.following", nil), t.T("thread.followingHint", nil)
		nombre = t.T("thread.unfollow", nil)
	}
	p.seguir = NuevoBotonBanda(rotulo, dibujo(glifoHilos, GlifoSeguir, "header-fg", v), p.AlSeguir)
	p.seguir.Nombre, p.seguir.Pulsado = nombre, p.Siguiendo
	sub := ""
	if p.NombreCanal != "" {
		sub = "#" + p.NombreCanal
	}
	p.cabecera = NuevaCabeceraPanel(t.T("thread.title", nil), sub,
		t.T("thread.close", nil), p.AlCerrar, ui.NuevaPista(pista, p.seguir))
	p.cuerpo = p.construirCuerpo(v)
	p.objetos = []fyne.CanvasObject{p.fondo, p.cuerpo, p.Compositor, p.cabecera, p.filete}
}

// construirCuerpo es la raíz, el separador y las respuestas, o el esqueleto o el
// aviso de error en su lugar.
func (p *PanelHilo) construirCuerpo(v tema.Variante) fyne.CanvasObject {
	t := p.T
	raiz := NuevaFilaMensaje(t, p.Locale, p.Raiz, false, nil)
	raiz.EnHilo = true
	cosas := []fyne.CanvasObject{
		hueco(ArribaCuerpoHilo),
		NuevoBloqueRaiz(raiz),
	}
	rotulo := t.T("thread.noReplies", nil)
	if n := len(p.Respuestas); n > 0 {
		rotulo = t.T("thread.replyCount", i18n.Vars{"count": n})
	}
	cosas = append(cosas, NuevoSeparadorHilo(rotulo))
	switch p.Estado {
	case Error:
		cosas = append(cosas, NuevoAvisoHilo(t.T("thread.loadFailed", nil), t.T("action.retry", nil), p.AlReintentar))
	case Cargando:
		cosas = append(cosas, NuevoEsqueletoHilo(t.T("thread.loading", nil)))
	default:
		for _, m := range p.Respuestas {
			f := NuevaFilaMensaje(t, p.Locale, m, false, nil)
			f.EnHilo = true
			cosas = append(cosas, f)
		}
	}
	cosas = append(cosas, hueco(ArribaCuerpoHilo))
	_ = v
	return container.NewVScroll(container.New(ui.Columna{}, cosas...))
}

type panelHiloRenderer struct {
	p     *PanelHilo
	flujo columnaConversacion
}

func (r *panelHiloRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, AltoCabecera)
}

func (r *panelHiloRenderer) Objects() []fyne.CanvasObject {
	if r.flujo.desbordando {
		return []fyne.CanvasObject{r.p.fondo, r.p.filete, r.flujo.desborde}
	}
	return r.p.objetos
}
func (r *panelHiloRenderer) Destroy() {}

func (r *panelHiloRenderer) altoMinimo(ancho float32) float32 {
	return AltoCabecera + AltoFilaMensaje(false) + r.p.Compositor.medidaPara(max(ancho-tema.Borde, 0)).Height
}

// Layout: el filete a la izquierda (`border-l`), la cabecera arriba, la caja de
// contestar abajo y el cuerpo con lo que sobra.
func (r *panelHiloRenderer) Layout(t fyne.Size) {
	p := r.p
	p.fondo.Resize(t)
	p.filete.Move(fyne.NewPos(0, 0))
	p.filete.Resize(fyne.NewSize(tema.Borde, t.Height))
	r.flujo.repartir(t, []fyne.CanvasObject{p.cabecera, p.cuerpo, p.Compositor})
}

func (r *panelHiloRenderer) colocarContenido(t fyne.Size) {
	p := r.p
	x, ancho := tema.Borde, max(t.Width-tema.Borde, 0)
	p.cabecera.Move(fyne.NewPos(x, 0))
	p.cabecera.Resize(fyne.NewSize(ancho, AltoCabecera))
	alto := p.Compositor.medidaPara(ancho).Height
	p.Compositor.Move(fyne.NewPos(x, max(t.Height-alto, AltoCabecera)))
	p.Compositor.Resize(fyne.NewSize(ancho, alto))
	p.cuerpo.Move(fyne.NewPos(x, AltoCabecera))
	p.cuerpo.Resize(fyne.NewSize(ancho, max(t.Height-AltoCabecera-alto, 0)))
}

// Refresh rehace el panel y lo deja abajo del todo: el hilo es corto y se lee por el
// final, que es el `scrollIntoView({block: 'end'})` de la web.
func (r *panelHiloRenderer) Refresh() {
	p := r.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.fondo.FillColor = tema.Color("surface", v)
	p.fondo.Refresh()
	p.filete.FillColor = tema.Color("border", v)
	p.filete.Refresh()
	p.construir(v)
	r.Layout(p.Size())
	if s, ok := p.cuerpo.(*container.Scroll); ok {
		s.ScrollToBottom()
	}
	p.Compositor.Refresh()
}

// ---- La cabecera de un panel de la derecha ----

// CabeceraPanel es la banda de tinta de un panel de la derecha: el título con su
// segunda línea a la izquierda y, a la derecha, lo que el panel ponga más el aspa.
// `header h-14 bg-header text-header-fg px-4 justify-between gap-2`.
type CabeceraPanel struct {
	widget.BaseWidget

	Titulo, Subtitulo string
	// Extra va a la izquierda del aspa (el «Seguir» del hilo), a HuecoExtra de ella: 0 es
	// el `gap-1` de las acciones; Reuniones lleva `gap-0.5`.
	Extra      fyne.CanvasObject
	HuecoExtra float32
	// Volver pone delante la flecha de volver (`-ml-1 h-8 w-8`, chevron de 18), con
	// su nombre en RotuloVolver; nil, no hay.
	Volver       func()
	RotuloVolver string
	volver       *ui.BotonIcono

	fondo          *canvas.Rectangle
	titulo, subtit *canvas.Text
	cerrar         *ui.BotonIcono
	objetos        []fyne.CanvasObject
	rotuloCerrar   string
	alCerrar       func()
}

var _ fyne.Widget = (*CabeceraPanel)(nil)

// NuevaCabeceraPanel crea la banda. `cerrar` es el nombre del aspa («Cerrar el hilo»).
func NuevaCabeceraPanel(titulo, subtitulo, cerrar string, alCerrar func(), extra fyne.CanvasObject) *CabeceraPanel {
	c := &CabeceraPanel{Titulo: titulo, Subtitulo: subtitulo, Extra: extra, rotuloCerrar: cerrar, alCerrar: alCerrar}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el `<h2>`.
func (c *CabeceraPanel) AccessibilityLabel() string { return c.Titulo }

// AccessibilityRole implementa fyne.Accessible.
func (c *CabeceraPanel) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// CreateRenderer compone la banda.
func (c *CabeceraPanel) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.fondo = canvas.NewRectangle(tema.Color("header-bg", v))
	c.titulo = ui.Texto(c.Titulo, escalonNombre, ui.SemiNegrita, tema.Color("header-fg", v))
	c.subtit = ui.Texto(c.Subtitulo, tema.TextoXS, ui.Normal, atenuado("header-fg", v, 0.8))
	c.cerrar = ui.NuevoBotonIcono(iconos.Cerrar, c.rotuloCerrar, c.alCerrar).SobreBanda()
	c.cerrar.Lado, c.cerrar.TamIcono = ui.LadoIconoLG, GlifoCerrarPanel
	c.objetos = []fyne.CanvasObject{c.fondo, c.titulo, c.subtit, c.cerrar}
	if c.Volver != nil {
		c.volver = ui.NuevoBotonIcono(iconos.ChevronIzquierda, c.RotuloVolver, c.Volver).SobreBanda()
		c.volver.Lado, c.volver.TamIcono = ui.LadoIconoLG, GlifoCerrarPanel
		c.objetos = append(c.objetos, c.volver)
	}
	if c.Extra != nil {
		c.objetos = append(c.objetos, c.Extra)
	}
	r := &cabeceraPanelRenderer{c: c}
	r.Refresh()
	return r
}

type cabeceraPanelRenderer struct{ c *CabeceraPanel }

func (r *cabeceraPanelRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, AltoCabecera) }
func (r *cabeceraPanelRenderer) Objects() []fyne.CanvasObject { return r.c.objetos }
func (r *cabeceraPanelRenderer) Destroy()                     {}

func (r *cabeceraPanelRenderer) Layout(t fyne.Size) {
	c := r.c
	c.fondo.Resize(t)
	der := t.Width - CanalonCabecera
	m := c.cerrar.MinSize()
	der -= m.Width
	c.cerrar.Move(fyne.NewPos(der, (t.Height-m.Height)/2))
	c.cerrar.Resize(m)
	if c.Extra != nil {
		me := c.Extra.MinSize()
		hueco := c.HuecoExtra
		if hueco == 0 {
			hueco = HuecoAcciones
		}
		der -= hueco + me.Width
		c.Extra.Move(fyne.NewPos(der, (t.Height-me.Height)/2))
		c.Extra.Resize(me)
	}
	x := CanalonCabecera
	if c.volver != nil {
		x -= tema.Espacio(1) // `-ml-1`
		c.volver.Move(fyne.NewPos(x, (t.Height-ui.LadoIconoLG)/2))
		c.volver.Resize(fyne.NewSize(ui.LadoIconoLG, ui.LadoIconoLG))
		x += ui.LadoIconoLG + tema.Espacio(2)
	}
	cabe := max(der-tema.Espacio(2)-x, 0)
	alto := escalonNombre.Linea
	if c.Subtitulo != "" {
		alto += tema.TextoXS.Linea
	}
	y := (t.Height - alto) / 2
	ui.Truncar(c.titulo, c.Titulo, escalonNombre, ui.SemiNegrita, cabe)
	ui.ColocarTexto(c.titulo, escalonNombre, x, y)
	if c.Subtitulo == "" {
		c.subtit.Hide()
		return
	}
	c.subtit.Show()
	ui.Truncar(c.subtit, c.Subtitulo, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(c.subtit, tema.TextoXS, x, y+escalonNombre.Linea)
}

func (r *cabeceraPanelRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.fondo.FillColor = tema.Color("header-bg", v)
	c.fondo.Refresh()
	c.titulo.Color = tema.Color("header-fg", v)
	c.titulo.Refresh()
	c.subtit.Color = atenuado("header-fg", v, 0.8)
	c.subtit.Refresh()
	r.Layout(c.Size())
}

// ---- El botón con texto sobre la banda ----

// BotonBanda es un botón con glifo y texto sobre la banda de tinta: `inline-flex h-8
// items-center gap-1.5 rounded-md px-2.5 text-sm font-medium text-header-fg
// hover:bg-chrome-hover`, y `bg-chrome-active` pulsado. La web lo escribe a mano (no
// es `Button`, que no tiene tono de cabecera) y aquí también.
//
// Sin glifo es el de solo texto: `h-8 rounded-md px-2 text-xs font-medium` (el
// «Reintentar» de Reuniones).
type BotonBanda struct {
	ui.Pulsable

	Rotulo  string
	Glifo   *iconos.Dibujo
	Pulsado bool

	aro     *ui.Aro
	caja    *ui.Caja
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

var _ fyne.Widget = (*BotonBanda)(nil)

// NuevoBotonBanda crea el botón. El nombre accesible es el rótulo salvo que se cambie.
func NuevoBotonBanda(rotulo string, glifo *iconos.Dibujo, alPulsar func()) *BotonBanda {
	b := &BotonBanda{Rotulo: rotulo, Glifo: glifo}
	b.Nombre, b.AlPulsar = rotulo, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MedidaBotonBanda es lo que ocupa el de glifo. Es la única cuenta.
func MedidaBotonBanda(rotulo string) fyne.Size {
	return fyne.NewSize(2*CanalonSeguir+GlifoSeguir+HuecoSeguir+ui.Medir(rotulo, tema.TextoSM, ui.Medio).Width,
		LadoAccionCabecera)
}

// CanalonBandaTexto es el `px-2` del de solo texto.
var CanalonBandaTexto = tema.Espacio(2)

// MedidaBotonBandaTexto es lo que ocupa el de solo texto.
func MedidaBotonBandaTexto(rotulo string) fyne.Size {
	return fyne.NewSize(2*CanalonBandaTexto+ui.Medir(rotulo, tema.TextoXS, ui.Medio).Width, LadoAccionCabecera)
}

// escalon es el del rótulo: `text-sm` con glifo, `text-xs` sin él.
func (b *BotonBanda) escalon() tema.Escalon {
	if b.Glifo == nil {
		return tema.TextoXS
	}
	return tema.TextoSM
}

// CreateRenderer compone la caja, el glifo y el texto.
func (b *BotonBanda) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.aro = ui.NuevoAroCon("chrome-focus", tema.RadioMD, ui.SinHueco, v)
	b.caja = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD)
	b.texto = ui.Texto(b.Rotulo, b.escalon(), ui.Medio, tema.Color("header-fg", v))
	b.objetos = append(b.aro.Objetos(), b.caja.Objetos()...)
	if b.Glifo != nil {
		b.objetos = append(b.objetos, b.Glifo.Objeto())
	}
	b.objetos = append(b.objetos, b.texto)
	r := &botonBandaRenderer{b: b}
	r.Refresh()
	return r
}

type botonBandaRenderer struct{ b *BotonBanda }

func (r *botonBandaRenderer) MinSize() fyne.Size {
	if r.b.Glifo == nil {
		return MedidaBotonBandaTexto(r.b.Rotulo)
	}
	return MedidaBotonBanda(r.b.Rotulo)
}
func (r *botonBandaRenderer) Objects() []fyne.CanvasObject { return r.b.objetos }
func (r *botonBandaRenderer) Destroy()                     {}

func (r *botonBandaRenderer) Layout(t fyne.Size) {
	b := r.b
	b.aro.Colocar(t)
	b.caja.Redimensionar(t)
	if b.Glifo == nil {
		ui.ColocarTexto(b.texto, tema.TextoXS, CanalonBandaTexto, (t.Height-tema.TextoXS.Linea)/2)
		return
	}
	b.Glifo.Colocar(CanalonSeguir, (t.Height-GlifoSeguir)/2, GlifoSeguir)
	ui.ColocarTexto(b.texto, tema.TextoSM, CanalonSeguir+GlifoSeguir+HuecoSeguir, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *botonBandaRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	switch {
	case b.Pulsado:
		fondo = tema.Color("chrome-active", v)
	case b.Encima():
		fondo = tema.Color("chrome-hover", v)
	}
	b.caja.Pintar(fondo, color.NRGBA{})
	b.aro.Ver(b.Enfocado())
	b.aro.Pintar(v)
	b.texto.Color = tema.Color("header-fg", v)
	b.texto.Refresh()
	r.Layout(b.Size())
}

// ---- Las piezas del cuerpo ----

// BloqueRaiz es la raíz destacada: `border-b border-border pb-1`.
type BloqueRaiz struct {
	widget.BaseWidget
	Fila *FilaMensaje

	filete *canvas.Rectangle
}

// NuevoBloqueRaiz envuelve la fila de la raíz.
func NuevoBloqueRaiz(f *FilaMensaje) *BloqueRaiz {
	b := &BloqueRaiz{Fila: f}
	b.ExtendBaseWidget(b)
	return b
}

// CreateRenderer compone la fila y su filete de abajo.
func (b *BloqueRaiz) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.filete = canvas.NewRectangle(tema.Color("border", v))
	return &bloqueRaizRenderer{b: b}
}

type bloqueRaizRenderer struct{ b *BloqueRaiz }

func (r *bloqueRaizRenderer) MinSize() fyne.Size {
	m := MedidaFilaMensaje(r.b.Fila, r.b.Size().Width)
	return fyne.NewSize(0, m.Height+AbajoRaiz+tema.Borde)
}

func (r *bloqueRaizRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.b.Fila, r.b.filete}
}
func (r *bloqueRaizRenderer) Destroy() {}

func (r *bloqueRaizRenderer) Layout(t fyne.Size) {
	alto := MedidaFilaMensaje(r.b.Fila, t.Width).Height
	r.b.Fila.Resize(fyne.NewSize(t.Width, alto))
	r.b.filete.Move(fyne.NewPos(0, alto+AbajoRaiz))
	r.b.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
}

func (r *bloqueRaizRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.b.filete.FillColor = tema.Color("border", v)
	r.b.filete.Refresh()
	r.Layout(r.b.Size())
}

// SeparadorHilo es «N respuestas» con su línea: `my-2 flex items-center gap-3 px-4`,
// el rótulo `text-xs font-semibold text-text-muted` y la línea `bg-border h-px flex-1`.
type SeparadorHilo struct {
	widget.BaseWidget
	Rotulo string

	texto *canvas.Text
	linea *canvas.Rectangle
}

// NuevoSeparadorHilo crea el separador.
func NuevoSeparadorHilo(rotulo string) *SeparadorHilo {
	s := &SeparadorHilo{Rotulo: rotulo}
	s.ExtendBaseWidget(s)
	return s
}

// AccessibilityLabel es el rótulo (`role="separator"`).
func (s *SeparadorHilo) AccessibilityLabel() string { return s.Rotulo }

// AccessibilityRole implementa fyne.Accessible.
func (s *SeparadorHilo) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// CreateRenderer compone el rótulo y la línea.
func (s *SeparadorHilo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	s.texto = ui.Texto(s.Rotulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v))
	s.linea = canvas.NewRectangle(tema.Color("border", v))
	return &separadorHiloRenderer{s: s}
}

type separadorHiloRenderer struct{ s *SeparadorHilo }

func (r *separadorHiloRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, 2*AireSeparadorHilo+tema.TextoXS.Linea)
}

func (r *separadorHiloRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.s.texto, r.s.linea}
}
func (r *separadorHiloRenderer) Destroy() {}

func (r *separadorHiloRenderer) Layout(t fyne.Size) {
	s := r.s
	x := CanalonFilaHilo
	ancho := ui.Medir(s.Rotulo, tema.TextoXS, ui.SemiNegrita).Width
	ui.ColocarTexto(s.texto, tema.TextoXS, x, AireSeparadorHilo)
	x += ancho + HuecoSeparador
	s.linea.Move(fyne.NewPos(x, AireSeparadorHilo+(tema.TextoXS.Linea-tema.Borde)/2))
	s.linea.Resize(fyne.NewSize(max(t.Width-CanalonFilaHilo-x, 0), tema.Borde))
}

func (r *separadorHiloRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.s.texto.Color = tema.Color("text-muted", v)
	r.s.texto.Refresh()
	r.s.linea.FillColor = tema.Color("border", v)
	r.s.linea.Refresh()
	r.Layout(r.s.Size())
}

// AvisoHilo es el `role="alert"` de «No se pudo cargar el hilo.» con su «Reintentar»
// subrayado: `px-4 py-3 text-sm text-danger-text`, aviso de 14 y `gap-1.5`.
type AvisoHilo struct {
	widget.BaseWidget
	Texto string

	reintentar *ui.Enlace
	glifo      *iconos.Dibujo
	texto      *canvas.Text
}

// NuevoAvisoHilo crea el aviso.
func NuevoAvisoHilo(texto, reintentar string, alReintentar func()) *AvisoHilo {
	a := &AvisoHilo{Texto: texto}
	if reintentar != "" {
		a.reintentar = ui.NuevoEnlace(reintentar, tema.TextoSM, ui.SemiNegrita, "danger-text", alReintentar)
		a.reintentar.Boton, a.reintentar.Subrayado = true, true
	}
	a.ExtendBaseWidget(a)
	return a
}

// AccessibilityLabel es el texto del aviso.
func (a *AvisoHilo) AccessibilityLabel() string { return a.Texto }

// AccessibilityRole implementa fyne.Accessible.
func (a *AvisoHilo) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// CreateRenderer compone glifo, texto y enlace.
func (a *AvisoHilo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a.glifo = iconos.Trazo(iconos.Aviso, iconos.TallaXS, "danger-text", v)
	a.texto = ui.Texto(a.Texto, tema.TextoSM, ui.Normal, tema.Color("danger-text", v))
	return &avisoHiloRenderer{a: a}
}

type avisoHiloRenderer struct{ a *AvisoHilo }

func (r *avisoHiloRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, 2*tema.Espacio(3)+tema.TextoSM.Linea)
}

func (r *avisoHiloRenderer) Objects() []fyne.CanvasObject {
	if r.a.reintentar == nil {
		return []fyne.CanvasObject{r.a.glifo.Objeto(), r.a.texto}
	}
	return []fyne.CanvasObject{r.a.glifo.Objeto(), r.a.texto, r.a.reintentar}
}
func (r *avisoHiloRenderer) Destroy() {}

func (r *avisoHiloRenderer) Layout(fyne.Size) {
	a := r.a
	x, y := CanalonFilaHilo, tema.Espacio(3)
	a.glifo.Colocar(x, y+(tema.TextoSM.Linea-iconos.TallaXS)/2, iconos.TallaXS)
	x += iconos.TallaXS + tema.Espacio(1.5)
	ui.ColocarTexto(a.texto, tema.TextoSM, x, y)
	x += ui.Medir(a.Texto, tema.TextoSM, ui.Normal).Width + tema.Espacio(1.5)
	if a.reintentar == nil {
		return
	}
	m := a.reintentar.MinSize()
	a.reintentar.Move(fyne.NewPos(x, y+(tema.TextoSM.Linea-m.Height)/2))
	a.reintentar.Resize(m)
}

func (r *avisoHiloRenderer) Refresh() { r.Layout(r.a.Size()) }

// NuevoEsqueletoHilo son las dos filas de «cargando» del hilo: `flex flex-col gap-3
// px-4 py-2`, cuadro de `2rem` y barras de `6rem` y del 60 y 45 %.
func NuevoEsqueletoHilo(nombre string) *EsqueletoMensajes {
	e := NuevoEsqueletoMensajes(nombre)
	e.Partes, e.AnchoNombre = []float32{0.60, 0.45}, 6*tema.Rem
	e.Arriba, e.Aire, e.SinAireArriba, e.Canalon = tema.Espacio(2), tema.Espacio(3), true, CanalonFilaHilo
	return e
}

// hueco es un espacio en blanco de ese alto, para una `ui.Columna`.
func hueco(alto float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.NRGBA{})
	r.SetMinSize(fyne.NewSize(0, alto))
	return r
}

// ---- «N respuestas» bajo un mensaje del canal ----

// AltoEnlaceHilo es `py-0.5 text-xs`: 14 más 3,5. HOJA.
var AltoEnlaceHilo = tema.TextoXS.Linea + 2*tema.Espacio(0.5)

// EnlaceHilo es el botón de «N respuestas» de la tira de un mensaje: `rounded-control
// text-primary-text hover:bg-selected inline-flex items-center gap-1 px-1.5 py-0.5
// text-xs font-medium`, con el chevron de 12 delante.
type EnlaceHilo struct {
	ui.Pulsable
	Rotulo string
	// alEncima avisa a la fila: sobre el enlace el mensaje sigue «encima».
	alEncima func(bool)

	aro     *ui.Aro
	caja    *ui.Caja
	glifo   *iconos.Dibujo
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

var _ fyne.Widget = (*EnlaceHilo)(nil)

// NuevoEnlaceHilo crea el botón.
func NuevoEnlaceHilo(rotulo string, alPulsar func()) *EnlaceHilo {
	e := &EnlaceHilo{Rotulo: rotulo}
	e.Nombre, e.AlPulsar = rotulo, alPulsar
	e.ExtendBaseWidget(e)
	return e
}

// MouseIn es el `:hover` del enlace, y avisa a la fila.
func (e *EnlaceHilo) MouseIn(ev *desktop.MouseEvent) {
	e.Pulsable.MouseIn(ev)
	if e.alEncima != nil {
		e.alEncima(true)
	}
}

// MouseOut apaga el `:hover` y suelta a la fila.
func (e *EnlaceHilo) MouseOut() {
	e.Pulsable.MouseOut()
	if e.alEncima != nil {
		e.alEncima(false)
	}
}

// MedidaEnlaceHilo es lo que ocupa. Es la única cuenta.
func MedidaEnlaceHilo(rotulo string) fyne.Size {
	return fyne.NewSize(2*tema.Espacio(1.5)+iconos.TallaXXS+tema.Espacio(1)+
		ui.Medir(rotulo, tema.TextoXS, ui.Medio).Width, AltoEnlaceHilo)
}

// CreateRenderer compone la píldora.
func (e *EnlaceHilo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e.aro = ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v)
	e.caja = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control"))
	e.glifo = iconos.Trazo(iconos.ChevronDerecha, iconos.TallaXXS, "primary-text", v)
	e.texto = ui.Texto(e.Rotulo, tema.TextoXS, ui.Medio, tema.Color("primary-text", v))
	e.objetos = append(e.aro.Objetos(), e.caja.Objetos()...)
	e.objetos = append(e.objetos, e.glifo.Objeto(), e.texto)
	r := &enlaceHiloRenderer{e: e}
	r.Refresh()
	return r
}

type enlaceHiloRenderer struct{ e *EnlaceHilo }

func (r *enlaceHiloRenderer) MinSize() fyne.Size           { return MedidaEnlaceHilo(r.e.Rotulo) }
func (r *enlaceHiloRenderer) Objects() []fyne.CanvasObject { return r.e.objetos }
func (r *enlaceHiloRenderer) Destroy()                     {}

func (r *enlaceHiloRenderer) Layout(t fyne.Size) {
	e := r.e
	e.aro.Colocar(t)
	e.caja.Redimensionar(t)
	x := tema.Espacio(1.5)
	e.glifo.Colocar(x, (t.Height-iconos.TallaXXS)/2, iconos.TallaXXS)
	ui.ColocarTexto(e.texto, tema.TextoXS, x+iconos.TallaXXS+tema.Espacio(1), (t.Height-tema.TextoXS.Linea)/2)
}

func (r *enlaceHiloRenderer) Refresh() {
	e := r.e
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if e.Encima() {
		fondo = tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
	}
	e.caja.Pintar(fondo, color.NRGBA{})
	e.aro.Ver(e.Enfocado())
	e.aro.Pintar(v)
	e.glifo.Tenir("primary-text", v)
	e.texto.Color = tema.Color("primary-text", v)
	e.texto.Refresh()
	r.Layout(e.Size())
}
