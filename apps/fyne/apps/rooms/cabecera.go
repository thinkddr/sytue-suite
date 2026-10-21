// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La cabecera del canal: la banda de tinta con el nombre, el tema y las acciones.
//
// No es `ui.CabeceraApp`, y la diferencia está medida: aquella es la cabecera BLANCA de
// una pantalla de contenido; esta va sobre `header-bg` —un paso de tinta bajo la barra
// superior— y mide 49 de alto contra los 56 de aquella, lleva dos líneas de texto y una
// marca delante. Son dos piezas distintas en la web (`header.bg-header` contra
// `PageHeader`) y aquí también.
//
// Medido en `rooms/canal.ancho.json` (1280×800, rem 14):
//
//	banda      1020 × **49** en (260, 56) · `bg-header` · `px-4` (14) · `gap-3` (10,5)
//	marca      **28 × 28**, radio 6, `bg-chrome-hover`, el «#» en `text-md` semi
//	nombre     `text-md` semi-negrita, `leading-tight` · tema `text-xs` al 80 %
//	acciones   **28 × 28** con radio 6 y glifo de **16**, separadas 3,5 (`gap-1`)
//	llamar     botón `sm` primario, 28 de alto, con la cámara delante
//	filete     `bg-border mx-1 h-5 w-px` entre las acciones y «Llamar»
var (
	// AltoCabecera es el `h-14` de la banda. MEDIDO: 49.
	AltoCabecera = tema.Espacio(14)
	// CanalonCabecera es su `px-4`. MEDIDO (padding `0px 14px`).
	CanalonCabecera = tema.Espacio(4)
	// HuecoCabecera es el `gap-3` entre marca, texto y acciones. MEDIDO: 10,5.
	HuecoCabecera = tema.Espacio(3)
	// HuecoAcciones es el `gap-1` entre los botones. MEDIDO: 3,5.
	HuecoAcciones = tema.Espacio(1)
	// LadoMarcaCanal es el `h-8 w-8` del «#» o del avatar. MEDIDO: 28.
	LadoMarcaCanal = tema.Espacio(8)
	// LadoAccionCabecera es el `h-8 w-8` de cada acción. MEDIDO: 28.
	LadoAccionCabecera = tema.Espacio(8)
	// GlifoCabecera es el `size={16}` de sus iconos. MEDIDO: 16×16.
	GlifoCabecera = iconos.TallaSM
	// AltoFileteCabecera es el `h-5` del separador vertical. HOJA: 17,5.
	AltoFileteCabecera = tema.Espacio(5)
	// LineaNombre es el `leading-tight` (1,25) del nombre del canal sobre `text-md`.
	LineaNombre = 1.25 * tema.TextoMD.Tam
)

// escalonNombre es `text-md leading-tight`: 14 de letra en caja de 17,5.
var escalonNombre = tema.Escalon{Tam: tema.TextoMD.Tam, Linea: LineaNombre}

// Cabecera es la banda del canal abierto.
//
// Las acciones que la web esconde por debajo de 768 (`max-md:hidden`) se esconden
// también aquí, y las cuatro que quedan —resumen, buscar, llamar y el menú del
// nombre— son las mismas: los seis botones juntos se salían 98 px de un iPhone SE.
type Cabecera struct {
	widget.BaseWidget

	T      *i18n.Translator
	Locale i18n.Locale

	// Canal es el abierto. Sin él la cabecera no se pinta: la pantalla enseña el
	// estado vacío entero.
	Canal Canal
	// Miembros es cuántos hay; cero esconde el contador y deja el icono, que es lo que
	// hace la web cuando la carga del roster falló.
	Miembros int
	// Caras son los tres primeros nombres del roster, para la pila de avatares.
	Caras []string
	// Fijados es el número del botón de la chincheta.
	Fijados int
	// Panel es el panel abierto a la derecha, para el `aria-pressed` de su botón.
	Panel string
	// Resumiendo apaga «Ponme al día» mientras Sytue AI contesta.
	Resumiendo bool
	// SinConexion apaga lo que no se puede hacer sin servidor.
	SinConexion bool
	// PuedeEditarTema es quien administra el equipo en un canal de la casa: el tema es
	// un botón que lo edita. EditandoTema cambia la línea por su campo.
	PuedeEditarTema, EditandoTema bool
	AlEditarTema                  func()
	AlGuardarTema                 func(string)
	AlCancelarTema                func()

	// AlAbrirMenu despliega el menú del canal colgando del nombre (`MessageMenu` con
	// el nombre como disparador). En un directo no hay menú.
	AlAbrirMenu   func(disparador fyne.CanvasObject)
	AlResumir     func()
	AlAbrirPanel  func(string)
	AlVerArchivos func()
	AlLlamar      func()
	// AlMenu es la hamburguesa de la ventana estrecha, que saca el carril como cajón
	// (`md:hidden` en la web, «equivale a atrás a la lista»); nil, no se pinta.
	AlMenu func()
	// Estrecha es la ventana por debajo de `md`: sale la hamburguesa y se esconden las
	// acciones con `max-md:hidden` —el roster, fijados, archivos, guardados,
	// programados y el filete—, que siguen en el menú del nombre.
	Estrecha bool

	// Los widgets se construyen una vez y el renderer los coloca.
	marca                        fyne.CanvasObject
	menu                         *ui.BotonIcono
	acciones                     []fyne.CanvasObject
	llamar                       *ui.Boton
	nombre, rotuloTema, insignia *canvas.Text
	disparador                   *DisparadorNombre
	pildora                      *ui.Caja
	fondo, filete                *canvas.Rectangle
	objetos                      []fyne.CanvasObject
	// editorTema, lineaTema y botonTema son la línea del tema cuando se edita o se
	// puede editar.
	editorTema *ui.EditorPlano
	lineaTema  *canvas.Rectangle
	botonTema  *BotonTema
}

