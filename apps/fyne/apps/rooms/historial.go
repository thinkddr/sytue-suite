// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"
	"time"

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

// El historial de mensajes: la columna que ocupa el hueco entre la cabecera y la caja
// de escribir.
//
// Medido en `rooms/canal.ancho.json` (1280×800, rem 14):
//
//	desplazador   992 de ancho (`min-h-0 flex-1 px-4`: 1020 − 28)
//	fila          **49** de alto de media, entre 22,75 y 100 según el mensaje
//	canalón       `px-2` (7) y `gap-3` (10,5) entre el avatar y el texto
//	sin agrupar   `mt-2 py-1` → 7 de aire y 3,5 arriba y abajo
//	agrupada      `py-0.5` → 1,75, sin avatar ni nombre
//	avatar        columna de `w-9` (31,5) con `pt-0.5`; el avatar `sm` mide 32
//	nombre        `text-sm` semi-negrita (11,38 / 15,75) · hora `.sytue-meta` (10,5 / 14)
//	cuerpo        `text-base` (12,25 / 19,25), `whitespace-pre-wrap`
//	día           `my-4` con píldora `px-3 py-0.5 text-xs` semi-negrita sobre `surface`
//
// # Por qué NO hay virtualizador
//
// Porque la web tampoco lo necesita: `HISTORY_CAP` en `ChatApp.tsx` recorta el
// histórico a **200 mensajes** en memoria y pagina hacia atrás. Doscientas filas caben
// en un `container.Scroll` sin reciclar, y reciclar aquí costaría lo de siempre —el
// hueco de 7 px que `widget.List` mete entre filas, que en un historial son 1.400—
// para ahorrar lo que no hace falta. Si algún día el tope sube, `ui.EnvolverLista` es
// el camino; está escrito en el informe.
var (
	// TopeHistorial es `HISTORY_CAP` de la web: cuántos mensajes se tienen a la vez.
	TopeHistorial = 200
	// CanalonHistorial es el `px-4` del desplazador. MEDIDO (992 con 1020 de columna).
	CanalonHistorial = tema.Espacio(4)
	// CanalonFila es el `px-2` de una fila, y HuecoFila su `gap-3`. MEDIDO.
	CanalonFila = tema.Espacio(2)
	HuecoFila   = tema.Espacio(3)
	// AireFila es el `mt-2` de una fila que abre grupo. MEDIDO: 7.
	AireFila = tema.Espacio(2)
	// RellenoFila y RellenoAgrupada son el `py-1` y el `py-0.5`. MEDIDO.
	RellenoFila     = tema.Espacio(1)
	RellenoAgrupada = tema.Espacio(0.5)
	// ColumnaAvatar es el `w-9` de la columna de la izquierda, y AireAvatar su
	// `pt-0.5`. MEDIDO: 31,5 y 1,75. El avatar `sm` mide 32 y se sale medio píxel por
	// cada lado, igual que en la web.
	ColumnaAvatar = tema.Espacio(9)
	AireAvatar    = tema.Espacio(0.5)
	// HuecoNombre es el `gap-2` entre el nombre y la hora. MEDIDO: 7.
	HuecoNombre = tema.Espacio(2)
	// AireCuerpo es el `mt-0.5` entre la línea del nombre y el texto. HOJA: 1,75.
	AireCuerpo = tema.Espacio(0.5)
	// AireDia es el `my-4` del separador de día, y AireNoLeidos el `my-3` del de
	// «Nuevos mensajes». MEDIDO: 14 y 10,5.
	AireDia      = tema.Espacio(4)
	AireNoLeidos = tema.Espacio(3)
	// HuecoSeparador es el `gap-3` entre la línea y la píldora. MEDIDO: 10,5.
	HuecoSeparador = tema.Espacio(3)
	// CanalonPildora es su `px-3` y RellenoPildora su `py-0.5`. MEDIDO.
	CanalonPildora = tema.Espacio(3)
	RellenoPildora = tema.Espacio(0.5)
	// AireReacciones es el `mt-1` de la tira de reacciones y HuecoReacciones su
	// `gap-1`. HOJA.
	AireReacciones  = tema.Espacio(1)
	HuecoReacciones = tema.Espacio(1)
)

// LadoIconoEstado es el `size={32}` del icono de peligro de un estado de error, y
// LadoIlustracion el `size = 160` por defecto de `<Illustration>`. HOJA.
var (
	LadoIconoEstado float32 = 32
	LadoIlustracion float32 = 160
)

// escalonMeta es `.sytue-meta`: `text-xs` en `text-subtle`. MEDIDO: 10,5 / 14 y
// rgb(90,110,121), que es `text-subtle` con otro nombre.
var escalonMeta = tema.TextoXS

// Historial es la lista de mensajes del canal abierto, en sus seis estados.
type Historial struct {
	widget.BaseWidget

	T      *i18n.Translator
	Locale i18n.Locale

	Estado   Estado
	Mensajes []Mensaje
	// NombreCanal es lo que dice el estado vacío («Escribe el primero en #equipo»).
	NombreCanal string
	// IndiceNoLeidos es ante qué fila va el separador; −1, no hay.
	IndiceNoLeidos int
	// UltimaLlamadaID destaca la ÚNICA tarjeta de llamada a la que se puede entrar.
	// Las demás son una línea sin caja: un canal con reuniones diarias era una pila de
	// cajas iguales en la que la que importa no destacaba.
	UltimaLlamadaID string
	// Ahora es el reloj con el que se escriben «Hoy» y «Ayer». El guardián lo congela.
	Ahora func() time.Time

	AlReintentar      func()
	AlEntrarEnLlamada func(Mensaje)
	AlReaccionar      func(Mensaje, string)
	AlAbrirHilo       func(Mensaje)
	// AlAcusar confirma el acuse de recibo de un mensaje que lo pide.
	AlAcusar func(Mensaje)
	// AlVotar marca o retira un valor de la tarjeta de un mensaje.
	AlVotar func(Mensaje, string, bool)
	// AdjuntosDe cablea la vista de los adjuntos de un mensaje: sus miniaturas y sus
	// acciones.
	AdjuntosDe func(Mensaje, *VistaAdjuntos)
	// EnlacesDe da las tarjetas de los enlaces de un mensaje (vistas previas y reuniones).
	EnlacesDe func(Mensaje) []fyne.CanvasObject
	// AlEmojis abre el selector de emoji de un mensaje colgando de su botón.
	AlEmojis func(Mensaje, fyne.CanvasObject)
	// Menu construye el «…» de un mensaje; nil, sin menú.
	Menu func(Mensaje) []fyne.CanvasObject
	// EditandoID es el mensaje que se está editando (`editingId`), o "".
	EditandoID string
	// Resaltado es el mensaje al que se ha saltado (`highlightId`): va teñido y con su
	// anillo, y la vista lo trae al centro.
	Resaltado         string
	AlGuardarEdicion  func(Mensaje, string)
	AlCancelarEdicion func()

	// HayAnteriores dice si quedan mensajes más viejos que pedir; CargandoAnteriores,
	// que ya van de camino (la píldora de arriba). Acercarse a menos de 300 px del
	// principio llama a AlCargarAnteriores, que es el `onLoadOlder` de la web.
	HayAnteriores, CargandoAnteriores bool
	AlCargarAnteriores                func()

	cuerpo  fyne.CanvasObject
	fondo   *canvas.Rectangle
	pildora *PildoraCargando
	objetos []fyne.CanvasObject
	// primero y altoAntes son el primer mensaje y el alto del contenido del último
	// pintado: si el primero cambia, se han añadido mensajes ARRIBA y la vista se
	// desplaza lo mismo que han crecido, para no moverse de lo que se leía.
	primero   string
	altoAntes float32
}

