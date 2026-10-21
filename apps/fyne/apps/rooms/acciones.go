// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Contrapartida de MessageView.tsx (la barra de acciones de un mensaje y la edición
// en línea) y MessageMenu.tsx (el «…») dentro de la pantalla de canal declarada en
// pantalla.go.
//
// La barra sale al pasar por encima del mensaje y, con el dedo, al tocarlo:
// `absolute -top-3.5 right-2 flex items-center gap-0.5 rounded-md border p-0.5
// shadow-sm sytue-popover-surface`, con las cuatro reacciones rápidas,
// «Responder en un hilo» y el «…». Cada botón es `h-7 w-7 rounded` con
// `hover:bg-bg-subtle`. HOJA.
var (
	// LadoBotonMensaje es el `h-7 w-7` de cada botón de la barra: 24,5.
	LadoBotonMensaje = tema.Espacio(7)
	// RellenoBarraMensaje es su `p-0.5` y HuecoBarraMensaje su `gap-0.5`.
	RellenoBarraMensaje = tema.Espacio(0.5)
	HuecoBarraMensaje   = tema.Espacio(0.5)
	// ArribaBarraMensaje es el `-top-3.5` y DerechaBarraMensaje el `right-2`.
	ArribaBarraMensaje  = tema.Espacio(3.5)
	DerechaBarraMensaje = tema.Espacio(2)
	// GlifoBarraMensaje es el `size={16}` de sus glifos.
	GlifoBarraMensaje = iconos.TallaSM
	// AnchoMenuMensaje es `MENU_W` de MessageMenu.tsx.
	AnchoMenuMensaje float32 = 232
)

// ReaccionesRapidas es `QUICK_REACTIONS`.
var ReaccionesRapidas = []string{"👍", "❤️", "😄", "🎉"}

// glifoResponder es la burbuja de «Responder en un hilo», que la web escribe en línea
// dentro de MessageView.tsx.
const glifoResponder = `<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />`

// BarraMensaje es la barra de acciones de un mensaje.
//
// # Por qué la barra lleva el `hover` y sus botones no
//
// Fyne entrega el `hover` SOLO al objeto de más arriba bajo el puntero. Si cada botón
// fuese `desktop.Hoverable`, pasar del mensaje a un botón le quitaría el `hover` al
// mensaje y la barra se escondería debajo del puntero. Aquí el `hover` es de la barra
// entera —que avisa al mensaje con `AlEncima`— y la barra resalta el botón que queda
// bajo el puntero con `MouseMoved`, que es lo que hace el `group-hover` de la web.
//
// # El teclado: una parada, y las flechas
//
// En la web la barra es un `Toolbar`: `opacity-0` sin el puntero encima —no
// `display:none`, para que siga en el recorrido del tabulador— y UNA sola parada de
// tabulación con las flechas para moverse dentro (roving tabindex). Aquí es igual: la
// barra está SIEMPRE en el árbol y es ella la que recibe el foco; «apagada» no pinta nada
// y no se deja pulsar (`pointer-events-none`), y con el foco se enciende, las flechas
// mueven el botón activo —con su anillo— e Intro o Espacio lo pulsan.
//
// ponytail: apagada sigue recibiendo el `hover` de su hueco, que sobresale 12,25 por
// encima de la fila (`-top-3.5`); pasar por ahí desde la fila de arriba enciende la de
// abajo. En la web `pointer-events-none` lo evita; aquí haría falta que la barra dejara
// pasar el `hover`, y Fyne no lo deja.
type BarraMensaje struct {
	widget.BaseWidget

	T *i18n.Translator
	// AlReaccionar recibe el emoji; nil, sin reacciones rápidas.
	AlReaccionar func(string)
	// AlEmojis abre el selector de emoji colgando de su botón (`EmojiPickerButton`, detrás
	// de las rápidas); nil, sin él.
	AlEmojis func(disparador fyne.CanvasObject)
	// AlHilo abre el hilo; nil, sin «Responder en un hilo».
	AlHilo func()
	// Menu construye las entradas del «…» al abrirlo; nil, sin menú.
	Menu func() []fyne.CanvasObject
	// AlEncima avisa al mensaje de que el puntero está sobre la barra.
	AlEncima func(bool)
	// AlFoco avisa al mensaje de que la barra tiene el foco (`focus-within`).
	AlFoco func(bool)
	// Apagada es `opacity-0 pointer-events-none`: está, pero no se ve ni se pulsa.
	Apagada bool

	// enfocada e indice son la parada de tabulación y el botón activo dentro.
	enfocada bool
	indice   int

	botones   []*botonMensaje
	resaltado int
	caja      *ui.Caja
	sombra    *ui.Sombra
	objetos   []fyne.CanvasObject
	menu      *ui.Menu
}

