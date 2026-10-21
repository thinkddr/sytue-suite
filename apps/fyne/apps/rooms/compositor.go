// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La caja de escribir.
//
// # El editor NO se reescribe
//
// Se usa `ui.EditorPlano`, que es `widget.Entry` con el cromo apagado por tema. Está
// medido en `ui/campo.go`: así el editor pinta su texto en el MISMO píxel que un
// `canvas.Text` nuestro del mismo escalón (desvío 0,000 px) y se le puede imponer el
// interlineado de CSS, que aquí es `leading-6` —21— y no el 1,2 em de la fuente.
// Escribir un editor propio para el compositor sería rehacer selección, cursor,
// portapapeles y deshacer para ganar cero.
//
// # Medidas, de `rooms/canal.ancho.json` (1280×800, rem 14)
//
//	caja        992 de ancho · `rounded-lg` (8) · filete `border` · `px-1.5 pt-1 pb-1.5`
//	área        **979,5 × 35** MEDIDO (`min-h-[2.5rem]`), `px-2.5 pt-2 pb-1`,
//	            `text-base` con `leading-6` (21), fondo transparente y sin borde
//	barra       `flex items-center gap-1 px-0.5 pt-0.5`, botones de **28 × 28** MEDIDO
//	            con glifos de **18** MEDIDO
//	enviar      el ÚNICO con caja: botón `sm` de solo icono, empujado a la derecha
//	            (`ms-auto`). `ghost` sin nada que enviar y `primary` en cuanto lo hay.
//
// La caja NO se enciende al enfocar: el foco lo pinta el área y solo ella. Mientras lo
// pintaban las dos salían dos rectángulos, uno alrededor de todo —barra incluida— y
// otro alrededor del texto; se quitó en la web el 2026-09-09.
var (
	// AnchoInteriorCompositor es lo que le queda al área dentro de la caja de 992.
	// MEDIDO: 979,5.
	AnchoInteriorCompositor float32 = 979.5
	// RadioCompositor es su `rounded-lg`.
	RadioCompositor = tema.RadioLG
	// CanalonCompositor es el `px-1.5` de la caja; ArribaCompositor el `pt-1` y
	// AbajoCompositor el `pb-1.5`.
	CanalonCompositor = tema.Espacio(1.5)
	ArribaCompositor  = tema.Espacio(1)
	AbajoCompositor   = tema.Espacio(1.5)
	// AltoAreaMinimo es el `min-h-[2.5rem]`. MEDIDO: 35.
	AltoAreaMinimo float32 = 2.5 * tema.Rem
	// AltoAreaMaximo es el `max-h-[200px]`: a partir de ahí desplaza por dentro y la
	// barra se queda debajo y a la vista.
	AltoAreaMaximo float32 = 200
	// CanalonArea es su `px-2.5`, ArribaArea el `pt-2` y AbajoArea el `pb-1`.
	CanalonArea = tema.Espacio(2.5)
	ArribaArea  = tema.Espacio(2)
	AbajoArea   = tema.Espacio(1)
	// CanalonBarra es el `px-0.5` de la barra, ArribaBarra su `pt-0.5` y HuecoBarra
	// su `gap-1`.
	CanalonBarra = tema.Espacio(0.5)
	ArribaBarra  = tema.Espacio(0.5)
	HuecoBarra   = tema.Espacio(1)
	// LadoBotonBarra es el `h-8 w-8` de cada botón. MEDIDO: 28.
	LadoBotonBarra = tema.Espacio(8)
	// GlifoBarra es el `size={18}` de sus iconos. MEDIDO: 18×18.
	GlifoBarra = iconos.TallaMD
	// MargenCompositor es el `px-4` de la franja donde vive la caja, el mismo que el
	// del historial.
	MargenCompositor = tema.Espacio(4)
	// ArribaFranja y AbajoFranja son su `pt-3` y su `pb-3` (el `max(0.75rem,
	// safe-area)` de abajo vale 10,5 fuera de un teléfono con barra de gestos), y
	// encima va un filete `border-t`. MEDIDO en `canal.ancho.png`: el filete en
	// y = 703 y la caja de 714,5 a 790, o sea 10,5 hasta el borde de la ventana.
	//
	// Antes eran 14 por los dos lados y sin filete: la franja medía 6 de más y el
	// historial perdía ese alto sin que nadie lo viera.
	ArribaFranja = tema.Espacio(3)
	AbajoFranja  = tema.Espacio(3)
	// MargenSeparadorBarra es el `mx-1` del pelo que parte la barra (SEPARADOR_DE_BARRA
	// de `cromoDeEditor.ts`: `bg-border mx-1 h-5 w-px`) y AltoSeparadorBarra su `h-5`.
	// A la izquierda lo que se AÑADE al mensaje; a la derecha CÓMO se envía.
	MargenSeparadorBarra = tema.Espacio(1)
	AltoSeparadorBarra   = tema.Espacio(5)
)

