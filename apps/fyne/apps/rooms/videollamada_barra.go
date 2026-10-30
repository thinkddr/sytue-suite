// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El resto de la barra de VideoRoom.tsx: el chat del canal, el panel de participantes
// con su moderación y la cola de la puerta, las reacciones y la mano, grabar, copiar el
// enlace y elegir dispositivos. Todo lo que modera lo REAUTORIZA el servicio (anfitrión
// o admin): pintar o no un botón no es una puerta.

// Paneles de la derecha: excluyentes, como en la web.
const (
	PanelChat          = "chat"
	PanelParticipantes = "participantes"
)

var (
	// AnchoPanelLlamada es el `w-72` de los dos paneles.
	AnchoPanelLlamada = tema.Espacio(72)
	// DuracionReaccion es `REACCION_MS`: lo que se ve una reacción antes de irse.
	DuracionReaccion = 4 * time.Second
)

// MensajeChat es una línea del chat de la llamada: un mensaje del canal.
type MensajeChat struct {
	ID, Autor, Texto string
	Mio              bool
}

// Moderacion es lo que la vista pide al servicio; la cumple la Sala con *api.Rooms.
type Moderacion struct {
	Silenciar      func(identidad string) error
	Expulsar       func(identidad string) error
	Palabra        func(identidad string, puede bool) error
	SilenciarTodos func() error
	Bloquear       func(cerrar bool) error
	Cola           func() ([]api.RoomsKnock, error)
	Responder      func(identidad string, admitir bool) error
	Grabar         func(grabar bool) error
}

// reaccionEnVuelo es una reacción que se ve hasta que caduca.
type reaccionEnVuelo struct {
	emoji, quien string
	hasta        time.Time
}

// extrasLlamada es el estado del resto de la barra.
type extrasLlamada struct {
	// Panel es el abierto ("", PanelChat o PanelParticipantes).
	Panel string
	// Chat es el del canal de la llamada; nil (en una reunión suelta), no hay chat.
	Chat         []MensajeChat
	ConChat      bool
	AlEnviarChat func(string)
	// Modera es ser anfitrión o admin; Mod, cómo se pide.
	Modera bool
	Mod    Moderacion
	// Bloqueada es la puerta echada; Cola, quién espera.
	Bloqueada bool
	Cola      []api.RoomsKnock
	// Fallo es el aviso rojo de una moderación que el servicio rechazó.
	Fallo string
	// AlCopiarEnlace copia el enlace de la reunión; AlDispositivos abre su diálogo.
	AlCopiarEnlace, AlDispositivos func()

	sinPalabra     map[string]bool
	reacciones     []reaccionEnVuelo
	grabPendiente  bool
	enlaceCopiado  bool
	entradaChat    *ui.Entrada
	menuReacciones *ui.Menu
	menuMas        *ui.Menu
}

// AbrirPanel abre (o cierra, si ya lo está) un panel de la derecha.
func (v *VistaLlamada) AbrirPanel(p string) {
	if v.Panel == p {
		p = ""
	}
	v.Panel = p
	if p == PanelParticipantes {
		v.mirarPuerta()
	}
	v.Refresh()
}

// Senal pinta una señal que llega: la reacción flota DuracionReaccion.
func (v *VistaLlamada) Senal(quien string, s llamada.Senal) {
	if s.Tipo != "reaccion" {
		v.Refresh() // la mano: el número de turno del panel
		return
	}
	v.reacciones = append(v.reacciones, reaccionEnVuelo{emoji: s.Emoji, quien: quien, hasta: time.Now().Add(DuracionReaccion)})
	v.Refresh()
}

// caducar quita las reacciones que ya se vieron. Lo llama el reloj de la Sala.
func (v *VistaLlamada) caducar(ahora time.Time) {
	vivas := v.reacciones[:0]
	for _, r := range v.reacciones {
		if r.hasta.After(ahora) {
			vivas = append(vivas, r)
		}
	}
	if len(vivas) != len(v.reacciones) {
		v.reacciones = vivas
		v.Refresh()
	}
}