var _ fyne.Widget = (*Historial)(nil)

// UmbralAnteriores es `LOAD_OLDER_PX`: a cuánto del principio se piden los anteriores.
const UmbralAnteriores float32 = 300

// NuevoHistorial crea la lista con su traductor.
func NuevoHistorial(t *i18n.Translator, l i18n.Locale) *Historial {
	h := &Historial{T: t, Locale: l, IndiceNoLeidos: -1, Ahora: time.Now}
	h.ExtendBaseWidget(h)
	return h
}

// AccessibilityLabel es el `aria-label` de la región de mensajes.
func (h *Historial) AccessibilityLabel() string { return h.T.T("message.listAria", nil) }

// AccessibilityRole implementa fyne.Accessible.
func (h *Historial) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// CreateRenderer construye el cuerpo que toque según el estado.
func (h *Historial) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	h.fondo = canvas.NewRectangle(tema.Color("surface", v))
	h.pildora = NuevaPildoraCargando(h.T.T("message.loadingOlder", nil))
	h.cuerpo = nil // lo construye Refresh, que así sabe que es la primera vez
	r := &historialRenderer{h: h}
	r.Refresh()
	return r
}

// construir devuelve el cuerpo de cada estado. Son seis pinturas distintas y no una
// lista vacía seis veces: el esqueleto no es el vacío y el vacío no es el error.
func (h *Historial) construir() fyne.CanvasObject {
	t := h.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	switch h.Estado {
	case Cargando:
		return NuevoEsqueletoMensajes(t.T("message.loading", nil))
	case Error:
		vac := ui.NuevoVacio(t.T("message.loadFailed", nil), t.T("error.checkConnection", nil))
		vac.Ilustracion = iconos.Trazo(iconos.Peligro, LadoIconoEstado, "danger-text", v).Objeto()
		vac.Accion = ui.NuevoBoton(t.T("action.retry", nil), h.AlReintentar).
			Con(ui.Secundario, ui.TallaSM)
		return vac
	case SinConexion:
		return ui.NuevoVacio(t.T("error.loadFailed", nil), t.T("error.checkConnection", nil))
	case SinPermiso:
		return ui.NuevoVacio(t.T("empty.pickChannel", nil), t.T("empty.pickChannelHelp", nil))
	case Vacio:
		// El título es el principio del canal y no «aún no hay mensajes»: la web
		// cambió a esto porque un canal recién creado no está vacío, está empezando.
		titulo := t.T("message.noneYet", nil)
		if h.NombreCanal != "" {
			titulo = t.T("message.channelStart", i18n.Vars{"channel": h.NombreCanal})
		}
		vac := ui.NuevoVacio(titulo, t.T("message.writeFirst", nil))
		vac.Ilustracion = iconos.Pintar(iconos.VacioChat, v).A(LadoIlustracion).Objeto()
		return vac
	case Listo:
	}

	filas := make([]fyne.CanvasObject, 0, 2*len(h.Mensajes))
	ahora := h.Ahora()
	for i, m := range h.Mensajes {
		if CambiaElDia(h.Mensajes, i) {
			filas = append(filas, NuevoSeparador(
				RotuloDia(h.T, h.Locale, m.Creado, ahora), false))
		}
		if i == h.IndiceNoLeidos {
			filas = append(filas, NuevoSeparador(t.T("message.newMessages", nil), true))
		}
		if m.Tipo == Llamada {
			filas = append(filas, NuevaTarjetaLlamada(h.T, h.Locale, m,
				m.ID == h.UltimaLlamadaID, h.AlEntrarEnLlamada))
			continue
		}
		f := NuevaFilaMensaje(h.T, h.Locale, m, Agrupado(h.Mensajes, i), h.AlReaccionar)
		f.AlAbrirHilo, f.Menu, f.AlEmojis, f.AlAcusar = h.AlAbrirHilo, h.Menu, h.AlEmojis, h.AlAcusar
		f.AlVotar, f.AdjuntosDe, f.EnlacesDe = h.AlVotar, h.AdjuntosDe, h.EnlacesDe
		f.Resaltada = m.ID != "" && m.ID == h.Resaltado
		if m.ID != "" && m.ID == h.EditandoID {
			f.Editando, f.AlGuardarEdicion, f.AlCancelarEdicion = true, h.AlGuardarEdicion, h.AlCancelarEdicion
		}
		filas = append(filas, f)
	}
	sc := container.NewVScroll(container.New(ui.Columna{}, filas...))
	sc.OnScrolled = func(p fyne.Position) {
		if p.Y < UmbralAnteriores && h.HayAnteriores && !h.CargandoAnteriores && h.AlCargarAnteriores != nil {
			h.AlCargarAnteriores()
		}
	}
	return sc
}

type historialRenderer struct{ h *Historial }

// MinSize es cero de ancho: el historial ocupa lo que le den, como el `flex-1` de la
// web. De alto pide una fila, que es lo que mide un canal con un solo mensaje.
func (r *historialRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, AltoFilaMensaje(false))
}

func (r *historialRenderer) Objects() []fyne.CanvasObject { return r.h.objetos }
func (r *historialRenderer) Destroy()                     {}