// escalonCompositor es `text-base leading-6`: 12,25 de letra en caja de 21.
var escalonCompositor = tema.Escalon{Tam: tema.TextoBase.Tam, Linea: 1.5 * tema.Rem}

// El driver de los guardianes es de escritorio; se puede simular la misma consulta
// de puntero que usa el compositor real sin cambiar los otros botones de la suite.
var tactilEnCompositor = ui.EsTactil

func ladoBotonesCompositor() float32 {
	if tactilEnCompositor() {
		return max(LadoBotonBarra, ui.SueloTactilBarra)
	}
	return LadoBotonBarra
}

// Compositor es la caja de escribir del canal.
type Compositor struct {
	widget.BaseWidget

	T      *i18n.Translator
	Locale i18n.Locale

	// NombreCanal va en la marca de agua («Escribe en #equipo»).
	NombreCanal string
	// MarcaDeAgua y Rotulo sustituyen a las del canal: son las del compositor del
	// hilo (`thread.replyPlaceholder` y `thread.replyAria`).
	MarcaDeAgua, Rotulo string
	// Sencillo es el compositor SIN `advanced` (el del hilo): ni prioridad ni
	// programar, y por eso tampoco el separador que los aparta.
	Sencillo bool
	// SinFormato quita explícitamente el «Aa». El ancho por sí solo no lo oculta:
	// en el teléfono también se alcanza desplazando la tira de acciones.
	SinFormato bool
	// Inerte apaga la caja entera: sin conexión, o sin permiso de escritura. La web
	// baja además la opacidad al 60 %.
	Inerte bool
	// FormatoVisible es el `aria-pressed` del botón «Aa».
	FormatoVisible bool
	// Prioridad, Acuse y ProgramarA son las opciones del envío (`ComposeOptions`): se
	// eligen en el «!» y en el reloj, se ven como fichas sobre la caja y se vacían al
	// enviar. Con ProgramarA puesto el botón dice «Programar» y el mensaje no sale ya.
	Prioridad  Prioridad
	Acuse      bool
	ProgramarA time.Time
	// Ahora es el reloj de los atajos de «Enviar más tarde»; los guardianes lo congelan.
	Ahora func() time.Time

	// Miembros son los del canal, para el autocompletado de «@»; Remotos, los comandos
	// de servidor de la organización, para el de «/».
	Miembros []Miembro
	Remotos  []api.RoomsCommand

	AlEnviar func(string)
	// AlComando despacha un comando integrado CON EFECTO (`onCommand`: presencia, tema,
	// directo…) y AlComandoRemoto uno de servidor (`onRemoteCommand`). Sin ellos, el
	// comando avisa «no disponible aquí» y no se envía, que es el compositor del hilo.
	AlComando       func(nombre, arg string)
	AlComandoRemoto func(nombre, arg string)
	// AlCambiarTexto recibe el texto en cada cambio: el borrador y `onTyping`.
	AlCambiarTexto func(string)
	// AlMas abre el menú «+» (`composer.messageActions`) colgando de su botón.
	AlMas   func(disparador fyne.CanvasObject)
	AlEmoji func()
	// AlFormato avisa de que la barra de formato se enseña o se esconde, para guardarlo.
	AlFormato func(visible bool)
	// AlCambiarAlto avisa a quien la contiene de que la franja ha cambiado de alto:
	// han salido o se han ido las fichas de las opciones.
	AlCambiarAlto func()

	// inicial es lo puesto con PonerTexto antes de que exista el área (un borrador en
	// la caja de un hilo recién abierto); emoji, el botón del que cuelga su selector.
	inicial string
	emoji   *ui.BotonIcono

	area      *ui.EditorPlano
	caja      *ui.Caja
	sombra    *ui.Sombra
	filete    *canvas.Rectangle
	pelo      *canvas.Rectangle
	botones   []fyne.CanvasObject
	barraLado *ui.TiraDeslizable
	enviar    *ui.Boton
	objetos   []fyne.CanvasObject
	hayTexto  bool
	// objFichas son las fichas de las opciones activas; emergente, el panel abierto del
	// «!» o del reloj.
	objFichas []fyne.CanvasObject
	emergente *ui.Emergente
	// sug es la lista del autocompletado y disparo/consulta lo que se completa;
	// insertando calla al autocompletado mientras la caja se escribe a sí misma.
	sug        *ListaSugerencias
	disparo    TipoDisparador
	consulta   string
	insertando bool
	// Pendientes son los adjuntos de la caja (`pending`): subiéndose, subidos o
	// fallidos; objPendientes, sus fichas, y anchoPendientes el ancho con el que se
	// parten en líneas.
	Pendientes      []Pendiente
	objPendientes   []fyne.CanvasObject
	anchoPendientes float32
	// aviso es `cmdNotice`: el comando que no existe o no está disponible aquí.
	aviso    string
	objAviso []fyne.CanvasObject
	formatos map[fyne.CanvasObject]bool
}