var (
	_ fyne.Widget       = (*BarraMensaje)(nil)
	_ desktop.Hoverable = (*BarraMensaje)(nil)
	_ fyne.Focusable    = (*BarraMensaje)(nil)
)

// FocusGained enciende la barra con el primer botón activo, que es lo que hace el
// roving tabindex al entrar.
func (b *BarraMensaje) FocusGained() {
	b.enfocada = true
	b.indice = min(b.indice, max(len(b.botones)-1, 0))
	if b.AlFoco != nil {
		b.AlFoco(true)
	}
	b.Refresh()
}

// FocusLost la suelta.
func (b *BarraMensaje) FocusLost() {
	b.enfocada = false
	if b.AlFoco != nil {
		b.AlFoco(false)
	}
	b.Refresh()
}

// TypedRune pulsa el activo con Espacio.
func (b *BarraMensaje) TypedRune(r rune) {
	if r == ' ' {
		b.pulsarActivo()
	}
}

// TypedKey mueve el activo con las flechas, Inicio y Fin, y lo pulsa con Intro.
func (b *BarraMensaje) TypedKey(k *fyne.KeyEvent) {
	if ui.SubirEscape(b, k) { // Escape y «atrás» son del diálogo de encima
		return
	}
	n := len(b.botones)
	if n == 0 {
		return
	}
	switch k.Name {
	case fyne.KeyRight, fyne.KeyDown:
		b.indice = (b.indice + 1) % n
	case fyne.KeyLeft, fyne.KeyUp:
		b.indice = (b.indice - 1 + n) % n
	case fyne.KeyHome:
		b.indice = 0
	case fyne.KeyEnd:
		b.indice = n - 1
	case fyne.KeyReturn, fyne.KeyEnter:
		b.pulsarActivo()
		return
	default:
		return
	}
	b.Refresh()
}

func (b *BarraMensaje) pulsarActivo() {
	if b.indice >= 0 && b.indice < len(b.botones) {
		b.botones[b.indice].Tapped(nil)
	}
}

// Enfocada dice si la barra tiene el foco.
func (b *BarraMensaje) Enfocada() bool { return b.enfocada }

// NuevaBarraMensaje crea la barra. Los ganchos se rellenan antes de montarla.
func NuevaBarraMensaje(t *i18n.Translator) *BarraMensaje {
	b := &BarraMensaje{T: t, resaltado: -1}
	b.ExtendBaseWidget(b)
	return b
}

// AccessibilityLabel es el `aria-label` del `role="toolbar"` y, con el foco, el del
// botón activo, que es lo que el lector anuncia al moverse con las flechas.
func (b *BarraMensaje) AccessibilityLabel() string {
	if b.enfocada && b.indice < len(b.botones) {
		return b.botones[b.indice].nombre
	}
	return b.T.T("composer.messageActions", nil)
}