// Layout deja el cuerpo dentro del `px-4`.
func (r *historialRenderer) Layout(t fyne.Size) {
	r.h.fondo.Resize(t)
	r.h.cuerpo.Move(fyne.NewPos(CanalonHistorial, 0))
	r.h.cuerpo.Resize(fyne.NewSize(max(t.Width-2*CanalonHistorial, 0), t.Height))
	// La píldora de «cargando anteriores»: `absolute inset-x-0 top-2 justify-center`,
	// encima de la lista sin empujarla.
	m := r.h.pildora.MinSize()
	r.h.pildora.Move(fyne.NewPos((t.Width-m.Width)/2, tema.Espacio(2)))
	r.h.pildora.Resize(m)
}

// Refresh REHACE el cuerpo: cambiar de canal o de estado no cambia el tamaño del
// widget, y Fyne solo llama a Layout cuando el tamaño cambia.
//
// Y deja el desplazamiento donde toca, que es lo que hace `MessageView`: un chat se
// abre por el FINAL, y un mensaje nuevo baja la vista solo si ya se estaba abajo. Quien
// se ha subido a leer lo de ayer no pierde el sitio porque alguien escriba.
func (r *historialRenderer) Refresh() {
	h := r.h
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	h.fondo.FillColor = tema.Color("surface", v)
	h.fondo.Refresh()
	pegado, antes := true, fyne.Position{}
	if s, ok := h.cuerpo.(*container.Scroll); ok {
		antes = s.Offset
		pegado = alFondo(s)
	}
	h.cuerpo = h.construir()
	h.objetos = []fyne.CanvasObject{h.fondo, h.cuerpo}
	if h.CargandoAnteriores {
		h.objetos = append(h.objetos, h.pildora)
	}
	r.Layout(h.Size())
	primero := ""
	if len(h.Mensajes) > 0 {
		primero = h.Mensajes[0].ID
	}
	if s, ok := h.cuerpo.(*container.Scroll); ok {
		alto := s.Content.MinSize().Height
		switch {
		case h.Resaltado != "":
			h.centrar(s)
		case primero != h.primero && h.primero != "" && !pegado:
			// Han entrado mensajes arriba: se sigue viendo lo mismo.
			s.Offset.Y = antes.Y + alto - h.altoAntes
			s.Refresh()
		case pegado:
			s.ScrollToBottom()
		default:
			s.Offset = antes
			s.Refresh()
		}
		h.altoAntes = alto
	}
	h.primero = primero
}

// centrar trae la fila resaltada al medio de la vista.
func (h *Historial) centrar(s *container.Scroll) {
	c, ok := s.Content.(*fyne.Container)
	if !ok {
		return
	}
	for _, o := range c.Objects {
		if f, es := o.(*FilaMensaje); es && f.Resaltada {
			y := f.Position().Y - (s.Size().Height-f.Size().Height)/2
			s.Offset.Y = max(0, min(y, c.MinSize().Height-s.Size().Height))
			s.Refresh()
			return
		}
	}
}

// QuitarResalte apaga el resalte sin rehacer la lista ni mover la vista, que es lo que
// hace la web al pasar los 2,5 s.
func (h *Historial) QuitarResalte() {
	s, ok := h.cuerpo.(*container.Scroll)
	if !ok {
		return
	}
	c, ok := s.Content.(*fyne.Container)
	if !ok {
		return
	}
	for _, o := range c.Objects {
		if f, es := o.(*FilaMensaje); es && f.Resaltada {
			f.Resaltada = false
			f.pintarHover()
		}
	}
}

// alFondo dice si la vista está abajo del todo, con un píxel de holgura por el
// redondeo del desplazamiento.
// Sin tamaño todavía no se ha visto nada, y se cuenta como abajo.
func alFondo(s *container.Scroll) bool {
	return s.Size().Height == 0 || s.Offset.Y >= s.Content.MinSize().Height-s.Size().Height-1
}

// ---- La fila de mensaje ----

// AltoFilaMensaje es lo que ocupa una fila de UNA línea de texto. Es la única cuenta
// del mínimo; el alto real lo pone el cuerpo, que se reparte en líneas.
//
// Sin agrupar: `mt-2` (7) + `py-1` (3,5 ×2) + la línea del nombre (15,75) + `mt-0.5`
// (1,75) + una línea de cuerpo (19,25) = **51,25**. Agrupada: `py-0.5` (1,75 ×2) +
// 19,25 = **22,75**, que es el mínimo que dio la medida.
func AltoFilaMensaje(agrupada bool) float32 {
	if agrupada {
		return 2*RellenoAgrupada + tema.TextoBase.Linea
	}
	return AireFila + 2*RellenoFila + tema.TextoSM.Linea + AireCuerpo + tema.TextoBase.Linea
}

// MedidaFilaMensaje es lo que ocupa la fila a un ancho dado, con el cuerpo ya repartido
// en líneas y su tira de reacciones.
func MedidaFilaMensaje(f *FilaMensaje, ancho float32) fyne.Size {
	arriba, abajo, _ := f.aires()
	alto := arriba + abajo
	if !f.Agrupada {
		alto += tema.TextoSM.Linea
		if !f.EnHilo {
			alto += AireCuerpo
		}
	}
	if f.Editando {
		alto += MedidaEdicion(f.filasEdicion()).Height
	} else {
		alto += f.altoPrioridad() + f.altoCuerpo(ancho)
		if f.editadoAparte(ancho) {
			alto += tema.TextoBase.Linea
		}
		alto += f.altoAdjuntos(ancho) + f.altoTarjeta(ancho) + f.altoEnlaces(ancho)
	}
	if t := f.altoTira(); t > 0 {
		alto += AireReacciones + t
	}
	return fyne.NewSize(ancho, alto+f.altoAcuse())
}