var _ fyne.Widget = (*Compositor)(nil)

// NuevoCompositor crea la caja con su traductor.
func NuevoCompositor(t *i18n.Translator, l i18n.Locale) *Compositor {
	c := &Compositor{T: t, Locale: l}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el `aria-label` del área de texto.
func (c *Compositor) AccessibilityLabel() string {
	if c.Rotulo != "" {
		return c.Rotulo
	}
	return c.T.T("composer.writeInAria", i18n.Vars{"channel": c.NombreCanal})
}

// AccessibilityRole implementa fyne.Accessible.
func (c *Compositor) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// Texto es lo escrito.
func (c *Compositor) Texto() string {
	if c.area == nil {
		return c.inicial
	}
	return c.area.Texto()
}

// marcaDeAgua es el `placeholder`: la del hilo si la hay, y si no «Escribe en #canal».
func (c *Compositor) marcaDeAgua() string {
	if c.MarcaDeAgua != "" {
		return c.MarcaDeAgua
	}
	return c.T.T("composer.writeIn", i18n.Vars{"channel": c.NombreCanal})
}

// tecla es el `onKeyDown` del `textarea`: Intro envía y Mayús+Intro parte la línea,
// que es lo que hace cualquier chat. Con la caja vacía Intro no hace nada —no envía un
// mensaje en blanco ni mete una línea vacía—.
func (c *Compositor) tecla(k *fyne.KeyEvent) bool {
	if c.SugerenciasAbiertas() {
		switch k.Name {
		case fyne.KeyDown:
			c.sug.Mover(1)
			return true
		case fyne.KeyUp:
			c.sug.Mover(-1)
			return true
		case fyne.KeyReturn, fyne.KeyEnter, fyne.KeyTab:
			c.Elegir(c.sug.Items[min(c.sug.Elegida, len(c.sug.Items)-1)])
			return true
		case fyne.KeyEscape:
			c.cerrarSugerencias()
			return true
		default:
		}
	}
	if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter || c.area.Mayus() {
		return false
	}
	c.Someter()
	return true
}

// Someter es `submit`: un comando se DESPACHA en vez de enviarse —uno que no existe
// avisa y no sale—; «/help» abre la lista de comandos, «/shrug» y «/me» se envían
// transformados; lo demás sale tal cual.
func (c *Compositor) Someter() {
	s := c.Texto()
	if (strings.TrimSpace(s) == "" && len(c.Subidos()) == 0) || c.subiendo() || c.Inerte || c.AlEnviar == nil {
		return
	}
	c.PonerAvisoComando("")
	tipo, nombre, arg := ParsearComando(s, c.Remotos)
	disponible := func(f func(string, string)) {
		if f == nil {
			c.PonerAvisoComando(c.T.T("command.unavailable", i18n.Vars{"name": nombre}))
			return
		}
		f(nombre, strings.TrimSpace(arg))
		c.Vaciar()
	}
	switch tipo {
	case ComandoDesconocido:
		c.PonerAvisoComando(c.T.T("command.unknown", i18n.Vars{"name": nombre}))
		return
	case ComandoRemoto:
		disponible(c.AlComandoRemoto)
		return
	case ComandoIntegrado:
		switch nombre {
		case "help":
			c.PonerTexto("/")
			c.recalcular()
			return
		case "shrug":
			s = Encoger(arg)
		case "me":
			s = Emote(arg)
		default:
			disponible(c.AlComando)
			return
		}
	case NoEsComando:
	}
	c.AlEnviar(s)
}

// PonerPendientes cambia los adjuntos de la caja: sus fichas, y el botón de enviar, que
// se enciende con uno subido aunque no haya texto y se apaga mientras alguno sube.
func (c *Compositor) PonerPendientes(ps []Pendiente) {
	antes := c.altoArriba()
	c.Pendientes = ps
	c.anchoPendientes = c.Size().Width - 2*MargenCompositor
	if c.anchoPendientes <= 0 {
		c.anchoPendientes = 1 << 20 // sin tamaño aún: una sola línea
	}
	c.objPendientes = c.objPendientes[:0]
	for _, p := range ps {
		c.objPendientes = append(c.objPendientes, NuevaFichaPendiente(c.T, p, func() {
			if p.cancelar != nil {
				p.cancelar()
			}
			c.PonerPendientes(slices.DeleteFunc(slices.Clone(c.Pendientes), func(x Pendiente) bool { return x.Local == p.Local }))
		}))
	}
	c.cambiaArriba(antes)
}

// Subidos son los adjuntos ya subidos, los que viajan al enviar.
func (c *Compositor) Subidos() []api.RoomsAttachment {
	var out []api.RoomsAttachment
	for _, p := range c.Pendientes {
		if p.Subido != nil {
			out = append(out, *p.Subido)
		}
	}
	return out
}

// subiendo dice si alguno está todavía de camino.
func (c *Compositor) subiendo() bool {
	return slices.ContainsFunc(c.Pendientes, func(p Pendiente) bool { return p.Subido == nil && !p.Fallido })
}

// puedeEnviar es `canSend`: con texto o un adjunto subido, y sin nada subiendo.
func (c *Compositor) puedeEnviar() bool {
	return !c.Inerte && !c.subiendo() && (c.hayTexto || len(c.Subidos()) > 0)
}

// PonerAvisoComando enseña (o quita, con "") el aviso de un comando sobre la caja.
func (c *Compositor) PonerAvisoComando(s string) {
	if c.aviso == s {
		return
	}
	antes := c.altoArriba()
	c.aviso = s
	c.objAviso = nil
	if s != "" {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		c.objAviso = []fyne.CanvasObject{
			iconos.Trazo(iconos.Aviso, iconos.TallaXXS, "danger-text", v).Objeto(),
			ui.Texto(s, tema.TextoXS, ui.Normal, tema.Color("danger-text", v)),
		}
	}
	c.cambiaArriba(antes)
}

// ---- El autocompletado ----

// SugerenciasAbiertas dice si la lista está a la vista.
func (c *Compositor) SugerenciasAbiertas() bool { return c.sug != nil && len(c.sug.Items) > 0 }

// recalcular es `refreshTrigger`: qué se completa según lo que hay pegado al cursor.
func (c *Compositor) recalcular() {
	if c.area == nil || c.insertando {
		return
	}
	c.disparo, c.consulta = Disparador(c.area.Texto(), c.area.CursorTextOffset())
	items := SugerenciasPara(c.disparo, c.consulta, c.Miembros, c.Remotos)
	if c.sug == nil {
		if len(items) == 0 {
			return
		}
		c.sug = NuevaListaSugerencias(c.T)
		c.sug.AlElegir = c.Elegir
	}
	c.sug.Items, c.sug.Elegida = items, 0
	c.sug.Rotulo = c.T.T(map[TipoDisparador]string{
		DisparaComando: "composer.commands", DisparaEmoji: "composer.emojiToInsert",
	}[c.disparo], nil)
	if c.disparo == DisparaMencion {
		c.sug.Rotulo = c.T.T("composer.mentionSuggestions", nil)
	}
	c.sug.Ancha = c.Size().Width >= ui.CorteSM
	c.sug.Refresh()
	c.montarObjetos()
	c.Refresh()
}

func (c *Compositor) cerrarSugerencias() {
	if c.SugerenciasAbiertas() {
		c.sug.Items = nil
		c.montarObjetos()
		c.Refresh()
	}
}

// Elegir es `pick`: cambia lo tecleado tras el cursor por la sugerencia, como si se
// escribiera, y cierra la lista. Un comando deja la caja en «/nombre », lista para el
// argumento.
func (c *Compositor) Elegir(s Sugerencia) {
	borrar, escribir := Reemplazo(s, c.consulta)
	c.insertando = true
	if borrar < 0 {
		c.area.PonerTexto(escribir)
	} else {
		for range borrar {
			c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
		}
		for _, r := range escribir {
			c.area.TypedRune(r)
		}
	}
	c.insertando = false
	c.cerrarSugerencias()
	c.enfocar()
	if c.AlCambiarTexto != nil {
		c.AlCambiarTexto(c.area.Texto())
	}
}

func (c *Compositor) enfocar() {
	if lienzo := fyne.CurrentApp().Driver().CanvasForObject(c); lienzo != nil {
		lienzo.Focus(c.area)
	}
}

// PonerTexto devuelve un texto a la caja: es lo que hace un envío que falla, para no
// perder lo escrito.
func (c *Compositor) PonerTexto(s string) {
	c.inicial = s // si aún no hay área, lo escribe al crearla
	if c.area != nil {
		c.area.PonerTexto(s)
	}
	c.hayTexto = s != ""
	c.Refresh()
}

// Vaciar deja la caja limpia, que es lo que hace un envío (`resetAfterSend`): el texto
// y también las opciones.
func (c *Compositor) Vaciar() {
	c.inicial = ""
	if c.area != nil {
		c.area.PonerTexto("")
	}
	c.hayTexto = false
	c.QuitarOpciones()
	if len(c.Pendientes) > 0 {
		c.PonerPendientes(nil)
	}
	c.Refresh()
}

func (c *Compositor) ahora() time.Time {
	if c.Ahora != nil {
		return c.Ahora()
	}
	return time.Now()
}

// PonerPrioridad elige la prioridad del envío (Normal la quita).
func (c *Compositor) PonerPrioridad(p Prioridad) { c.Prioridad = p; c.opcionesCambian() }

// PonerAcuse pide, o no, acuse de recibo.
func (c *Compositor) PonerAcuse(si bool) { c.Acuse = si; c.opcionesCambian() }

// PonerProgramado programa el envío para ese instante; el cero lo deja en «enviar ya».
func (c *Compositor) PonerProgramado(t time.Time) { c.ProgramarA = t; c.opcionesCambian() }

// QuitarOpciones vuelve a un envío normal, ya y sin acuse.
func (c *Compositor) QuitarOpciones() {
	if c.Prioridad == Normal && !c.Acuse && c.ProgramarA.IsZero() {
		return
	}
	c.Prioridad, c.Acuse, c.ProgramarA = Normal, false, time.Time{}
	c.opcionesCambian()
}

// opcionesCambian rehace las fichas y, si con ellas cambia el alto de la franja, avisa
// a quien la contiene.
func (c *Compositor) opcionesCambian() {
	antes := c.altoArriba()
	c.objFichas = c.fichas()
	c.cambiaArriba(antes)
}

// BotonEmoji es el botón del emoji, del que cuelga su selector.
func (c *Compositor) BotonEmoji() fyne.CanvasObject { return c.emoji }

// Insertar es `insertGlyph`: escribe en el cursor, como si se tecleara, y devuelve el
// foco a la caja.
func (c *Compositor) Insertar(s string) {
	if c.area == nil {
		c.PonerTexto(c.inicial + s)
		return
	}
	for _, r := range s {
		c.area.TypedRune(r)
	}
	c.enfocar()
}

// AltoCompositor es lo que ocupa la caja con el área a su alto mínimo. Es la única
// cuenta: 1 + 3,5 + 35 + 1,75 + 28 + 5,25 + 1 = **75,5**.
func AltoCompositor(altoArea float32) float32 {
	return 2*tema.Borde + ArribaCompositor + max(altoArea, AltoAreaMinimo) +
		ArribaBarra + ladoBotonesCompositor() + AbajoCompositor
}

// AnchoAreaCompositor es lo que mide la CAJA del área de texto dentro de una caja de
// `anchoCaja`: la web le pone `box-sizing: border-box`, así que sus 979,5 medidos
// incluyen su propio `px-2.5`. Con los 992 de la caja del canal da exactamente eso.
func AnchoAreaCompositor(anchoCaja float32) float32 {
	return max(anchoCaja-2*tema.Borde-2*CanalonCompositor, 0)
}

// MedidaCompositor es la franja entera: el filete de arriba, el `pt-3`, la caja y el
// `pb-3`. 1 + 10,5 + 75,5 + 10,5 = **97,5**. De ancho, lo que le den (`px-4` dentro).
func MedidaCompositor() fyne.Size {
	return fyne.NewSize(0, tema.Borde+ArribaFranja+AltoCompositor(AltoAreaMinimo)+AbajoFranja)
}

// Medida es la franja con lo que tenga ENCIMA de la caja: las fichas de las opciones
// activas suman su fila y su `mb-2`.
func (c *Compositor) Medida() fyne.Size {
	m := MedidaCompositor()
	m.Height += c.altoArriba() + c.altoBarra() - ladoBotonesCompositor()
	return m
}

// altoBarra mide los controles que realmente se pintan, incluido Enviar. Un
// control táctil de 44 no puede vivir en una franja reservada para 32.
func (c *Compositor) altoBarra() float32 {
	alto := ladoBotonesCompositor()
	for _, b := range c.botones {
		alto = max(alto, b.MinSize().Height)
	}
	if c.enviar != nil {
		alto = max(alto, c.enviar.MinSize().Height)
	}
	return alto
}

// altoArriba es lo que va ENTRE el filete y la caja: el aviso de un comando y las
// fichas de las opciones, cada uno con su `mb-2`.
func (c *Compositor) altoArriba() float32 {
	alto := c.altoFichas()
	if len(c.objPendientes) > 0 {
		alto += colocarEnvuelto(c.objPendientes, 0, 0, c.anchoPendientes, HuecoFichas, false) + AireFichas
	}
	if c.aviso != "" {
		alto += tema.TextoXS.Linea + AireFichas
	}
	return alto
}

// cambiaArriba rehace lo que se pinta y avisa si la franja cambió de alto.
func (c *Compositor) cambiaArriba(antes float32) {
	if c.caja != nil {
		c.montarObjetos()
		c.Refresh()
	}
	if c.altoArriba() != antes && c.AlCambiarAlto != nil {
		c.AlCambiarAlto()
	}
}

// CreateRenderer compone la caja, el área y la barra.
func (c *Compositor) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())

	c.sombra = ui.NuevaSombra(tema.SombraSM, RadioCompositor)
	c.caja = ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), RadioCompositor)
	c.filete = canvas.NewRectangle(tema.Color("border", v))
	c.pelo = canvas.NewRectangle(tema.Color("border", v))
	c.pelo.SetMinSize(fyne.NewSize(tema.Borde, AltoSeparadorBarra))

	c.area = ui.NuevoEditorPlano(escalonCompositor, func(bool) { c.Refresh() })
	c.area.MultiLine = true
	c.area.Wrapping = fyne.TextWrapWord
	c.area.Scroll = fyne.ScrollVerticalOnly
	c.area.SetPlaceHolder(c.marcaDeAgua())
	c.area.PonerTexto(c.inicial) // antes del gancho: no es teclear
	c.area.AlCambiar(func(s string) {
		hay := s != ""
		if hay != c.hayTexto {
			c.hayTexto = hay
			c.Refresh() // el botón de enviar se enciende
		}
		if c.insertando {
			return
		}
		c.PonerAvisoComando("")
		c.recalcular()
		if c.AlCambiarTexto != nil {
			c.AlCambiarTexto(s)
		}
	})
	c.area.Tecla = c.tecla
	c.area.Atajo = c.atajo
	c.area.SoloLectura(c.Inerte)

	c.enviar = c.botonEnviar()
	c.montarBarra()

	r := &compositorRenderer{c: c}
	// Regla 7 de `ui.go`: sin esto la MARCA DE AGUA no se pinta —`widget.Entry` solo
	// la materializa en su propio Refresh— y la caja nace sin decir dónde se escribe.
	r.Refresh()
	return r
}