// Reaccionar manda una de las seis reacciones.
func (v *VistaLlamada) Reaccionar(emoji string) {
	v.Lanzar(func() { _ = v.L.Senal(llamada.Reaccion(emoji)) }) // una reacción que no sale no se reintenta
}

// AlternarMano levanta o baja la mano propia.
func (v *VistaLlamada) AlternarMano() {
	alzada := !v.manoPropia()
	v.Lanzar(func() {
		_ = v.L.Senal(llamada.Mano(alzada))
		fyne.Do(v.Refresh)
	})
}

func (v *VistaLlamada) manoPropia() bool {
	for _, p := range v.L.Participantes() {
		if p.Yo {
			return p.ManoAlzada
		}
	}
	return false
}

// Recibir añade un mensaje del canal al chat de la llamada.
func (v *VistaLlamada) Recibir(m MensajeChat) {
	for _, c := range v.Chat {
		if c.ID == m.ID {
			return
		}
	}
	v.Chat = append(v.Chat, m)
	if v.Panel == PanelChat {
		v.Refresh()
	}
}

// EnviarChat manda lo escrito al canal.
func (v *VistaLlamada) EnviarChat() {
	if v.entradaChat == nil || v.AlEnviarChat == nil {
		return
	}
	texto := v.entradaChat.Texto()
	if len([]rune(texto)) == 0 {
		return
	}
	v.AlEnviarChat(texto)
	v.entradaChat.PonerTexto("")
}

// moderar pide una acción al servicio y dice si la rechazó.
func (v *VistaLlamada) moderar(f func() error, luego func()) {
	if f == nil {
		return
	}
	v.Fallo = ""
	v.Lanzar(func() {
		err := f()
		fyne.Do(func() {
			if err != nil {
				v.Fallo = v.T.T("meet.moderationFailed", nil)
			} else if luego != nil {
				luego()
			}
			v.Refresh()
		})
	})
}

// mirarPuerta trae la cola de quien espera, si la puerta está echada y se modera.
func (v *VistaLlamada) mirarPuerta() {
	if !v.Modera || !v.Bloqueada || v.Mod.Cola == nil {
		v.Cola = nil
		return
	}
	v.Lanzar(func() {
		cola, err := v.Mod.Cola()
		fyne.Do(func() {
			if err == nil {
				v.Cola = cola
				v.Refresh()
			}
		})
	})
}

// AlternarGrabacion empieza o para la grabación. Que está grabando lo dice el SFU a
// todos (la cabecera); hasta entonces el botón dice «Empezando…».
func (v *VistaLlamada) AlternarGrabacion() {
	if !v.Modera || v.Mod.Grabar == nil {
		return
	}
	grabar := !v.L.Grabando()
	v.grabPendiente = true
	v.moderar(func() error { return v.Mod.Grabar(grabar) }, nil)
}

// AlternarBloqueo cierra o reabre la puerta; si el servicio dice que no, se deshace.
func (v *VistaLlamada) AlternarBloqueo() {
	cerrar := !v.Bloqueada
	v.moderar(func() error { return v.Mod.Bloquear(cerrar) }, func() {
		v.Bloqueada = cerrar
		v.mirarPuerta()
	})
}

// botonesExtra son los de la barra entre compartir y salir.
func (v *VistaLlamada) botonesExtra() []fyne.CanvasObject {
	t := v.T
	icono := func(i iconos.Icono, rotulo string, activo bool, f func()) *ui.Boton {
		variante := ui.Secundario
		if activo {
			variante = ui.Primario
		}
		b := ui.NuevoBoton("", f).Con(variante, ui.TallaSM).ConIcono(i, 18)
		b.Rotulo = rotulo
		return b
	}
	var out []fyne.CanvasObject
	if v.ConChat {
		out = append(out, icono(iconos.Comentario, t.T("meet.chat", nil), v.Panel == PanelChat,
			func() { v.AbrirPanel(PanelChat) }))
	}
	n := len(v.L.Participantes())
	out = append(out, icono(iconos.Usuarios, fmt.Sprintf("%s (%d)", t.T("meet.participants", nil), n),
		v.Panel == PanelParticipantes, func() { v.AbrirPanel(PanelParticipantes) }))
	var reaccionar *ui.Boton
	reaccionar = icono(iconos.Mano, t.T("meet.react", nil), false, func() {
		elems := make([]fyne.CanvasObject, 0, len(llamada.Reacciones))
		for _, e := range llamada.Reacciones {
			elems = append(elems, ui.NuevoElementoMenu(e, iconos.Ninguno, func() { v.Reaccionar(e) }))
		}
		v.menuReacciones = ui.NuevoMenu(elems...)
		v.menuReacciones.Rotulo = t.T("meet.react", nil)
		v.menuReacciones.Mostrar(reaccionar)
	})
	out = append(out, reaccionar)
	var mas *ui.Boton
	mas = icono(iconos.Puntos, t.T("meet.more", nil), false, func() {
		v.menuMas = ui.NuevoMenu(v.elementosMas()...)
		v.menuMas.Rotulo = t.T("meet.more", nil)
		v.menuMas.Mostrar(mas)
	})
	return append(out, mas)
}