// FilaMensaje es una fila del historial. No es `ui.FilaLista`, y la diferencia es de
// fondo: aquella es una fila de altura fija con título y subtítulo; esta AGRUPA por
// autor —la segunda de la misma persona pierde avatar y nombre y deja la hora en el
// canalón, visible al pasar—, reparte el cuerpo en las líneas que hagan falta y lleva
// su tira de reacciones debajo. Es propia de Rooms igual que en la web.
type FilaMensaje struct {
	widget.BaseWidget

	T        *i18n.Translator
	Locale   i18n.Locale
	Mensaje  Mensaje
	Agrupada bool
	// EnHilo es la fila del panel del hilo: otros aires, sin agrupar y sin tira de
	// reacciones, que en la web viven solo en el histórico del canal.
	EnHilo bool

	AlReaccionar func(Mensaje, string)
	AlAbrirHilo  func(Mensaje)
	AlEmojis     func(Mensaje, fyne.CanvasObject)
	// AlAcusar es «Confirmar» en un mensaje que pide acuse; nil, sin botón.
	AlAcusar func(Mensaje)
	// AlVotar vota en la tarjeta del mensaje; nil, de solo lectura.
	AlVotar func(Mensaje, string, bool)
	// AdjuntosDe cablea la vista de sus adjuntos; EnlacesDe da las tarjetas de sus
	// enlaces.
	AdjuntosDe func(Mensaje, *VistaAdjuntos)
	EnlacesDe  func(Mensaje) []fyne.CanvasObject
	// Menu construye las entradas del «…» de este mensaje; nil, sin menú.
	Menu func(Mensaje) []fyne.CanvasObject
	// Resaltada es `m.id === highlightId`: `bg-selected ring-2 ring-primary`.
	Resaltada bool
	// Editando cambia el texto por la caja de editarlo (`editingId === m.id`).
	Editando          bool
	AlGuardarEdicion  func(Mensaje, string)
	AlCancelarEdicion func()

	// encima, barraEncima e hijos dicen si el puntero está sobre la fila, sobre su
	// barra o sobre uno de sus controles; tocado es el toque del dedo (`tocado`).
	// Cualquiera de los cuatro enseña la barra de acciones.
	encima, barraEncima, tocado bool
	// focoBarra es el `focus-within` de la barra: con el foco dentro, se ve.
	focoBarra bool
	hijos     int
	barra     *BarraMensaje
	edicion   *Edicion
	anillo    *ui.Aro

	editado    *canvas.Text
	enlaceHilo *EnlaceHilo
	// prioridad es la píldora de encima del texto y acuse, la fila de debajo; tarjeta,
	// la encuesta o la tarea bajo el texto.
	prioridad *Pildora
	acuse     []fyne.CanvasObject
	tarjeta   *VistaTarjeta
	adjuntos  *VistaAdjuntos
	enlaces   []fyne.CanvasObject
	// siguiendo y glifoSiguiendo son el «Siguiendo» de la tira.
	siguiendo      *canvas.Text
	glifoSiguiendo fyne.CanvasObject
	fondo          *canvas.Rectangle
	avatar         *ui.Avatar
	hora           *canvas.Text
	nombre         *canvas.Text
	insignia       *ui.Insignia
	cuerpo         *CuerpoRico
	reacciones     []fyne.CanvasObject
	// masReaccion es el «+» del final de las reacciones.
	masReaccion *MasReaccion
	objetos     []fyne.CanvasObject
	ancho       float32
}

var (
	_ fyne.Widget       = (*FilaMensaje)(nil)
	_ desktop.Hoverable = (*FilaMensaje)(nil)
	_ fyne.Tappable     = (*FilaMensaje)(nil)
)

// MouseIn es el `group-hover` de la fila.
func (f *FilaMensaje) MouseIn(*desktop.MouseEvent) { f.encima = true; f.pintarHover() }

// MouseMoved no hace nada: la fila entera es la diana.
func (f *FilaMensaje) MouseMoved(*desktop.MouseEvent) {}

// MouseOut apaga el `hover` de la fila.
func (f *FilaMensaje) MouseOut() { f.encima = false; f.pintarHover() }

// Tapped es el sustituto TÁCTIL del `hover`: con el dedo, tocar el mensaje despliega
// su barra y volver a tocarlo la recoge (`revelaPorToque`). Con ratón no hace nada.
func (f *FilaMensaje) Tapped(*fyne.PointEvent) {
	if !ui.EsTactil() || f.Editando {
		return
	}
	f.tocado = !f.tocado
	f.pintarHover()
}

// hijoEncima lo llaman los controles de la fila —reacciones, «N respuestas»— al entrar
// y salir el puntero: sobre ellos la fila sigue «encima», como en la web.
func (f *FilaMensaje) hijoEncima(si bool) {
	if si {
		f.hijos++
	} else {
		f.hijos = max(f.hijos-1, 0)
	}
	f.pintarHover()
}

// conHover dice si la fila se pinta como con el puntero encima: el fondo `bg-subtle`
// y la barra de acciones.
func (f *FilaMensaje) conHover() bool {
	return f.encima || f.barraEncima || f.hijos > 0 || f.tocado || f.focoBarra
}