// montarBarra (re)hace los botones y su tira desplazable: cuáles hay depende del
// formato y de si es el compositor sencillo del hilo.
func (c *Compositor) montarBarra() {
	c.botones = c.barra()
	var acciones []fyne.CanvasObject
	for i := 0; i < len(c.botones); i++ {
		if i == c.indicePelo() {
			acciones = append(acciones, container.New(ui.Margen{Izq: MargenSeparadorBarra, Der: MargenSeparadorBarra}, c.pelo))
		}
		acciones = append(acciones, c.botones[i])
	}
	c.barraLado = ui.NuevaTiraDeslizable(ladoBotonesCompositor(), HuecoBarra, acciones...)
	c.barraLado.AnchoPropio = true
	c.barraLado.HuecoEntre = func(anterior, actual fyne.CanvasObject) float32 {
		if c.formatos[anterior] && c.formatos[actual] {
			return HuecoFormato
		}
		return HuecoBarra
	}
	c.montarObjetos()
}

// montarObjetos rehace la lista de lo que se pinta con las piezas que ya hay: las
// fichas cambian sin que cambie la barra, y rehacer sus botones dejaría el panel del
// «!» colgando de uno que ya no está en la ventana.
func (c *Compositor) montarObjetos() {
	c.objetos = []fyne.CanvasObject{c.filete}
	c.objetos = append(c.objetos, c.objPendientes...)
	c.objetos = append(c.objetos, c.objAviso...)
	c.objetos = append(c.objetos, c.objFichas...)
	c.objetos = append(c.objetos, c.sombra.Objeto())
	c.objetos = append(c.objetos, c.caja.Objetos()...)
	c.objetos = append(c.objetos, c.area.Objeto(), c.barraLado, c.enviar)
	// La lista del autocompletado, la ÚLTIMA: sale por encima de la franja y se pinta
	// sobre el historial, como el `absolute bottom-full` de la web.
	if c.SugerenciasAbiertas() {
		c.objetos = append(c.objetos, c.sug)
	}
}