// elementosMas es el «…»: lo secundario.
func (v *VistaLlamada) elementosMas() []fyne.CanvasObject {
	t := v.T
	var out []fyne.CanvasObject
	if v.AlDispositivos != nil {
		out = append(out, ui.NuevoElementoMenu(t.T("meet.devices", nil), iconos.Ninguno, v.AlDispositivos))
	}
	if v.AlCopiarEnlace != nil {
		rotulo := t.T("meet.copyLink", nil)
		if v.enlaceCopiado {
			rotulo = t.T("meet.linkCopied", nil)
		}
		out = append(out, ui.NuevoElementoMenu(rotulo, iconos.Ninguno, func() {
			v.AlCopiarEnlace()
			v.enlaceCopiado = true
		}))
	}
	// La vista: mosaico u orador (el último que habló en grande).
	modo := t.T("meet.layoutSpeaker", nil)
	if v.Modo == ModoOrador {
		modo = t.T("meet.layoutGrid", nil)
	}
	out = append(out, ui.NuevoElementoMenu(modo, iconos.Ninguno, v.AlternarModo))
	mano := t.T("meet.raiseHand", nil)
	if v.manoPropia() {
		mano = t.T("meet.lowerHand", nil)
	}
	out = append(out, ui.NuevoElementoMenu(mano, iconos.Ninguno, v.AlternarMano))
	if v.Modera {
		if v.Mod.Grabar != nil {
			grabar := t.T("meet.startRecording", nil)
			switch {
			case v.grabPendiente && !v.L.Grabando():
				grabar = t.T("meet.recordingStarting", nil)
			case v.L.Grabando():
				grabar = t.T("meet.stopRecording", nil)
			}
			out = append(out, ui.NuevoElementoMenu(grabar, iconos.Ninguno, v.AlternarGrabacion))
		}
		out = append(out,
			ui.NuevoElementoMenu(t.T("meet.muteEveryone", nil), iconos.Ninguno, func() {
				v.moderar(func() error { return v.Mod.SilenciarTodos() }, nil)
			}))
		bloqueo := t.T("meet.lockRoom", nil)
		if v.Bloqueada {
			bloqueo = t.T("meet.unlockRoom", nil)
		}
		out = append(out, ui.NuevoElementoMenu(bloqueo, iconos.Ninguno, v.AlternarBloqueo))
	}
	return out
}

// panel es el de la derecha abierto, o nil.
func (v *VistaLlamada) panel(vr tema.Variante) fyne.CanvasObject {
	switch v.Panel {
	case PanelChat:
		return v.cajaPanel(vr, v.panelChat(vr))
	case PanelParticipantes:
		return v.cajaPanel(vr, v.panelParticipantes(vr))
	}
	return nil
}

func tituloPanel(s string) fyne.CanvasObject {
	return ui.NuevoRotulo(s, tema.TextoSM, ui.SemiNegrita, "neutral-0")
}

func textoPanel(s, tinta string, vr tema.Variante) fyne.CanvasObject {
	return ui.NuevoParrafo(s, tema.TextoXS, ui.Normal, tinta, vr)
}