// AccessibilityRole implementa fyne.Accessible.
func (b *BarraMensaje) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MedidaBarraMensaje es lo que ocupa con `n` botones. Es la única cuenta.
func MedidaBarraMensaje(n int) fyne.Size {
	lado := 2*tema.Borde + 2*RellenoBarraMensaje
	return fyne.NewSize(lado+float32(n)*LadoBotonMensaje+float32(max(n-1, 0))*HuecoBarraMensaje,
		lado+LadoBotonMensaje)
}

// CreateRenderer monta la caja con su sombra y los botones.
func (b *BarraMensaje) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := b.T
	b.botones = nil
	if b.AlReaccionar != nil {
		for _, e := range ReaccionesRapidas {
			b.botones = append(b.botones, nuevoBotonEmoji(e, t.T("reaction.with", i18n.Vars{"emoji": e}),
				func() { b.AlReaccionar(e) }))
		}
	}
	if b.AlEmojis != nil {
		var emo *botonMensaje
		emo = nuevoBotonGlifo(glifoEmoji, t.T("reaction.withAny", nil), func() { b.AlEmojis(emo) }, v)
		b.botones = append(b.botones, emo)
	}
	if b.AlHilo != nil {
		b.botones = append(b.botones, nuevoBotonGlifo(glifoResponder, t.T("message.replyInThread", nil), b.AlHilo, v))
	}
	if b.Menu != nil {
		var mas *botonMensaje
		mas = nuevoBotonGlifo(glifoMas, t.T("message.moreActions", nil), func() { b.abrirMenu(mas) }, v)
		b.botones = append(b.botones, mas)
	}
	b.sombra = ui.NuevaSombra(tema.SombraSM, tema.RadioMD)
	b.caja = ui.NuevaCaja(tema.Color("menu-surface", v), tema.Color("border", v), tema.RadioMD)
	b.objetos = append([]fyne.CanvasObject{b.sombra.Objeto()}, b.caja.Objetos()...)
	for _, x := range b.botones {
		b.objetos = append(b.objetos, x)
	}
	r := &barraMensajeRenderer{b: b}
	r.Refresh()
	return r
}

// abrirMenu despliega el «…» con las entradas de ahora: fijado, guardado y seguido
// cambian, así que se construyen al abrirlo y no al montar la barra. El panel mide los
// 232 fijos de la web (`MENU_W`).
func (b *BarraMensaje) abrirMenu(disparador fyne.CanvasObject) {
	b.menu = ui.NuevoMenu(b.Menu()...)
	b.menu.Rotulo = b.T.T("message.moreActions", nil)
	b.menu.Ancho = AnchoMenuMensaje
	b.menu.Mostrar(disparador)
}

// MouseIn avisa al mensaje: el puntero sigue «dentro» aunque esté sobre la barra.
func (b *BarraMensaje) MouseIn(e *desktop.MouseEvent) {
	if b.AlEncima != nil {
		b.AlEncima(true)
	}
	b.MouseMoved(e)
}

// MouseMoved resalta el botón que queda bajo el puntero.
func (b *BarraMensaje) MouseMoved(e *desktop.MouseEvent) {
	i := -1
	for j, x := range b.botones {
		p, s := x.Position(), x.Size()
		if e.Position.X >= p.X && e.Position.X < p.X+s.Width && e.Position.Y >= p.Y && e.Position.Y < p.Y+s.Height {
			i = j
		}
	}
	if i != b.resaltado {
		b.resaltado = i
		b.Refresh()
	}
}

// MouseOut apaga el resaltado y suelta al mensaje.
func (b *BarraMensaje) MouseOut() {
	b.resaltado = -1
	b.Refresh()
	if b.AlEncima != nil {
		b.AlEncima(false)
	}
}

type barraMensajeRenderer struct{ b *BarraMensaje }

func (r *barraMensajeRenderer) MinSize() fyne.Size {
	return MedidaBarraMensaje(len(r.b.botones))
}
func (r *barraMensajeRenderer) Objects() []fyne.CanvasObject { return r.b.objetos }
func (r *barraMensajeRenderer) Destroy()                     {}