// cuantosBotones son los que lleva la barra: añadir y emoji siempre, el «Aa» salvo
// SinFormato y prioridad y programar fuera del compositor sencillo.
func (c *Compositor) cuantosBotones() int {
	n := 2
	if !c.SinFormato {
		n++
		if c.FormatoVisible {
			n += len(marcasDeBarra)
		}
	}
	if !c.Sencillo {
		n += 2
	}
	return n
}

// indicePelo es ante qué botón de la barra va el separador: tras «añadir», «emoji» y
// «formato», que AÑADEN algo, y antes de «prioridad» y «programar», que dicen CÓMO se
// envía. Sin esos dos (el compositor sencillo) no hay separador: −1.
func (c *Compositor) indicePelo() int {
	if c.Sencillo {
		return -1
	}
	return len(c.botones) - 2
}

// barra son los controles de debajo del texto, en el orden del DOM: añadir, emoji,
// formato, prioridad y programar. Prioridad y programar abren su panel aquí mismo: lo
// que eligen es estado de la caja, como en la web.
func (c *Compositor) barra() []fyne.CanvasObject {
	t := c.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	nuevo := func(cuerpo, rotulo string, pulsado bool, al func()) *ui.BotonIcono {
		b := ui.NuevoBotonIcono(iconos.Ninguno, rotulo, al)
		b.Glifo = dibujo(cuerpo, GlifoBarra, "text-muted", v)
		b.Lado, b.TamIcono, b.Radio = ladoBotonesCompositor(), GlifoBarra, tema.Forma("radius-control")
		b.Pulsado = pulsado
		b.Inerte = c.Inerte
		return b
	}
	var mas *ui.BotonIcono
	mas = ui.NuevoBotonIcono(iconos.Mas, t.T("composer.addToMessage", nil), func() {
		if c.AlMas != nil {
			c.AlMas(mas)
		}
	})
	mas.Lado, mas.TamIcono, mas.Radio = ladoBotonesCompositor(), GlifoBarra, tema.Forma("radius-control")
	mas.Inerte = c.Inerte
	texto := ui.NuevoBotonIcono(iconos.Texto, t.T("composer.format", nil), c.AlternarFormato)
	texto.Lado, texto.TamIcono, texto.Radio = ladoBotonesCompositor(), GlifoBarra, tema.Forma("radius-control")
	texto.Pulsado, texto.Inerte = c.FormatoVisible, c.Inerte
	c.emoji = nuevo(glifoEmoji, t.T("composer.insertEmoji", nil), false, func() {
		if c.AlEmoji != nil {
			c.AlEmoji()
		}
	})
	out := []fyne.CanvasObject{mas, c.emoji}
	c.formatos = map[fyne.CanvasObject]bool{}
	if !c.SinFormato {
		out = append(out, texto)
		if c.FormatoVisible {
			for _, m := range marcasDeBarra {
				b := ui.NuevoBotonIcono(m.icono, t.T(m.clave, nil), func() { c.AplicarMarca(m.marca) })
				b.Lado, b.TamIcono, b.Radio = ladoBotonesCompositor(), GlifoFormato, tema.Forma("radius-control")
				b.Inerte = c.Inerte
				c.formatos[b] = true
				out = append(out, b)
			}
		}
	}
	if c.Sencillo {
		return out
	}
	var prio, reloj *ui.BotonIcono
	prio = nuevo(glifoPrioridad, t.T("composer.priorityAndAck", nil), false, func() { c.abrirPrioridad(prio) })
	reloj = nuevo(glifoReloj, t.T("composer.sendLater", nil), false, func() { c.abrirProgramar(reloj) })
	return append(out, prio, reloj)
}