// panelChat es el chat del canal: los últimos mensajes y la caja para escribir.
func (v *VistaLlamada) panelChat(vr tema.Variante) fyne.CanvasObject {
	t := v.T
	var filas []fyne.CanvasObject
	if len(v.Chat) == 0 {
		filas = append(filas, textoPanel(t.T("meet.chatEmpty", nil), "neutral-400", vr))
	}
	for _, m := range v.Chat {
		filas = append(filas, textoPanel(m.Autor, "neutral-400", vr), textoPanel(m.Texto, "neutral-200", vr))
	}
	if v.entradaChat == nil {
		v.entradaChat = ui.NuevaEntrada(t.T("meet.chatPlaceholder", nil))
	}
	enviar := ui.NuevoBoton(t.T("meet.chatSend", nil), v.EnviarChat).Con(ui.Primario, ui.TallaSM)
	lista := container.NewVScroll(container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...))
	caja := container.NewBorder(nil, nil, nil, enviar, v.entradaChat)
	return container.NewBorder(tituloPanel(t.T("meet.chat", nil)), caja, nil, nil, lista)
}

// panelParticipantes es la cola de la puerta, quién está (con su turno de mano) y, para
// quien modera, sus tres acciones por persona.
func (v *VistaLlamada) panelParticipantes(vr tema.Variante) fyne.CanvasObject {
	t := v.T
	ps := v.L.Participantes()
	filas := v.filasPuerta(vr)
	filas = append(filas, tituloPanel(fmt.Sprintf("%s (%d)", t.T("meet.participants", nil), len(ps))))
	turnos := map[string]int{}
	for _, p := range ps {
		if p.ManoAlzada {
			turnos[p.Identidad] = len(turnos) + 1
		}
	}
	for _, p := range ps {
		filas = append(filas, v.filasParticipante(p, turnos[p.Identidad], vr)...)
	}
	if v.Bloqueada {
		filas = append(filas, textoPanel(t.T("meet.roomLockedNotice", nil), "neutral-400", vr))
	}
	if v.Fallo != "" {
		filas = append(filas, textoPanel(v.Fallo, "danger-text", vr))
	}
	return container.NewVScroll(container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...))
}

// filasPuerta es quién espera en la puerta, con «Dejar entrar» y «No dejar entrar».
func (v *VistaLlamada) filasPuerta(vr tema.Variante) []fyne.CanvasObject {
	if !v.Modera || !v.Bloqueada || len(v.Cola) == 0 {
		return nil
	}
	t := v.T
	filas := []fyne.CanvasObject{tituloPanel(fmt.Sprintf("%s (%d)", t.T("meet.knocking", nil), len(v.Cola)))}
	for _, k := range v.Cola {
		quien := k.Name
		if quien == "" {
			quien = k.Identity
		}
		admitir := ui.NuevoBoton(t.T("meet.admit", nil), v.responder(k.Identity, true)).Con(ui.Primario, ui.TallaXS)
		negar := ui.NuevoBoton(t.T("meet.deny", nil), v.responder(k.Identity, false)).Con(ui.Secundario, ui.TallaXS)
		filas = append(filas, container.NewBorder(nil, nil, nil,
			container.New(ui.Fila{Hueco: tema.Espacio(1)}, admitir, negar), textoPanel(quien, "neutral-200", vr)))
	}
	return filas
}