func (r *barraMensajeRenderer) Layout(t fyne.Size) {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.sombra.Ajustar(fyne.NewPos(0, 0), t, v)
	// Apagada, la sombra también: Ajustar la vuelve a enseñar cuando la barra pasa de no
	// tener tamaño a tenerlo, y quedaba una caja gris junto a cada mensaje (visto en una
	// ventana real el 2026-09-27; el pintor de software no pinta sombras).
	if b.Apagada && !b.enfocada {
		b.sombra.Objeto().Hide()
	}
	b.caja.Redimensionar(t)
	x := tema.Borde + RellenoBarraMensaje
	for _, x2 := range b.botones {
		x2.Move(fyne.NewPos(x, tema.Borde+RellenoBarraMensaje))
		x2.Resize(fyne.NewSize(LadoBotonMensaje, LadoBotonMensaje))
		x += LadoBotonMensaje + HuecoBarraMensaje
	}
}

func (r *barraMensajeRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.caja.Pintar(tema.Color("menu-surface", v), tema.Color("border", v))
	visible := !b.Apagada || b.enfocada
	for _, o := range b.objetos {
		if visible {
			o.Show()
		} else {
			o.Hide()
		}
	}
	for i, x := range b.botones {
		x.resaltado = i == b.resaltado
		x.enfocado = b.enfocada && i == b.indice
		x.Refresh()
	}
	r.Layout(b.Size())
}

// botonMensaje es un botón de la barra: un emoji o un glifo en `h-7 w-7 rounded`.
// Pulsable pero NO `Hoverable` (ver BarraMensaje); su `hover` lo pinta la barra.
type botonMensaje struct {
	widget.BaseWidget

	nombre    string
	emoji     string
	glifo     *iconos.Dibujo
	alPulsar  func()
	resaltado bool
	// enfocado es el activo de la barra con el foco: lleva el anillo.
	enfocado bool

	caja  *ui.Caja
	aro   *ui.Aro
	texto *canvas.Text
}

var (
	_ fyne.Tappable   = (*botonMensaje)(nil)
	_ fyne.Accessible = (*botonMensaje)(nil)
)

func nuevoBotonEmoji(emoji, nombre string, al func()) *botonMensaje {
	b := &botonMensaje{nombre: nombre, emoji: emoji, alPulsar: al}
	b.ExtendBaseWidget(b)
	return b
}

func nuevoBotonGlifo(cuerpo, nombre string, al func(), v tema.Variante) *botonMensaje {
	b := &botonMensaje{nombre: nombre, glifo: dibujo(cuerpo, GlifoBarraMensaje, "text-muted", v), alPulsar: al}
	b.ExtendBaseWidget(b)
	return b
}

// Tapped ejecuta la acción.
func (b *botonMensaje) Tapped(*fyne.PointEvent) {
	if b.alPulsar != nil {
		b.alPulsar()
	}
}

// AccessibilityLabel es su `aria-label`.
func (b *botonMensaje) AccessibilityLabel() string { return b.nombre }

// AccessibilityRole implementa fyne.Accessible.
func (b *botonMensaje) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (b *botonMensaje) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.caja = ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM)
	b.aro = ui.NuevoAro(tema.RadioSM, ui.SinHueco, v)
	objetos := append(b.aro.Objetos(), b.caja.Objetos()...)
	if b.glifo != nil {
		objetos = append(objetos, b.glifo.Objeto())
	} else {
		b.texto = ui.Texto(b.emoji, tema.TextoSM, ui.Normal, tema.Color("text", v))
		objetos = append(objetos, b.texto)
	}
	r := &botonMensajeRenderer{b: b, objetos: objetos}
	r.Refresh()
	return r
}

type botonMensajeRenderer struct {
	b       *botonMensaje
	objetos []fyne.CanvasObject
}