// botonEnviar es el único control CON CAJA de toda la barra, y por eso se lee como la
// acción principal sin que nadie lo explique.
//
// Es `ui.Boton` con `SoloIcono` y no `ui.BotonIcono`, que es lo que dice la regla de
// `ui/botonicono.go`: una acción PRINCIPAL que resulta que no lleva texto. Lo mismo
// que escribe la web (`<Button variant=… size="sm" iconOnly>`).
//
// `ghost` mientras no hay nada que enviar: un relleno de acento encendido en cada canal
// y en cada hilo con la caja vacía es lo más ruidoso del panel. Se enciende al haber
// texto, y la talla no cambia, así que no salta nada.
func (c *Compositor) botonEnviar() *ui.Boton {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := ui.NuevoBoton("", c.Someter).Con(c.varianteEnviar(), ui.TallaSM)
	b.SoloIcono, b.TamIcono = true, GlifoBarra
	b.Rotulo = c.rotuloEnviar()
	b.Glifo = dibujo(glifoEnviar, GlifoBarra, c.tintaEnviar(), v)
	b.Inerte = !c.puedeEnviar()
	return b
}

// rotuloEnviar es «Programar» con un envío programado y «Enviar» sin él.
func (c *Compositor) rotuloEnviar() string {
	if !c.ProgramarA.IsZero() {
		return c.T.T("composer.schedule", nil)
	}
	return c.T.T("composer.send", nil)
}