func (f *FilaMensaje) pintarHover() {
	if f.fondo == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo.FillColor = color.NRGBA{}
	switch {
	case f.Resaltada:
		f.fondo.FillColor = tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
	case f.conHover() && !f.focoBarra && !f.EnHilo, f.encima && !f.EnHilo:
		// El fondo es `group-hover`: el foco en la barra la enciende, pero no tiñe.
		f.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	f.fondo.Refresh()
	if f.anillo != nil {
		f.anillo.Ver(f.Resaltada)
		f.anillo.Pintar(v)
	}
	if f.barra != nil {
		// Siempre en el árbol —para que el tabulador llegue—, y apagada sin el puntero
		// ni el foco: `opacity-0`, no `display:none`.
		apagada := !f.conHover() || f.Editando
		if f.barra.Apagada != apagada {
			f.barra.Apagada = apagada
			f.barra.Refresh()
		}
	}
}

// Los aires de la fila del HILO (`ThreadPanel.tsx`): `flex gap-3 px-4 py-2`, sin el
// `mt-2` de la del canal, sin agrupar y sin el `mt-0.5` antes del texto. HOJA.
var (
	CanalonFilaHilo = tema.Espacio(4)
	RellenoFilaHilo = tema.Espacio(2)
)

// aires son el aire de arriba, el de abajo y el canalón de los lados de la fila: los
// del canal (`mt-2 py-1 px-2`, o `py-0.5` agrupada) o los del hilo.
func (f *FilaMensaje) aires() (arriba, abajo, canalon float32) {
	switch {
	case f.EnHilo:
		return RellenoFilaHilo, RellenoFilaHilo, CanalonFilaHilo
	case f.Agrupada:
		return RellenoAgrupada, RellenoAgrupada, CanalonFila
	}
	return AireFila + RellenoFila, RellenoFila, CanalonFila
}

// conEnlaceHilo dice si la fila lleva «N respuestas»: en el canal, con respuestas y
// alguien que abra el hilo. Dentro del hilo no, que ya se está en él.
func (f *FilaMensaje) conEnlaceHilo() bool {
	return !f.EnHilo && f.Mensaje.Respuestas > 0 && f.AlAbrirHilo != nil
}

// conSiguiendo dice si la fila lleva «Siguiendo»: en el canal, si se sigue su hilo.
func (f *FilaMensaje) conSiguiendo() bool { return !f.EnHilo && f.Mensaje.Siguiendo }

// LadoGlifoSiguiendo es el `size={12}` del icono de «Siguiendo».
const LadoGlifoSiguiendo float32 = 12

// altoTira es el alto de la tira de debajo del texto —reacciones, «N respuestas» y
// «Siguiendo», `mt-1 flex flex-wrap items-center gap-1`—, o cero si no hay tira.
func (f *FilaMensaje) altoTira() float32 {
	alto := float32(0)
	if len(f.Mensaje.Reacciones) > 0 && !f.EnHilo {
		alto = AltoReaccion
		if f.AlEmojis != nil {
			alto = AltoMasReaccion
		}
	}
	if f.conEnlaceHilo() || f.conSiguiendo() {
		alto = max(alto, AltoEnlaceHilo) // «Siguiendo» lleva el mismo `px-1.5 py-0.5 text-xs`
	}
	return alto
}

// editadoAparte dice si «(editado)» no cabe detrás de la última línea y baja a la
// siguiente, que es lo que hace el navegador con un `<span>` en línea.
func (f *FilaMensaje) editadoAparte(ancho float32) bool {
	if !f.Mensaje.Editado || f.Editando {
		return false
	}
	_, fin := MaquetarCuerpoDe(f.Mensaje.Cuerpo, f.Mensaje.Autor.Nombre, f.Mensaje.Menciones, f.cabe(ancho))
	return fin+AireEditado+f.anchoEditado() > f.cabe(ancho)
}

// altoCuerpo es lo que ocupa el texto con su Markdown. Sin cuerpo no hay párrafo
// (`m.body ? <p>… : null`), salvo que diga «(editado)», que necesita su renglón.
func (f *FilaMensaje) altoCuerpo(ancho float32) float32 {
	if f.Mensaje.Cuerpo == "" {
		if f.Mensaje.Editado {
			return tema.TextoBase.Linea
		}
		return 0
	}
	alto, _ := MaquetarCuerpoDe(f.Mensaje.Cuerpo, f.Mensaje.Autor.Nombre, f.Mensaje.Menciones, f.cabe(ancho))
	return alto
}

// filasEdicion son las filas de la caja de editar: las de lo que lleva escrito.
func (f *FilaMensaje) filasEdicion() int {
	if f.edicion != nil {
		return f.edicion.filas()
	}
	return max(1, min(FilasEdicionTope, len(partirEnLineas(f.Mensaje.Cuerpo))))
}

func (f *FilaMensaje) anchoEditado() float32 {
	return ui.Medir(f.T.T("message.edited", nil), escalonMeta, ui.Normal).Width
}

// altoAdjuntos es lo que ocupan los adjuntos, con su `mt-1.5`, o cero.
func (f *FilaMensaje) altoAdjuntos(ancho float32) float32 {
	if f.adjuntos == nil {
		return 0
	}
	return MedidaAdjuntos(f.adjuntos, f.cabe(ancho)).Height
}

// altoEnlaces es lo que ocupan las tarjetas de sus enlaces, cada una con su `mt-1.5`.
func (f *FilaMensaje) altoEnlaces(ancho float32) float32 {
	alto := float32(0)
	for _, e := range f.enlaces {
		alto += AireTarjetaEnlace + MedidaTarjetaEnlace(e.(*TarjetaEnlace), f.cabe(ancho)).Height //nolint:errcheck,forcetypeassert // Solo NuevaTarjetaEnlace rellena esta lista.
	}
	return alto
}

// altoTarjeta es lo que ocupa la encuesta o la tarea, con su `mt-1.5`, o cero.
func (f *FilaMensaje) altoTarjeta(ancho float32) float32 {
	if f.tarjeta == nil {
		return 0
	}
	return MedidaTarjeta(f.tarjeta, f.cabe(ancho)).Height
}

// AireEditado es el `ml-1` de «(editado)». HOJA.
var AireEditado = tema.Espacio(1)

// NuevaFilaMensaje crea la fila.
func NuevaFilaMensaje(t *i18n.Translator, l i18n.Locale, m Mensaje, agrupada bool,
	alReaccionar func(Mensaje, string),
) *FilaMensaje {
	f := &FilaMensaje{T: t, Locale: l, Mensaje: m, Agrupada: agrupada, AlReaccionar: alReaccionar}
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es quién, cuándo y qué: lo que un lector de pantalla necesita de
// una fila agrupada, que en pantalla ya no lleva ni nombre ni hora.
func (f *FilaMensaje) AccessibilityLabel() string {
	return f.Mensaje.Autor.Nombre + ", " +
		RotuloFechaCompleta(f.Locale, f.Mensaje.Creado) + ": " + f.Mensaje.Cuerpo
}

// AccessibilityRole implementa fyne.Accessible.
func (f *FilaMensaje) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// cabe es el ancho de la columna del texto: la fila menos sus canalones, la columna
// del avatar y el `gap-3`.
func (f *FilaMensaje) cabe(ancho float32) float32 {
	_, _, canalon := f.aires()
	return max(ancho-2*canalon-ColumnaAvatar-HuecoFila, 0)
}

// partirEnLineas corta por `\n`, que es lo que conserva `whitespace-pre-wrap`.
func partirEnLineas(s string) []string {
	var out []string
	ini := 0
	for i := range len(s) {
		if s[i] == '\n' {
			out = append(out, s[ini:i])
			ini = i + 1
		}
	}
	return append(out, s[ini:])
}

// CreateRenderer compone la fila. Solo primitivas y widgets de `ui`.
//
//nolint:gocyclo // Cada rama crea una pieza visual opcional de la fila de mensaje.
func (f *FilaMensaje) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo = canvas.NewRectangle(color.NRGBA{})
	f.fondo.CornerRadius = tema.RadioMD
	// El anillo del mensaje enlazado es el `ring-2 ring-primary` de la hoja: un aro de
	// 2 px pegado a la caja, que es lo que dibuja `ui.Aro` sin hueco.
	f.anillo = ui.NuevoAroCon("primary", tema.RadioMD, ui.SinHueco, v)
	f.objetos = append(f.anillo.Objetos(), f.fondo)

	if f.Agrupada {
		// La hora, en el canalón del avatar y alineada a la derecha. En la web aparece
		// al pasar por encima (`opacity-0 group-hover:opacity-100`) y SIEMPRE con el
		// dedo (`pointer-coarse:opacity-100`): con el dedo el `hover` no llega nunca y
		// la hora de un mensaje agrupado es justo lo que se busca al releer. Aquí se
		// queda puesta, que es el caso táctil, y no se apaga: esconderla detrás del
		// ratón en un teléfono es perderla.
		f.hora = ui.Texto(RotuloHora(f.Locale, f.Mensaje.Creado), escalonMeta, ui.Normal,
			tema.Color("text-subtle", v))
		f.objetos = append(f.objetos, f.hora)
	} else {
		f.avatar = ui.NuevoAvatar(f.Mensaje.Autor.Nombre).Con(ui.AvatarSM)
		f.nombre = ui.Texto(f.Mensaje.Autor.Nombre, tema.TextoSM, ui.SemiNegrita,
			tema.Color("text", v))
		f.hora = ui.Texto(RotuloHora(f.Locale, f.Mensaje.Creado), escalonMeta, ui.Normal,
			tema.Color("text-subtle", v))
		f.objetos = append(f.objetos, f.avatar, f.nombre, f.hora)
		if f.Mensaje.Autor.Bot {
			// «BOT» es TEXTO y no solo un color: marca los mensajes de webhooks y hay
			// que poder leerlo (WCAG 1.4.1).
			f.insignia = ui.NuevaInsignia(f.T.T("message.bot", nil), ui.InsigniaNeutra)
			f.insignia.Cuadrada = true
			f.objetos = append(f.objetos, f.insignia)
		}
	}
	if f.Mensaje.Prioridad != Normal && !f.EnHilo {
		f.prioridad = NuevaPildoraPrioridad(f.T, f.Mensaje.Prioridad)
		f.objetos = append(f.objetos, f.prioridad)
	}
	f.cuerpo = NuevoCuerpoRico(f.Mensaje.Cuerpo, f.Mensaje.Menciones)
	f.cuerpo.Autor = f.Mensaje.Autor.Nombre
	f.cuerpo.T = f.T
	f.cuerpo.AlCopiar = func(s string) { fyne.CurrentApp().Clipboard().SetContent(s) }
	f.objetos = append(f.objetos, f.cuerpo)
	if f.Mensaje.Editado {
		f.editado = ui.Texto(f.T.T("message.edited", nil), escalonMeta, ui.Normal,
			tema.Color("text-subtle", v))
		f.objetos = append(f.objetos, f.editado)
	}
	for _, r := range f.Mensaje.Reacciones {
		if f.EnHilo {
			break
		}
		p := NuevaReaccion(f.T, r, func() {
			if f.AlReaccionar != nil {
				f.AlReaccionar(f.Mensaje, r.Emoji)
			}
		})
		p.alEncima = f.hijoEncima
		o := p.ConPista()
		f.reacciones = append(f.reacciones, o)
		f.objetos = append(f.objetos, o)
	}
	if len(f.reacciones) > 0 && f.AlEmojis != nil {
		var mas *MasReaccion
		mas = NuevoMasReaccion(f.T, func() { f.AlEmojis(f.Mensaje, mas) })
		mas.alEncima = f.hijoEncima
		f.masReaccion = mas
		f.objetos = append(f.objetos, mas)
	}
	if f.conEnlaceHilo() {
		f.enlaceHilo = NuevoEnlaceHilo(
			f.T.T("thread.replyCountInThread", i18n.Vars{"count": f.Mensaje.Respuestas}),
			func() { f.AlAbrirHilo(f.Mensaje) })
		f.enlaceHilo.alEncima = f.hijoEncima
		f.objetos = append(f.objetos, f.enlaceHilo)
	}
	if f.conSiguiendo() {
		// Texto e icono, nunca solo color: `sytue-meta font-medium` con ThreadsGlyph.
		f.glifoSiguiendo = dibujo(glifoHilos, LadoGlifoSiguiendo, "text-subtle", v).Objeto()
		f.siguiendo = ui.Texto(f.T.T("thread.following", nil), escalonMeta, ui.Medio, tema.Color("text-subtle", v))
		f.objetos = append(f.objetos, f.glifoSiguiendo, f.siguiendo)
	}
	if len(f.Mensaje.Adjuntos) > 0 {
		f.adjuntos = NuevaVistaAdjuntos(f.T, f.Mensaje.Adjuntos)
		if f.AdjuntosDe != nil {
			f.AdjuntosDe(f.Mensaje, f.adjuntos)
		}
		f.objetos = append(f.objetos, f.adjuntos)
	}
	if f.Mensaje.Tarjeta.Visible() {
		var votar func(string, bool)
		if f.AlVotar != nil && !f.Mensaje.Enviando && !f.Mensaje.Fallido {
			votar = func(valor string, poner bool) { f.AlVotar(f.Mensaje, valor, poner) }
		}
		f.tarjeta = NuevaVistaTarjeta(f.T, f.Locale, f.Mensaje.Tarjeta, votar)
		f.objetos = append(f.objetos, f.tarjeta)
	}
	if f.EnlacesDe != nil && !f.EnHilo {
		f.enlaces = f.EnlacesDe(f.Mensaje)
		f.objetos = append(f.objetos, f.enlaces...)
	}
	f.montarAcuse()
	if f.Editando {
		f.edicion = NuevaEdicion(f.T, f.Mensaje.Cuerpo,
			func(s string) {
				if f.AlGuardarEdicion != nil {
					f.AlGuardarEdicion(f.Mensaje, s)
				}
			}, f.AlCancelarEdicion)
		f.objetos = append(f.objetos, f.edicion)
	}
	f.montarBarra()
	r := &filaMensajeRenderer{f: f}
	r.Refresh()
	return r
}

type filaMensajeRenderer struct{ f *FilaMensaje }

func (r *filaMensajeRenderer) MinSize() fyne.Size {
	return MedidaFilaMensaje(r.f, r.f.ancho)
}

func (r *filaMensajeRenderer) Objects() []fyne.CanvasObject { return r.f.objetos }
func (r *filaMensajeRenderer) Destroy()                     {}

// Layout coloca canalón, avatar, cabecera y cuerpo.
func (r *filaMensajeRenderer) Layout(t fyne.Size) {
	f := r.f
	f.ancho = t.Width
	// El fondo del `hover` y la barra son los del `div` de la fila, que en la web no
	// incluye su `mt-2`.
	margen := float32(0)
	if !f.Agrupada && !f.EnHilo {
		margen = AireFila
	}
	f.fondo.Move(fyne.NewPos(0, margen))
	f.fondo.Resize(fyne.NewSize(t.Width, max(t.Height-margen, 0)))
	f.anillo.ColocarEn(fyne.NewPos(0, margen), fyne.NewSize(t.Width, max(t.Height-margen, 0)))
	if f.barra != nil {
		m := f.barra.MinSize()
		f.barra.Move(fyne.NewPos(t.Width-DerechaBarraMensaje-m.Width, margen-ArribaBarraMensaje))
		f.barra.Resize(m)
	}

	y, _, canalon := f.aires()
	x := canalon + ColumnaAvatar + HuecoFila

	switch {
	case f.Agrupada:
		// `pt-1 text-right leading-4` dentro de la columna de 31,5.
		ancho := ui.Medir(f.hora.Text, escalonMeta, ui.Normal).Width
		ui.ColocarTexto(f.hora, escalonMeta, canalon+ColumnaAvatar-ancho, y)
	default:
		f.avatar.Move(fyne.NewPos(canalon, y+AireAvatar))
		f.avatar.Resize(ui.MedidaAvatar(ui.AvatarSM))
		// La línea del nombre: nombre, insignia y hora, alineados por la BASE, que es
		// lo que hace `items-baseline` con dos escalones distintos.
		nx := x
		anchoNombre := ui.Truncar(f.nombre, f.Mensaje.Autor.Nombre, tema.TextoSM, ui.SemiNegrita,
			max(t.Width-canalon-nx, 0))
		ui.ColocarTexto(f.nombre, tema.TextoSM, nx, y)
		nx += anchoNombre + HuecoNombre
		if f.insignia != nil {
			m := ui.MedidaInsignia(f.insignia.Texto)
			f.insignia.Move(fyne.NewPos(nx, y+(tema.TextoSM.Linea-m.Height)/2))
			f.insignia.Resize(m)
			nx += m.Width + HuecoNombre
		}
		ui.ColocarTexto(f.hora, escalonMeta, nx, y+(tema.TextoSM.Linea-escalonMeta.Linea))
		y += tema.TextoSM.Linea
		if !f.EnHilo {
			y += AireCuerpo
		}
	}

	if f.edicion != nil {
		f.cuerpo.Hide()
		if f.prioridad != nil {
			f.prioridad.Hide()
		}
		m := MedidaEdicion(f.edicion.filas())
		f.edicion.Move(fyne.NewPos(x, y))
		f.edicion.Resize(fyne.NewSize(f.cabe(t.Width), m.Height))
		y += m.Height
		r.colocarAcuse(x, r.colocarTira(x, y))
		return
	}
	if f.prioridad != nil {
		m := MedidaPildora(f.prioridad)
		f.prioridad.Show()
		f.prioridad.Move(fyne.NewPos(x, y))
		f.prioridad.Resize(m)
		y += m.Height + AirePrioridad
	}
	altoCuerpo := f.altoCuerpo(t.Width)
	f.cuerpo.Show()
	f.cuerpo.Move(fyne.NewPos(x, y))
	f.cuerpo.Resize(fyne.NewSize(f.cabe(t.Width), altoCuerpo))
	y += altoCuerpo
	if f.editado != nil {
		// «(editado)» sigue a la última línea, en su base, o baja a la siguiente.
		_, ultima := MaquetarCuerpoDe(f.Mensaje.Cuerpo, f.Mensaje.Autor.Nombre, f.Mensaje.Menciones, f.cabe(t.Width))
		ex, ey := x+ultima+AireEditado, y-tema.TextoBase.Linea
		if f.editadoAparte(t.Width) {
			ex, ey = x, y
			y += tema.TextoBase.Linea
		}
		ui.ColocarTexto(f.editado, escalonMeta, ex, ey+(tema.TextoBase.Linea-escalonMeta.Linea)/2)
	}
	if f.adjuntos != nil {
		alto := f.altoAdjuntos(t.Width)
		f.adjuntos.Move(fyne.NewPos(x, y))
		f.adjuntos.Resize(fyne.NewSize(f.cabe(t.Width), alto))
		y += alto
	}
	if f.tarjeta != nil {
		alto := f.altoTarjeta(t.Width)
		f.tarjeta.Move(fyne.NewPos(x, y))
		f.tarjeta.Resize(fyne.NewSize(f.cabe(t.Width), alto))
		y += alto
	}
	for _, e := range f.enlaces {
		c := e.(*TarjetaEnlace) //nolint:errcheck,forcetypeassert // Solo NuevaTarjetaEnlace rellena esta lista.
		m := MedidaTarjetaEnlace(c, f.cabe(t.Width))
		y += AireTarjetaEnlace
		c.Move(fyne.NewPos(x, y))
		c.Resize(m)
		y += m.Height
	}

	r.colocarAcuse(x, r.colocarTira(x, y))
}

// colocarAcuse pone la fila del acuse (`mt-1.5 flex items-center gap-2`) a partir de y.
func (r *filaMensajeRenderer) colocarAcuse(x, y float32) {
	f := r.f
	if len(f.acuse) == 0 {
		return
	}
	y += AireAcuse
	colocarEnvuelto(f.acuse, x, y, f.cabe(f.ancho), HuecoAcuse, true)
}

// colocarTira pone las reacciones y «N respuestas» debajo del texto, y devuelve dónde
// acaba.
func (r *filaMensajeRenderer) colocarTira(x, y float32) float32 {
	f := r.f
	alto := f.altoTira()
	if alto == 0 {
		return y
	}
	y += AireReacciones
	rx := x
	for _, p := range f.reacciones {
		m := p.MinSize()
		p.Move(fyne.NewPos(rx, y+(alto-m.Height)/2))
		p.Resize(m)
		rx += m.Width + HuecoReacciones
	}
	if f.masReaccion != nil {
		m := MedidaMasReaccion()
		f.masReaccion.Move(fyne.NewPos(rx, y+(alto-m.Height)/2))
		f.masReaccion.Resize(m)
		rx += m.Width + HuecoReacciones
	}
	if f.enlaceHilo != nil {
		m := f.enlaceHilo.MinSize()
		f.enlaceHilo.Move(fyne.NewPos(rx, y+(alto-m.Height)/2))
		f.enlaceHilo.Resize(m)
		rx += m.Width + HuecoReacciones
	}
	if f.siguiendo != nil {
		gx := rx + tema.Espacio(1.5) // `px-1.5`, y `gap-1` entre el icono y el texto
		f.glifoSiguiendo.Move(fyne.NewPos(gx, y+(alto-LadoGlifoSiguiendo)/2))
		f.glifoSiguiendo.Resize(fyne.NewSize(LadoGlifoSiguiendo, LadoGlifoSiguiendo))
		ui.ColocarTexto(f.siguiendo, escalonMeta, gx+LadoGlifoSiguiendo+tema.Espacio(1), y+(alto-escalonMeta.Linea)/2)
	}
	return y + alto
}

// Refresh repinta con los colores del tema y rehace la disposición.
func (r *filaMensajeRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if f.nombre != nil {
		f.nombre.Color = tema.Color("text", v)
		f.nombre.Refresh()
	}
	if f.hora != nil {
		f.hora.Color = tema.Color("text-subtle", v)
		f.hora.Refresh()
	}
	f.cuerpo.Refresh()
	if f.editado != nil {
		f.editado.Color = tema.Color("text-subtle", v)
		f.editado.Refresh()
	}
	f.pintarHover() // el fondo y el anillo del resalte, y la barra si está encima
	r.Layout(f.Size())
}

// montarBarra pone la barra de acciones, escondida hasta el `hover`. Va la ÚLTIMA de
// los objetos para pintarse encima de la fila, y sale por arriba de ella como el
// `-top-3.5` de la web. Las filas del hilo no la llevan (`ThreadPanel.tsx`).
func (f *FilaMensaje) montarBarra() {
	if f.EnHilo || f.Mensaje.Enviando || f.Mensaje.Fallido ||
		(f.AlReaccionar == nil && f.AlAbrirHilo == nil && f.Menu == nil) {
		return
	}
	b := NuevaBarraMensaje(f.T)
	if f.AlReaccionar != nil {
		b.AlReaccionar = func(e string) { f.AlReaccionar(f.Mensaje, e) }
		if f.AlEmojis != nil {
			b.AlEmojis = func(d fyne.CanvasObject) { f.AlEmojis(f.Mensaje, d) }
		}
	}
	if f.AlAbrirHilo != nil {
		b.AlHilo = func() { f.AlAbrirHilo(f.Mensaje) }
	}
	if f.Menu != nil {
		b.Menu = func() []fyne.CanvasObject { return f.Menu(f.Mensaje) }
	}
	b.AlEncima = func(si bool) {
		f.barraEncima = si
		f.pintarHover()
	}
	b.AlFoco = func(si bool) {
		f.focoBarra = si
		f.pintarHover()
	}
	b.Apagada = true
	f.barra = b
	f.objetos = append(f.objetos, b)
}

// ---- El separador de día y el de «Nuevos mensajes» ----

// Medidas del separador. MEDIDO: la píldora sale 17,5 de alto (14 de caja de línea
// más `py-0.5` por lado) y el bloque 45,5 con su `my-4`.
var (
	// AltoPildora es `text-xs` con `py-0.5`.
	AltoPildora = tema.TextoXS.Linea + 2*RellenoPildora
)

// Separador es la línea con una píldora en medio. Sirve para los dos: el de día
// —filete `border`, píldora de `surface` con el texto en `text-muted`— y el de
// «Nuevos mensajes», que va entero en `primary` porque marca dónde te quedaste.
type Separador struct {
	widget.BaseWidget

	Rotulo string
	// Acento es el de «Nuevos mensajes».
	Acento bool

	izq, der *canvas.Rectangle
	caja     *ui.Caja
	texto    *canvas.Text
	objetos  []fyne.CanvasObject
}

var _ fyne.Widget = (*Separador)(nil)

// NuevoSeparador crea el separador con su rótulo ya traducido.
func NuevoSeparador(rotulo string, acento bool) *Separador {
	s := &Separador{Rotulo: rotulo, Acento: acento}
	s.ExtendBaseWidget(s)
	return s
}

// AccessibilityLabel es el rótulo; en la web es un `role="separator"` con su
// `aria-label`.
func (s *Separador) AccessibilityLabel() string { return s.Rotulo }

// AccessibilityRole implementa fyne.Accessible.
func (s *Separador) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// MedidaSeparador es lo que ocupa de alto, con su margen.
func MedidaSeparador(acento bool) float32 {
	aire := AireDia
	if acento {
		aire = AireNoLeidos
	}
	return 2*aire + AltoPildora
}

func (s *Separador) tokens() (filete, fondo, tinta string) {
	if s.Acento {
		return "primary", "primary", "on-primary"
	}
	return "border", "surface", "text-muted"
}

// CreateRenderer compone las dos líneas y la píldora.
func (s *Separador) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	filete, fondo, tinta := s.tokens()
	borde := tema.Color("border", v)
	if s.Acento {
		borde = color.NRGBA{}
	}
	s.izq = canvas.NewRectangle(tema.Color(filete, v))
	s.der = canvas.NewRectangle(tema.Color(filete, v))
	s.caja = ui.NuevaCaja(tema.Color(fondo, v), borde, tema.RadioPill)
	s.texto = ui.Texto(s.Rotulo, tema.TextoXS, ui.SemiNegrita, tema.Color(tinta, v))
	s.objetos = []fyne.CanvasObject{s.izq, s.der}
	s.objetos = append(s.objetos, s.caja.Objetos()...)
	s.objetos = append(s.objetos, s.texto)
	r := &separadorRenderer{s: s}
	r.Refresh()
	return r
}