func (r *botonMensajeRenderer) MinSize() fyne.Size {
	return fyne.NewSize(LadoBotonMensaje, LadoBotonMensaje)
}
func (r *botonMensajeRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonMensajeRenderer) Destroy()                     {}

func (r *botonMensajeRenderer) Layout(t fyne.Size) {
	b := r.b
	b.aro.Colocar(t)
	b.caja.Redimensionar(t)
	if b.glifo != nil {
		b.glifo.Colocar((t.Width-GlifoBarraMensaje)/2, (t.Height-GlifoBarraMensaje)/2, GlifoBarraMensaje)
		return
	}
	ancho := ui.Medir(b.emoji, tema.TextoSM, ui.Normal).Width
	ui.ColocarTexto(b.texto, tema.TextoSM, (t.Width-ancho)/2, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *botonMensajeRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := color.NRGBA{}, "text-muted"
	if b.resaltado {
		fondo, tinta = tema.Color("bg-subtle", v), "text"
	}
	b.caja.Pintar(fondo, color.NRGBA{})
	b.aro.Ver(b.enfocado)
	b.aro.Pintar(v)
	if b.glifo != nil {
		b.glifo.Tenir(tinta, v)
	}
	r.Layout(b.Size())
}

// ---- La edición en línea ----

// Medidas de la edición (MessageView.tsx): el `textarea` es `mt-0.5 w-full rounded-md
// border px-2.5 py-1.5 text-base` con `rows = min(6, líneas)`, y debajo `mt-1 flex
// items-center gap-2` con «Guardar» y «Cancelar» `sm` y la pista en `.sytue-meta`. HOJA.
var (
	CanalonEdicion    = tema.Espacio(2.5)
	RellenoEdicion    = tema.Espacio(1.5)
	AireBotonesEdita  = tema.Espacio(1)
	HuecoBotonesEdita = tema.Espacio(2)
	// FilasEdicionTope es el `Math.min(6, …)` de las filas.
	FilasEdicionTope = 6
)

// Edicion es la caja de editar un mensaje propio: Intro guarda, Mayús+Intro parte la
// línea y Escape cancela.
type Edicion struct {
	widget.BaseWidget

	T        *i18n.Translator
	Original string

	AlGuardar  func(string)
	AlCancelar func()

	area              *ui.EditorPlano
	aro               *ui.Aro
	caja              *ui.Caja
	guardar, cancelar *ui.Boton
	pista             *canvas.Text
	objetos           []fyne.CanvasObject
	enfocada          bool
}

var _ fyne.Widget = (*Edicion)(nil)

// NuevaEdicion crea la caja con el texto de ahora.
func NuevaEdicion(t *i18n.Translator, original string, alGuardar func(string), alCancelar func()) *Edicion {
	e := &Edicion{T: t, Original: original, AlGuardar: alGuardar, AlCancelar: alCancelar}
	e.ExtendBaseWidget(e)
	return e
}

// AccessibilityLabel es el `aria-label` del `textarea`.
func (e *Edicion) AccessibilityLabel() string { return e.T.T("message.edit", nil) }

// AccessibilityRole implementa fyne.Accessible.
func (e *Edicion) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// Texto es lo escrito.
func (e *Edicion) Texto() string {
	if e.area == nil {
		return e.Original
	}
	return e.area.Texto()
}

// filas es el `rows` del `textarea`: tantas como líneas tenga el texto, hasta seis.
func (e *Edicion) filas() int {
	n := len(partirEnLineas(e.Texto()))
	return max(1, min(FilasEdicionTope, n))
}

// MedidaEdicion es lo que ocupa con `filas` filas, sin el `mt-0.5` de encima. Es la
// única cuenta.
func MedidaEdicion(filas int) fyne.Size {
	caja := 2*tema.Borde + 2*RellenoEdicion + float32(filas)*tema.TextoBase.Linea
	return fyne.NewSize(0, caja+AireBotonesEdita+ui.MedidaBoton("", ui.TallaSM, 0).Height)
}

func (e *Edicion) guardarAhora() {
	if e.AlGuardar != nil {
		e.AlGuardar(e.Texto())
	}
}

// CreateRenderer compone la caja, el editor, los dos botones y la pista.
func (e *Edicion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := e.T
	e.aro = ui.NuevoAro(tema.RadioMD, ui.SinHueco, v)
	e.caja = ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	e.area = ui.NuevoEditorPlano(tema.TextoBase, func(si bool) {
		e.enfocada = si
		e.Refresh()
	})
	e.area.MultiLine = true
	e.area.Wrapping = fyne.TextWrapWord
	e.area.Scroll = fyne.ScrollVerticalOnly
	e.area.PonerTexto(e.Original)
	e.area.AlCambiar(func(string) { e.Refresh() })
	e.area.Tecla = func(k *fyne.KeyEvent) bool {
		switch {
		case (k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter) && !e.area.Mayus():
			e.guardarAhora()
			return true
		case k.Name == fyne.KeyEscape:
			if e.AlCancelar != nil {
				e.AlCancelar()
			}
			return true
		}
		return false
	}
	e.guardar = ui.NuevoBoton(t.T("action.save", nil), e.guardarAhora).Con(ui.Primario, ui.TallaSM)
	e.cancelar = ui.NuevoBoton(t.T("action.cancel", nil), e.AlCancelar).Con(ui.Secundario, ui.TallaSM)
	e.pista = ui.Texto(t.T("message.editHint", nil), escalonMeta, ui.Normal, tema.Color("text-subtle", v))
	e.objetos = append(e.aro.Objetos(), e.caja.Objetos()...)
	e.objetos = append(e.objetos, e.area.Objeto(), e.guardar, e.cancelar, e.pista)
	r := &edicionRenderer{e: e}
	r.Refresh()
	return r
}

// Enfocar pone el cursor en la caja, que es el `autofocus` de la web.
func (e *Edicion) Enfocar() {
	if c := fyne.CurrentApp().Driver().CanvasForObject(e); c != nil && e.area != nil {
		c.Focus(e.area)
	}
}

type edicionRenderer struct{ e *Edicion }

func (r *edicionRenderer) MinSize() fyne.Size           { return MedidaEdicion(r.e.filas()) }
func (r *edicionRenderer) Objects() []fyne.CanvasObject { return r.e.objetos }
func (r *edicionRenderer) Destroy()                     {}

func (r *edicionRenderer) Layout(t fyne.Size) {
	e := r.e
	alto := 2*tema.Borde + 2*RellenoEdicion + float32(e.filas())*tema.TextoBase.Linea
	caja := fyne.NewSize(t.Width, alto)
	e.aro.Colocar(caja)
	e.caja.Redimensionar(caja)
	e.area.Colocar(fyne.NewPos(tema.Borde+CanalonEdicion, tema.Borde+RellenoEdicion),
		fyne.NewSize(max(t.Width-2*(tema.Borde+CanalonEdicion), 0), float32(e.filas())*tema.TextoBase.Linea))
	y := alto + AireBotonesEdita
	x := float32(0)
	for _, b := range []*ui.Boton{e.guardar, e.cancelar} {
		m := b.MinSize()
		b.Move(fyne.NewPos(x, y))
		b.Resize(m)
		x += m.Width + HuecoBotonesEdita
	}
	alt := ui.MedidaBoton("", ui.TallaSM, 0).Height
	ui.ColocarTexto(e.pista, escalonMeta, x, y+(alt-escalonMeta.Linea)/2)
}

func (r *edicionRenderer) Refresh() {
	e := r.e
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	e.caja.Pintar(tema.Color("surface", v), tema.Color("border", v))
	e.aro.Ver(e.enfocada)
	e.aro.Pintar(v)
	e.pista.Color = tema.Color("text-subtle", v)
	e.pista.Refresh()
	r.Layout(e.Size())
}