func (c *Compositor) varianteEnviar() ui.Variante {
	if c.puedeEnviar() {
		return ui.Primario
	}
	return ui.Fantasma
}

func (c *Compositor) tintaEnviar() string {
	if c.puedeEnviar() {
		return "on-primary"
	}
	return "text-muted"
}

type compositorRenderer struct{ c *Compositor }

func (r *compositorRenderer) MinSize() fyne.Size           { return r.c.Medida() }
func (r *compositorRenderer) Objects() []fyne.CanvasObject { return r.c.objetos }
func (r *compositorRenderer) Destroy()                     {}

// Layout pone el filete arriba, la caja dentro de su `px-4 pt-3 pb-3`, el área encima
// y la barra debajo.
func (r *compositorRenderer) Layout(t fyne.Size) {
	c := r.c
	c.filete.Move(fyne.NewPos(0, 0))
	c.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	x0, y0 := MargenCompositor, tema.Borde+ArribaFranja
	if c.SugerenciasAbiertas() {
		alto := MedidaListaSugerencias(len(c.sug.Items))
		c.sug.Move(fyne.NewPos(x0, -AireSugerencias-alto))
		c.sug.Resize(fyne.NewSize(max(t.Width-2*x0, 0), alto))
	}
	// Los adjuntos, en su fila que parte línea (`mb-2 flex flex-wrap gap-2`).
	if len(c.objPendientes) > 0 {
		y0 += colocarEnvuelto(c.objPendientes, x0, y0, c.anchoPendientes, HuecoFichas, true) + AireFichas
	}
	// El aviso de un comando: `mb-2 flex items-center gap-1.5 text-xs`.
	if len(c.objAviso) == 2 {
		g := c.objAviso[0]
		g.Move(fyne.NewPos(x0, y0+(tema.TextoXS.Linea-iconos.TallaXXS)/2))
		g.Resize(fyne.NewSize(iconos.TallaXXS, iconos.TallaXXS))
		ui.ColocarTexto(c.objAviso[1].(*canvas.Text), tema.TextoXS, x0+iconos.TallaXXS+tema.Espacio(1.5), y0) //nolint:errcheck,forcetypeassert // El aviso se construye siempre con un canvas.Text en la segunda posición.
		y0 += tema.TextoXS.Linea + AireFichas
	}
	// Las fichas, en su fila sobre la caja (`mb-2 flex gap-2`).
	fx := x0
	for _, f := range c.objFichas {
		m := f.MinSize()
		f.Move(fyne.NewPos(fx, y0))
		f.Resize(m)
		fx += m.Width + HuecoFichas
	}
	y0 += c.altoFichas()
	ancho := max(t.Width-2*MargenCompositor, 0)
	alto := max(t.Height-y0-AbajoFranja, 0)

	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.sombra.Ajustar(fyne.NewPos(x0, y0), fyne.NewSize(ancho, alto), v)
	ui.MoverCaja(c.caja, fyne.NewPos(x0, y0), fyne.NewSize(ancho, alto))

	dentro := max(ancho-2*tema.Borde-2*CanalonCompositor, 0)
	x := x0 + tema.Borde + CanalonCompositor
	y := y0 + tema.Borde + ArribaCompositor

	altoBarra := c.altoBarra()
	altoArea := max(alto-2*tema.Borde-ArribaCompositor-ArribaBarra-altoBarra-AbajoCompositor,
		AltoAreaMinimo)
	c.area.Colocar(fyne.NewPos(x+CanalonArea, y+ArribaArea),
		fyne.NewSize(max(dentro-2*CanalonArea, 0), max(altoArea-ArribaArea-AbajoArea, 0)))
	y += altoArea + ArribaBarra

	// Enviar tiene su hueco fijo. La tira de acciones recibe SOLO lo que queda y
	// se desplaza por dentro cuando el canal estrecha entre sus dos laterales.
	m := c.enviar.MinSize()
	m.Width = max(m.Width, ladoBotonesCompositor())
	m.Height = max(m.Height, ladoBotonesCompositor())
	derecha := x + dentro - CanalonBarra - m.Width
	c.enviar.Move(fyne.NewPos(derecha, y+(altoBarra-m.Height)/2))
	c.enviar.Resize(m)
	c.barraLado.Move(fyne.NewPos(x+CanalonBarra, y))
	c.barraLado.Resize(fyne.NewSize(max(derecha-HuecoBarra-x-CanalonBarra, 0), altoBarra))
	c.barraLado.Refresh()
}

// Refresh repinta la caja y rehace la barra: encender el botón de enviar cambia su
// variante, no su tamaño, así que Fyne no llamaría a Layout por su cuenta.
func (r *compositorRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, borde := tema.Color("surface", v), tema.Color("border", v)
	if c.Inerte {
		// `opacity-60` de la web sobre la caja entera.
		relleno.A = uint8(float32(relleno.A) * 0.6)
		borde.A = uint8(float32(borde.A) * 0.6)
	}
	c.caja.Pintar(relleno, borde)
	for _, f := range []*canvas.Rectangle{c.filete, c.pelo} {
		f.FillColor = tema.Color("border", v)
		f.Refresh()
	}
	if len(c.botones) != c.cuantosBotones() {
		c.montarBarra()
	}
	c.enviar.Variante, c.enviar.Rotulo = c.varianteEnviar(), c.rotuloEnviar()
	c.enviar.Glifo.Tenir(c.tintaEnviar(), v)
	c.enviar.Inerte = !c.puedeEnviar()
	c.enviar.Refresh()
	c.area.SetPlaceHolder(c.marcaDeAgua())
	c.area.SoloLectura(c.Inerte)
	c.area.Objeto().Refresh()
	r.Layout(c.Size())
}