// filasParticipante es una persona del panel: su nombre con el NÚMERO de turno de la
// mano —no un icono: con cinco manos alzadas lo que el anfitrión necesita saber es a
// quién le toca— y, si se modera, silenciar, la palabra y expulsar.
func (v *VistaLlamada) filasParticipante(p llamada.Participante, turno int, vr tema.Variante) []fyne.CanvasObject {
	t := v.T
	nombre := p.Nombre
	if nombre == "" {
		nombre = p.Identidad
	}
	if turno > 0 {
		nombre = strconv.Itoa(turno) + " · " + nombre
	}
	if p.Hablando {
		nombre += " · " + t.T("meet.speaking", nil)
	}
	if !v.Modera || p.Yo {
		return []fyne.CanvasObject{textoPanel(nombre, "neutral-200", vr)}
	}
	id := p.Identidad
	silenciar := ui.NuevoBoton(t.T("meet.mute", nil), func() {
		v.moderar(func() error { return v.Mod.Silenciar(id) }, nil)
	}).Con(ui.Secundario, ui.TallaXS)
	silenciar.Rotulo = t.T("meet.muteParticipant", nil) + ": " + nombre
	clavePal := "meet.revokeSpeech"
	if v.sinPalabra[id] {
		clavePal = "meet.grantSpeech"
	}
	palabra := ui.NuevoBoton(t.T(clavePal, nil), func() { v.AlternarPalabra(id) }).Con(ui.Secundario, ui.TallaXS)
	expulsar := ui.NuevoBoton(t.T("meet.remove", nil), func() {
		v.moderar(func() error { return v.Mod.Expulsar(id) }, nil)
	}).Con(ui.Peligro, ui.TallaXS)
	expulsar.Rotulo = t.T("meet.removeParticipant", nil) + ": " + nombre
	return []fyne.CanvasObject{
		textoPanel(nombre, "neutral-200", vr),
		container.New(ui.Fila{Hueco: tema.Espacio(1)}, silenciar, palabra, expulsar),
	}
}

// AlternarPalabra quita la palabra a alguien o se la devuelve: el escalón entre
// silenciar (que el otro deshace) y expulsar.
func (v *VistaLlamada) AlternarPalabra(id string) {
	puede := v.sinPalabra[id] // sin palabra: se le devuelve
	v.moderar(func() error { return v.Mod.Palabra(id, puede) }, func() {
		if v.sinPalabra == nil {
			v.sinPalabra = map[string]bool{}
		}
		v.sinPalabra[id] = !puede
	})
}

func (v *VistaLlamada) responder(identidad string, admitir bool) func() {
	return func() {
		// Fuera de la cola YA, como la web: el siguiente vistazo confirma.
		cola := v.Cola[:0:0]
		for _, k := range v.Cola {
			if k.Identity != identidad {
				cola = append(cola, k)
			}
		}
		v.Cola = cola
		v.moderar(func() error { return v.Mod.Responder(identidad, admitir) }, nil)
	}
}

// cajaPanel es `border-l bg-neutral-900 p-3` del panel.
func (v *VistaLlamada) cajaPanel(vr tema.Variante, dentro fyne.CanvasObject) fyne.CanvasObject {
	fondo := canvas.NewRectangle(tema.Color("neutral-900", vr))
	filete := canvas.NewRectangle(conAlfa(tema.Color("neutral-0", vr), 0x1a))
	m := tema.Espacio(3)
	return container.NewStack(fondo, container.NewBorder(nil, nil, container.New(anchoFijo(tema.Borde), filete), nil,
		container.New(ui.Margen{Izq: m, Arr: m, Der: m, Abj: m}, dentro)))
}

// grabacion es el aviso «Grabando» de la cabecera: lo ve TODO el mundo, no solo quien
// lo arranca (en la UE grabar a alguien sin que lo sepa no es una decisión de producto).
func (v *VistaLlamada) grabacion(vr tema.Variante) fyne.CanvasObject {
	if !v.L.Grabando() {
		v.grabPendiente = false
		return nil
	}
	v.grabPendiente = false
	fondo := canvas.NewRectangle(tema.Color("danger", vr))
	fondo.CornerRadius = tema.RadioMD
	rotulo := ui.NuevoRotulo(v.T.T("meet.recording", nil), tema.TextoXS, ui.SemiNegrita, "neutral-0")
	m := tema.Espacio(2)
	return container.NewStack(fondo, container.New(ui.Margen{Izq: m, Der: m, Arr: tema.Espacio(0.5), Abj: tema.Espacio(0.5)}, rotulo))
}

// flotantes son las reacciones en vuelo, centradas sobre la barra.
func (v *VistaLlamada) flotantes() fyne.CanvasObject {
	if len(v.reacciones) == 0 {
		return nil
	}
	objs := make([]fyne.CanvasObject, 0, len(v.reacciones))
	for _, r := range v.reacciones {
		objs = append(objs, ui.NuevoRotulo(r.emoji, tema.Texto2XL, ui.Normal, "neutral-0"))
	}
	return container.New(ui.Fila{Hueco: tema.Espacio(2)}, objs...)
}