type separadorRenderer struct{ s *Separador }

func (r *separadorRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, MedidaSeparador(r.s.Acento))
}
func (r *separadorRenderer) Objects() []fyne.CanvasObject { return r.s.objetos }
func (r *separadorRenderer) Destroy()                     {}

// Layout centra la píldora y reparte las dos líneas a los lados (`flex-1`).
func (r *separadorRenderer) Layout(t fyne.Size) {
	s := r.s
	aire := AireDia
	if s.Acento {
		aire = AireNoLeidos
	}
	anchoTexto := ui.Medir(s.Rotulo, tema.TextoXS, ui.SemiNegrita).Width
	anchoPildora := anchoTexto + 2*CanalonPildora
	lado := max((t.Width-anchoPildora)/2-HuecoSeparador, 0)
	medio := aire + AltoPildora/2

	s.izq.Move(fyne.NewPos(0, medio-tema.Borde/2))
	s.izq.Resize(fyne.NewSize(lado, tema.Borde))
	s.der.Move(fyne.NewPos(t.Width-lado, medio-tema.Borde/2))
	s.der.Resize(fyne.NewSize(lado, tema.Borde))

	x := lado + HuecoSeparador
	ui.MoverCaja(s.caja, fyne.NewPos(x, aire), fyne.NewSize(anchoPildora, AltoPildora))
	ui.ColocarTexto(s.texto, tema.TextoXS, x+CanalonPildora, aire+RellenoPildora)
}

// Refresh repinta con los colores del tema actual.
func (r *separadorRenderer) Refresh() {
	s := r.s
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	filete, fondo, tinta := s.tokens()
	borde := tema.Color("border", v)
	if s.Acento {
		borde = color.NRGBA{}
	}
	s.izq.FillColor = tema.Color(filete, v)
	s.izq.Refresh()
	s.der.FillColor = tema.Color(filete, v)
	s.der.Refresh()
	s.caja.Pintar(tema.Color(fondo, v), borde)
	s.texto.Color = tema.Color(tinta, v)
	s.texto.Refresh()
	r.Layout(s.Size())
}