// Las claves de los paneles que abre la cabecera.
const (
	PanelMiembros   = "members"
	PanelBusqueda   = "search"
	PanelFijados    = "pinned"
	PanelGuardados  = "saved"
	PanelProgramado = "scheduled"
)

var _ fyne.Widget = (*Cabecera)(nil)

// NuevaCabecera crea la banda con su traductor.
func NuevaCabecera(t *i18n.Translator, l i18n.Locale) *Cabecera {
	c := &Cabecera{T: t, Locale: l}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el nombre del canal, que en la web es el `<h1>` de la vista.
func (c *Cabecera) AccessibilityLabel() string { return c.Canal.Nombre }

// AccessibilityRole implementa fyne.Accessible.
func (c *Cabecera) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MedidaCabecera es lo que ocupa: todo el ancho que le den por el alto de la banda. Es
// la única cuenta.
func MedidaCabecera() fyne.Size { return fyne.NewSize(0, AltoCabecera) }

// CreateRenderer construye la banda. Solo primitivas y widgets de `ui`.
func (c *Cabecera) CreateRenderer() fyne.WidgetRenderer {
	c.montar(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r := &cabeceraRenderer{c: c}
	r.Refresh()
	return r
}

// montar (re)construye las piezas de la banda. Se llama también en cada Refresh: al
// cambiar de canal cambian la marca (el «#» o el avatar), la insignia de privado y
// los botones —un directo no lleva roster—, y construirlos es barato.
func (c *Cabecera) montar(v tema.Variante) {
	t := c.T

	c.fondo = canvas.NewRectangle(tema.Color("header-bg", v))
	c.filete = canvas.NewRectangle(ui.BordeSobre("border", "header-bg", v))
	c.marca = c.marcaCanal(v)

	// El nombre del canal ES el disparador de su menú (Slack, Teams y Mattermost lo
	// hacen así). En un directo no hay menú que abrir y se queda como texto.
	c.nombre = ui.Texto(c.Canal.Nombre, escalonNombre, ui.SemiNegrita, tema.Color("header-fg", v))
	c.disparador = nil
	if c.Canal.Clase != Directo {
		// El nombre del canal ES el disparador de su menú: `min-h-[24px] px-1.5 py-0.5
		// gap-1 rounded-md`, el chevron de 14 y `hover:bg-chrome-hover`.
		c.disparador = NuevoDisparadorNombre(t.T("channel.menuAria", i18n.Vars{"name": c.Canal.Nombre}), nil)
		d := c.disparador
		d.AlPulsar = func() {
			if c.AlAbrirMenu != nil {
				c.AlAbrirMenu(d)
			}
		}
	}
	c.rotuloTema = ui.Texto(c.lineaDeDebajo(), tema.TextoXS, ui.Normal, atenuado("header-fg", v, 0.8))
	c.insignia, c.pildora = nil, nil
	if c.Canal.Clase == Privado {
		// `rounded-pill border border-chrome-border px-2 py-0.5 text-xs font-medium`.
		c.insignia = ui.Texto(t.T("channel.privateBadge", nil), tema.TextoXS, ui.Medio,
			tema.Color("header-fg", v))
		c.pildora = ui.NuevaCaja(tema.Color("header-bg", v), tema.Color("chrome-border", v), tema.RadioPill)
	}

	c.acciones = c.construirAcciones(v)
	c.llamar = ui.NuevoBoton(t.T("call.call", nil), c.AlLlamar).Con(ui.Primario, ui.TallaSM)
	c.llamar.Glifo = dibujo(glifoVideo, GlifoCabecera, "on-primary", v)
	c.llamar.TamIcono = GlifoCabecera
	c.llamar.Inerte = c.SinConexion

	c.menu = nil
	if c.Estrecha && c.AlMenu != nil {
		c.menu = ui.NuevoBotonIcono(iconos.Menu, t.T("sidebar.openChannels", nil), c.AlMenu).SobreBanda()
		c.menu.Lado, c.menu.TamIcono = LadoAccionCabecera, GlifoCabecera
	}
	c.objetos = []fyne.CanvasObject{c.fondo, c.marca}
	if c.disparador != nil {
		c.objetos = append(c.objetos, c.disparador)
	}
	c.objetos = append(c.objetos, c.nombre)
	if c.pildora != nil {
		c.objetos = append(c.objetos, c.pildora.Objetos()...)
	}
	if c.menu != nil {
		c.objetos = append(c.objetos, c.menu)
	}
	if c.insignia != nil {
		c.objetos = append(c.objetos, c.insignia)
	}
	c.objetos = append(c.objetos, c.rotuloTema)
	if c.Canal.Clase != Directo {
		c.montarTema(v)
	}
	c.objetos = append(c.objetos, c.acciones...)
	c.objetos = append(c.objetos, c.filete, c.llamar)
}

// marcaCanal es el «#» sobre `chrome-hover` de un canal, o el avatar de un directo.
func (c *Cabecera) marcaCanal(v tema.Variante) fyne.CanvasObject {
	if c.Canal.Clase == Directo {
		a := ui.NuevoAvatar(c.Canal.Nombre).Con(ui.AvatarSM)
		a.Presencia = c.Canal.Presencia
		return a
	}
	_ = v
	return NuevaMarcaCanal()
}

// construirAcciones son los botones de la derecha, EN EL ORDEN del DOM: resumen,
// miembros, buscar, fijados, archivos, guardados, programados.
func (c *Cabecera) construirAcciones(v tema.Variante) []fyne.CanvasObject {
	t := c.T
	boton := func(rotulo, cuerpo, panel string, al func()) *ui.BotonIcono {
		b := ui.NuevoBotonIcono(iconos.Ninguno, rotulo, al).SobreBanda()
		b.Glifo = dibujo(cuerpo, GlifoCabecera, "header-fg", v)
		b.Lado, b.TamIcono = LadoAccionCabecera, GlifoCabecera
		b.Pulsado = panel != "" && c.Panel == panel
		return b
	}
	del := func(i iconos.Icono, rotulo, panel string, al func()) *ui.BotonIcono {
		b := ui.NuevoBotonIcono(i, rotulo, al).SobreBanda()
		b.Lado, b.TamIcono = LadoAccionCabecera, GlifoCabecera
		b.Pulsado = panel != "" && c.Panel == panel
		return b
	}
	resumir := del(iconos.Informacion, t.T("channel.summarize", nil), "", c.AlResumir)
	resumir.Inerte = c.Resumiendo || c.SinConexion

	buscar := del(iconos.Buscar, t.T("channel.search", nil), PanelBusqueda,
		func() { c.AlAbrirPanel(PanelBusqueda) })
	if c.Estrecha {
		return []fyne.CanvasObject{resumir, buscar}
	}
	out := []fyne.CanvasObject{resumir}
	if c.Canal.Clase != Directo {
		out = append(out, c.pilaDeCaras(v))
	}
	out = append(out,
		buscar,
		boton(t.T("channel.pinned", nil), glifoChincheta, PanelFijados,
			func() { c.AlAbrirPanel(PanelFijados) }),
		boton(t.T("channel.files", nil), glifoClip, "", c.AlVerArchivos),
		boton(t.T("channel.saved", nil), glifoMarcador, PanelGuardados,
			func() { c.AlAbrirPanel(PanelGuardados) }),
		boton(t.T("channel.scheduled", nil), glifoReloj, PanelProgramado,
			func() { c.AlAbrirPanel(PanelProgramado) }),
	)
	return out
}

// pilaDeCaras es el botón del roster: hasta tres avatares solapados y el número.
//
// ponytail: los avatares van en FILA con su `gap`, no solapados por el `-ml-1.5` con
// el anillo del color de la banda. El solape pide recortar un anillo alrededor de cada
// avatar y `ui.Avatar` no tiene anillo; sumarlo es tocar una pieza compartida por una
// cabecera. El botón mide lo mismo porque el número es lo que manda su ancho.
func (c *Cabecera) pilaDeCaras(v tema.Variante) fyne.CanvasObject {
	t := c.T
	rotulo := t.T("channel.members", nil)
	if c.Miembros > 0 {
		rotulo = t.T("channel.membersCount", i18n.Vars{"count": c.Miembros})
	}
	if c.Miembros == 0 {
		b := ui.NuevoBotonIcono(iconos.Usuarios, rotulo, func() { c.AlAbrirPanel(PanelMiembros) }).
			SobreBanda()
		b.Lado, b.TamIcono = LadoAccionCabecera, GlifoCabecera
		b.Pulsado = c.Panel == PanelMiembros
		return b
	}
	_ = v
	return NuevaPilaCaras(c.Caras, Contador(c.Miembros), rotulo,
		c.Panel == PanelMiembros, func() { c.AlAbrirPanel(PanelMiembros) })
}

type cabeceraRenderer struct{ c *Cabecera }

func (r *cabeceraRenderer) MinSize() fyne.Size           { return MedidaCabecera() }
func (r *cabeceraRenderer) Objects() []fyne.CanvasObject { return r.c.objetos }
func (r *cabeceraRenderer) Destroy()                     {}

// Layout reparte la banda: marca, bloque de texto que se estira, y acciones a la
// derecha. El texto se recorta con puntos suspensivos, que es el `truncate` de la web.
func (r *cabeceraRenderer) Layout(t fyne.Size) {
	c := r.c
	c.fondo.Resize(t)

	x := CanalonCabecera
	if c.menu != nil {
		// `-ml-1 h-8 w-8` y el `gap-3` de la banda.
		x -= tema.Espacio(1)
		c.menu.Move(fyne.NewPos(x, (t.Height-LadoAccionCabecera)/2))
		c.menu.Resize(fyne.NewSize(LadoAccionCabecera, LadoAccionCabecera))
		x += LadoAccionCabecera + HuecoCabecera
	}
	c.marca.Move(fyne.NewPos(x, (t.Height-LadoMarcaCanal)/2))
	c.marca.Resize(fyne.NewSize(LadoMarcaCanal, LadoMarcaCanal))
	x += LadoMarcaCanal + HuecoCabecera

	// Las acciones, desde la derecha: primero «Llamar», su filete y luego los botones
	// en orden inverso, para que el sobrante se lo quede el bloque de texto.
	der := t.Width - CanalonCabecera
	mLlamar := c.llamar.MinSize()
	der -= mLlamar.Width
	c.llamar.Move(fyne.NewPos(der, (t.Height-mLlamar.Height)/2))
	c.llamar.Resize(mLlamar)
	if c.Estrecha {
		c.filete.Hide()
	} else {
		c.filete.Show()
		der -= HuecoAcciones + tema.Espacio(1)
		c.filete.Move(fyne.NewPos(der, (t.Height-AltoFileteCabecera)/2))
		c.filete.Resize(fyne.NewSize(tema.Borde, AltoFileteCabecera))
		der -= tema.Espacio(1)
	}
	for i := len(c.acciones) - 1; i >= 0; i-- {
		m := c.acciones[i].MinSize()
		der -= m.Width
		c.acciones[i].Move(fyne.NewPos(der, (t.Height-m.Height)/2))
		c.acciones[i].Resize(m)
		der -= HuecoAcciones
	}

	// El bloque de texto: dos líneas centradas en la banda (`flex-col justify-center`).
	// Con disparador, la fila del nombre mide sus 24 (`min-h-[24px]`) y el texto va
	// centrado en ella; sin él, la caja de línea de 17,5.
	cabe := max(der-HuecoCabecera-x, 0)
	fila := escalonNombre.Linea
	if c.disparador != nil {
		fila = AltoDisparadorNombre
	}
	alto := fila
	if c.conLineaTema() {
		alto += tema.TextoXS.Linea
	}
	y := (t.Height - alto) / 2
	anchoNombre := cabe
	if c.insignia != nil {
		mi := MedidaPildoraPrivado(c.insignia.Text).Width
		anchoNombre = max(cabe-mi-tema.Espacio(2), 0)
	}
	if c.disparador != nil {
		anchoNombre = max(anchoNombre-DisparadorSobra(), 0)
	}
	ancho := ui.Truncar(c.nombre, c.Canal.Nombre, escalonNombre, ui.SemiNegrita, anchoNombre)
	ui.ColocarTexto(c.nombre, escalonNombre, x, y+(fila-escalonNombre.Linea)/2)
	fin := x + ancho
	if d := c.disparador; d != nil {
		// El `-mx-1.5` del `<h1>`: el texto se queda alineado y la caja sobresale.
		d.Move(fyne.NewPos(x-CanalonDisparador, y))
		d.Resize(fyne.NewSize(ancho+DisparadorSobra(), AltoDisparadorNombre))
		fin = x - CanalonDisparador + ancho + DisparadorSobra()
	}
	if c.insignia != nil {
		mp := MedidaPildoraPrivado(c.insignia.Text)
		px := fin + tema.Espacio(2)
		ui.MoverCaja(c.pildora, fyne.NewPos(px, y+(fila-mp.Height)/2), mp)
		ui.ColocarTexto(c.insignia, tema.TextoXS, px+tema.Borde+tema.Espacio(2), y+(fila-tema.TextoXS.Linea)/2)
	}
	c.colocarTema(x, y+fila, cabe)
}

// MedidaPildoraPrivado es la píldora de «Privado»: el texto con `px-2 py-0.5` y borde.
func MedidaPildoraPrivado(texto string) fyne.Size {
	return fyne.NewSize(ui.Medir(texto, tema.TextoXS, ui.Medio).Width+2*tema.Espacio(2)+2*tema.Borde,
		tema.TextoXS.Linea+2*tema.Espacio(0.5)+2*tema.Borde)
}

// ---- El nombre del canal como disparador de su menú ----

// Medidas del disparador. HOJA.
var (
	// AltoDisparadorNombre es su `min-h-[24px]`, en px para cumplir WCAG 2.5.8.
	AltoDisparadorNombre float32 = 24
	// CanalonDisparador es su `px-1.5`, HuecoDisparador el `gap-1` y GlifoDisparador el
	// chevron de 14.
	CanalonDisparador = tema.Espacio(1.5)
	HuecoDisparador   = tema.Espacio(1)
	GlifoDisparador   = iconos.TallaXS
)

// DisparadorSobra es lo que el disparador ocupa además del nombre: sus dos canalones,
// el hueco y el chevron.
func DisparadorSobra() float32 { return 2*CanalonDisparador + HuecoDisparador + GlifoDisparador }

// DisparadorNombre es la caja pulsable del nombre: el nombre lo pinta la cabecera
// (para que se recorte igual con o sin menú) y esto pone el fondo del `hover`, el
// chevron y el anillo de foco.
type DisparadorNombre struct {
	ui.Pulsable

	aro     *ui.Aro
	caja    *ui.Caja
	chevron *iconos.Dibujo
}

var _ fyne.Widget = (*DisparadorNombre)(nil)

// NuevoDisparadorNombre crea el disparador con su nombre accesible («Menú del canal
// #equipo»).
func NuevoDisparadorNombre(nombre string, alPulsar func()) *DisparadorNombre {
	d := &DisparadorNombre{}
	d.Nombre, d.AlPulsar = nombre, alPulsar
	d.ExtendBaseWidget(d)
	return d
}

// CreateRenderer compone el fondo, el chevron y el anillo.
func (d *DisparadorNombre) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	d.aro = ui.NuevoAroCon("chrome-focus", tema.Forma("radius-control"), ui.SinHueco, v).Abierto()
	d.caja = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control"))
	d.chevron = iconos.Trazo(iconos.ChevronAbajo, GlifoDisparador, "header-fg", v)
	r := &disparadorRenderer{d: d}
	r.objetos = append(d.aro.Antes(d.caja.Objetos()...), d.chevron.Objeto())
	r.Refresh()
	return r
}

type disparadorRenderer struct {
	d       *DisparadorNombre
	objetos []fyne.CanvasObject
}

func (r *disparadorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(DisparadorSobra(), AltoDisparadorNombre)
}
func (r *disparadorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *disparadorRenderer) Destroy()                     {}

func (r *disparadorRenderer) Layout(t fyne.Size) {
	d := r.d
	d.aro.Colocar(t)
	d.caja.Redimensionar(t)
	d.chevron.Colocar(t.Width-CanalonDisparador-GlifoDisparador, (t.Height-GlifoDisparador)/2, GlifoDisparador)
}

func (r *disparadorRenderer) Refresh() {
	d := r.d
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if d.Encima() {
		fondo = tema.Color("chrome-hover", v)
	}
	d.caja.Pintar(fondo, color.NRGBA{})
	d.aro.Ver(d.Enfocado())
	d.aro.Pintar(v)
	d.chevron.Tenir("header-fg", v)
	r.Layout(d.Size())
}

// Refresh repinta con los colores del tema actual y rehace la disposición: cambiar de
// canal no cambia el tamaño de la banda, y sin el Layout el nombre se quedaría con el
// recorte del canal anterior.
func (r *cabeceraRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.montar(v)
	c.fondo.FillColor = tema.Color("header-bg", v)
	c.fondo.Refresh()
	c.filete.FillColor = ui.BordeSobre("border", "header-bg", v)
	c.filete.Refresh()
	c.nombre.Color = tema.Color("header-fg", v)
	c.nombre.Refresh()
	c.rotuloTema.Color = atenuado("header-fg", v, 0.8)
	c.rotuloTema.Refresh()
	r.Layout(c.Size())
}

// ---- La marca «#» de la cabecera ----

// MarcaCanal es el cuadrado de 28 con el «#» dentro: `bg-chrome-hover text-md
// font-semibold grid h-8 w-8 place-items-center rounded-md`. Es decorativo
// (`aria-hidden`): el nombre del canal ya está al lado.
//
// La misma pieza es el `ChannelIcon` de las listas de los diálogos (Explorar,
// Reenviar), que cambia la banda por el papel: `bg-bg-subtle text-text-muted`.
type MarcaCanal struct {
	widget.BaseWidget
	// Fondo y Tinta son los tokens de la caja y del «#».
	Fondo, Tinta string
}

var _ fyne.Widget = (*MarcaCanal)(nil)

// NuevaMarcaCanal crea la marca de la banda.
func NuevaMarcaCanal() *MarcaCanal {
	m := &MarcaCanal{Fondo: "chrome-hover", Tinta: "header-fg"}
	m.ExtendBaseWidget(m)
	return m
}

// NuevaMarcaCanalPapel es `ChannelIcon`: la marca sobre el papel de un diálogo.
func NuevaMarcaCanalPapel() *MarcaCanal {
	m := &MarcaCanal{Fondo: "bg-subtle", Tinta: "text-muted"}
	m.ExtendBaseWidget(m)
	return m
}

// MedidaMarcaCanal es el `h-8 w-8`. MEDIDO: 28.
func MedidaMarcaCanal() fyne.Size { return fyne.NewSize(LadoMarcaCanal, LadoMarcaCanal) }

// CreateRenderer compone la caja y el signo.
func (m *MarcaCanal) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e := tema.Escalon{Tam: tema.TextoMD.Tam, Linea: tema.TextoMD.Tam}
	r := &marcaCanalRenderer{
		m:     m,
		caja:  ui.NuevaCaja(tema.Color(m.Fondo, v), color.NRGBA{}, tema.RadioMD),
		signo: ui.Texto("#", e, ui.SemiNegrita, tema.Color(m.Tinta, v)),
		e:     e,
	}
	r.objetos = append(r.caja.Objetos(), r.signo)
	r.Refresh()
	return r
}

type marcaCanalRenderer struct {
	m       *MarcaCanal
	caja    *ui.Caja
	signo   *canvas.Text
	e       tema.Escalon
	objetos []fyne.CanvasObject
}

func (r *marcaCanalRenderer) MinSize() fyne.Size           { return MedidaMarcaCanal() }
func (r *marcaCanalRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *marcaCanalRenderer) Destroy()                     {}

func (r *marcaCanalRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	ancho := ui.Medir("#", r.e, ui.SemiNegrita).Width
	ui.ColocarTexto(r.signo, r.e, (t.Width-ancho)/2, (t.Height-r.e.Linea)/2)
}

func (r *marcaCanalRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color(r.m.Fondo, v), color.NRGBA{})
	r.signo.Color = tema.Color(r.m.Tinta, v)
	r.signo.Refresh()
	r.Layout(r.m.Size())
}
