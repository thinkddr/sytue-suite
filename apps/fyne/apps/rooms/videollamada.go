// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"encoding/json"
	"errors"
	"image/color"
	"math"
	"net/http"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La llamada NATIVA: VideoRoom.tsx de la web, contra el mismo SFU y con el mismo token
// (`POST /rooms/{id}/join`). El transporte y los códecs son `apps/fyne/llamada`; esto es
// lo que se ve: la antesala, la rejilla de teselas y la barra.
//
// Se abre en su PROPIA ventana. En la web la llamada tapa el canal, porque una pestaña
// no tiene más sitio; en escritorio el canal sigue a mano y la llamada se mueve aparte,
// que es lo que la web busca con «Sacar a otra ventana». En móvil una ventana nueva es
// la pantalla entera, que es lo mismo que la web en un teléfono.

// EnCurso es la llamada conectada que la vista pinta; la cumple *llamada.Llamada.
type EnCurso interface {
	Participantes() []llamada.Participante
	Micro(abierto bool) error
	Camara(encendida bool) error
	Pantalla(encendida bool) error
	Senal(s llamada.Senal) error
	Grabando() bool
	CambiarAudio(d llamada.Dispositivo) error
	Reconectando() bool
	Colgar()
}

var _ EnCurso = (*llamada.Llamada)(nil)

// Llamador es lo que la llamada pide al servicio; lo cumple *api.Rooms. Es aparte de
// Reunidor porque sin él la sala se abre en la web, como antes.
type Llamador interface {
	JoinMeeting(ctx context.Context, id, nombre, clave string) (api.RoomsCall, error)
	CallChannel(ctx context.Context, canal, titulo, nombre string) (api.RoomsCall, error)
	KnockChannel(ctx context.Context, canal string) error
	PostCall(ctx context.Context, canal string) error
}

var _ Llamador = (*api.Rooms)(nil)

// Moderador es lo que modera la llamada; lo cumple *api.Rooms.
type Moderador interface {
	MuteMeetingParticipant(ctx context.Context, id, identidad string) error
	RemoveMeetingParticipant(ctx context.Context, id, identidad string) error
	SetMeetingSpeech(ctx context.Context, id, identidad string, puede bool) error
	MuteAllMeeting(ctx context.Context, id string) (int, error)
	LockMeeting(ctx context.Context, id string, cerrar bool) error
	MeetingKnocks(ctx context.Context, id string) ([]api.RoomsKnock, error)
	AnswerMeetingKnock(ctx context.Context, id, identidad string, admitir bool) error
	RecordMeeting(ctx context.Context, id string, grabar bool) error
}

var _ Moderador = (*api.Rooms)(nil)

// moderacionDe ata la moderación a una sala.
func (sa *Sala) moderacionDe(id string) Moderacion {
	m, ok := sa.S.(Moderador)
	if !ok {
		return Moderacion{}
	}
	ctx := func() context.Context { return sa.ctx }
	return Moderacion{
		Silenciar:      func(q string) error { return m.MuteMeetingParticipant(ctx(), id, q) },
		Expulsar:       func(q string) error { return m.RemoveMeetingParticipant(ctx(), id, q) },
		Palabra:        func(q string, puede bool) error { return m.SetMeetingSpeech(ctx(), id, q, puede) },
		SilenciarTodos: func() error { _, err := m.MuteAllMeeting(ctx(), id); return err },
		Bloquear:       func(cerrar bool) error { return m.LockMeeting(ctx(), id, cerrar) },
		Cola:           func() ([]api.RoomsKnock, error) { return m.MeetingKnocks(ctx(), id) },
		Responder:      func(q string, admitir bool) error { return m.AnswerMeetingKnock(ctx(), id, q, admitir) },
		Grabar:         func(grabar bool) error { return m.RecordMeeting(ctx(), id, grabar) },
	}
}

// mensajeChat es un mensaje del canal como línea del chat de la llamada.
func (sa *Sala) mensajeChat(m api.RoomsMessage) MensajeChat {
	autor := m.AuthorName
	if autor == "" {
		autor = m.AuthorSub
	}
	return MensajeChat{ID: m.ID, Autor: autor, Texto: m.Body, Mio: m.AuthorSub == sa.Yo}
}

// Reintentos de la puerta: la web reintenta entrar cada 8 s y se rinde a las 38 vueltas
// (unos cinco minutos), para no dejar un temporizador latiendo en una ventana olvidada.
const (
	PausaPuerta    = 8 * time.Second
	IntentosPuerta = 38
	// AnilloHablando es el `shadow-[0_0_0_2px]` de quien habla.
	AnilloHablando float32 = 2
)

// ---- La vista ----

// VistaLlamada es VideoRoom.tsx conectado: la cabecera con el título y el aforo, la
// rejilla y la barra de controles.
type VistaLlamada struct {
	widget.BaseWidget
	T      *i18n.Translator
	Titulo string
	L      EnCurso

	AlColgar func()
	// ConCamara y ConPantalla dicen si hay cómo abrirlas; sin ellas, el botón está
	// apagado e inerte (la cámara) o no está (compartir).
	ConCamara, ConPantalla bool
	// Esperar es cómo se lanza lo que bloquea (abrir la cámara pide permiso); los
	// guardianes lo hacen síncrono.
	Lanzar func(func())

	micro, camara, pantalla bool
	// Modo es mosaico u orador; Fijado, la tesela fijada a mano; ultimoHablante, el
	// último que habló (escenario.go).
	Modo, Fijado, ultimoHablante string
	protagonista                 string
	extrasLlamada
	panelObj, grabObj, flotObj fyne.CanvasObject
	fondo                      *canvas.Rectangle
	filete                     *canvas.Rectangle
	titulo                     *canvas.Text
	aforo                      *canvas.Text
	teselas                    []*TeselaLlamada
	rejilla                    *fyne.Container
	barra                      *fyne.Container
	objetos                    []fyne.CanvasObject
}

var _ fyne.Widget = (*VistaLlamada)(nil)

// NuevaVistaLlamada crea la vista con el micro como se eligió en la antesala.
func NuevaVistaLlamada(t *i18n.Translator, titulo string, l EnCurso, micro bool) *VistaLlamada {
	v := &VistaLlamada{T: t, Titulo: titulo, L: l, micro: micro, Modo: ModoMosaico, Lanzar: func(f func()) { go f() }}
	v.ExtendBaseWidget(v)
	return v
}

// AccessibilityLabel es el título de la sala.
func (v *VistaLlamada) AccessibilityLabel() string { return v.Titulo }

// AccessibilityRole implementa fyne.Accessible.
func (v *VistaLlamada) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

// Micro dice si el micrófono está abierto, para los guardianes.
func (v *VistaLlamada) Micro() bool { return v.micro }

// AlternarMicro es el primer botón de la barra.
func (v *VistaLlamada) AlternarMicro() {
	if err := v.L.Micro(!v.micro); err != nil {
		v.micro = false // sin permiso o sin tarjeta: cerrado, no roto
	} else {
		v.micro = !v.micro
	}
	v.Refresh()
}

// componerExtras rehace el panel, el aviso de grabación y las reacciones en vuelo.
func (v *VistaLlamada) componerExtras(vr tema.Variante) {
	v.panelObj, v.grabObj, v.flotObj = v.panel(vr), v.grabacion(vr), v.flotantes()
	objs := v.objetos[:6]
	for _, o := range []fyne.CanvasObject{v.flotObj, v.panelObj, v.grabObj} {
		if o != nil {
			objs = append(objs, o)
		}
	}
	v.objetos = objs
}

// Camara dice si la cámara propia está encendida; Pantalla, si se comparte.
func (v *VistaLlamada) Camara() bool   { return v.camara }
func (v *VistaLlamada) Pantalla() bool { return v.pantalla }

// AlternarCamara es el segundo botón. Abrir la cámara puede pedir permiso y tardar: va
// fuera del hilo de la interfaz y el botón cambia cuando contesta.
func (v *VistaLlamada) AlternarCamara() {
	quiero := !v.camara
	v.Lanzar(func() {
		err := v.L.Camara(quiero)
		fyne.Do(func() {
			v.camara = quiero && err == nil
			v.Refresh()
		})
	})
}

// AlternarPantalla es «Compartir pantalla» y «Dejar de compartir».
func (v *VistaLlamada) AlternarPantalla() {
	quiero := !v.pantalla
	v.Lanzar(func() {
		err := v.L.Pantalla(quiero)
		fyne.Do(func() {
			v.pantalla = quiero && err == nil
			v.Refresh()
		})
	})
}

// Medidas: la cabecera `h-14 px-4`, el escenario `p-3` con `gap-3`, la barra flotante
// `bottom-4` y el hueco que le deja el escenario (`pb-20`).
var (
	AltoCabeceraLlamada = tema.Espacio(14)
	AireEscenario       = tema.Espacio(3)
	HuecoEscenario      = tema.Espacio(3)
	BajoEscenario       = tema.Espacio(20)
)

// Columnas es `cols` de la web: 1, 2 hasta cuatro, 3 a partir de cinco.
func Columnas(n int) int {
	switch {
	case n <= 1:
		return 1
	case n <= 4:
		return 2
	}
	return 3
}

// CreateRenderer compone la vista.
func (v *VistaLlamada) CreateRenderer() fyne.WidgetRenderer {
	vr := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	v.fondo = canvas.NewRectangle(tema.Color("neutral-1000", vr))
	v.filete = canvas.NewRectangle(conAlfa(tema.Color("neutral-0", vr), 0x1a))
	v.titulo = ui.Texto(v.Titulo, tema.TextoBase, ui.SemiNegrita, tema.Color("neutral-0", vr))
	v.aforo = ui.Texto("", tema.TextoSM, ui.Normal, tema.Color("neutral-400", vr))
	v.rejilla = container.NewWithoutLayout()
	v.barra = container.New(ui.Fila{Hueco: tema.Espacio(1)})
	v.objetos = []fyne.CanvasObject{v.fondo, v.filete, v.titulo, v.aforo, v.rejilla, v.barra}
	v.componerExtras(vr)
	r := &vistaLlamadaRenderer{v: v}
	r.Refresh()
	return r
}

// RefrescarVideo cambia solo los fotogramas, sin rehacer la rejilla: es lo que llega 30
// veces por segundo.
//
// Por identidad y no por posición: la pantalla compartida y el protagonista cambian el
// orden de las teselas respecto al de los participantes.
func (v *VistaLlamada) RefrescarVideo() {
	porID := map[string]llamada.Participante{}
	for _, p := range v.L.Participantes() {
		porID[p.Identidad] = p
	}
	for _, t := range v.teselas {
		if p, ok := porID[t.Identidad]; ok {
			t.PonerImagen(p)
		}
	}
}

// AlternarFijado fija una tesela en grande, o la suelta si ya lo estaba. Fijarse a uno
// mismo también vale (para revisar el encuadre).
func (v *VistaLlamada) AlternarFijado(id string) {
	if v.Fijado == id {
		id = ""
	}
	v.Fijado = id
	v.Refresh()
}

// AlternarModo pasa de mosaico a orador y vuelta.
func (v *VistaLlamada) AlternarModo() {
	if v.Modo == ModoOrador {
		v.Modo = ModoMosaico
	} else {
		v.Modo = ModoOrador
	}
	v.Refresh()
}

// Protagonista es la tesela que va en grande, para los guardianes.
func (v *VistaLlamada) Protagonista() string { return v.protagonista }

type vistaLlamadaRenderer struct{ v *VistaLlamada }

func (r *vistaLlamadaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(320, AltoCabeceraLlamada+BajoEscenario+120)
}
func (r *vistaLlamadaRenderer) Objects() []fyne.CanvasObject { return r.v.objetos }
func (r *vistaLlamadaRenderer) Destroy()                     {}

func (r *vistaLlamadaRenderer) Layout(t fyne.Size) {
	v := r.v
	v.fondo.Resize(t)
	v.filete.Move(fyne.NewPos(0, AltoCabeceraLlamada-tema.Borde))
	v.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	x := tema.Espacio(4)
	ancho := ui.Medir(v.titulo.Text, tema.TextoBase, ui.SemiNegrita).Width
	ui.ColocarTexto(v.titulo, tema.TextoBase, x, (AltoCabeceraLlamada-tema.TextoBase.Linea)/2)
	ui.ColocarTexto(v.aforo, tema.TextoSM, x+ancho+tema.Espacio(2), (AltoCabeceraLlamada-tema.TextoSM.Linea)/2)

	// El escenario: la rejilla de `cols` columnas con filas iguales, o la compartida
	// arriba a lo ancho con dos alturas y el resto debajo, como `enGrande`.
	origen := fyne.NewPos(AireEscenario, AltoCabeceraLlamada+AireEscenario)
	area := fyne.NewSize(t.Width-2*AireEscenario, max(t.Height-AltoCabeceraLlamada-AireEscenario-BajoEscenario, 0))
	n := len(v.teselas)
	if n > 0 {
		cols := Columnas(n)
		grande := v.protagonista != ""
		filas := (n + cols - 1) / cols
		if grande {
			filas = 2 + (n-1+cols-1)/cols
		}
		anchoCol := (area.Width - float32(cols-1)*HuecoEscenario) / float32(cols)
		altoFila := (area.Height - float32(filas-1)*HuecoEscenario) / float32(filas)
		fila, col := 0, 0
		for i, ts := range v.teselas {
			if i == 0 && grande {
				ts.Move(origen)
				ts.Resize(fyne.NewSize(area.Width, 2*altoFila+HuecoEscenario))
				fila = 2
				continue
			}
			ts.Move(origen.Add(fyne.NewPos(float32(col)*(anchoCol+HuecoEscenario), float32(fila)*(altoFila+HuecoEscenario))))
			ts.Resize(fyne.NewSize(anchoCol, altoFila))
			col++
			if col == cols {
				col, fila = 0, fila+1
			}
		}
	}
	// La barra flotante, centrada a `bottom-4`.
	m := v.barra.MinSize()
	v.barra.Move(fyne.NewPos((t.Width-m.Width)/2, t.Height-tema.Espacio(4)-m.Height))
	v.barra.Resize(m)
	// «Grabando» a la derecha de la cabecera (`ml-auto`), las reacciones sobre la barra
	// (`bottom-24`) y el panel pegado a la derecha bajo la cabecera (`w-72`).
	if v.grabObj != nil {
		g := v.grabObj.MinSize()
		v.grabObj.Move(fyne.NewPos(t.Width-tema.Espacio(4)-g.Width, (AltoCabeceraLlamada-g.Height)/2))
		v.grabObj.Resize(g)
	}
	if v.flotObj != nil {
		f := v.flotObj.MinSize()
		v.flotObj.Move(fyne.NewPos((t.Width-f.Width)/2, t.Height-tema.Espacio(24)-f.Height))
		v.flotObj.Resize(f)
	}
	if v.panelObj != nil {
		v.panelObj.Move(fyne.NewPos(max(t.Width-AnchoPanelLlamada, 0), AltoCabeceraLlamada))
		v.panelObj.Resize(fyne.NewSize(min(AnchoPanelLlamada, t.Width), max(t.Height-AltoCabeceraLlamada, 0)))
	}
}

func (r *vistaLlamadaRenderer) Refresh() {
	v := r.v
	t := v.T
	ps := v.L.Participantes()
	v.aforo.Text = "· " + t.T("meet.inRoom", i18n.Vars{"count": len(ps)})
	if v.L.Reconectando() {
		v.aforo.Text = "· " + t.T("meet.reconnecting", nil)
	}
	v.aforo.Refresh()

	// Una tesela por participante y, delante, la pantalla que alguien comparta.
	var teselas []*TeselaLlamada
	compartiendo := ""
	for _, p := range ps {
		if p.Compartida != nil {
			teselas = append(teselas, NuevaTeselaLlamada(t, p, true))
			if compartiendo == "" {
				compartiendo = idPantalla(p.Identidad)
			}
		}
	}
	for _, p := range ps {
		ts := NuevaTeselaLlamada(t, p, false)
		ts.MicroCerrado = p.MicroCerrado || (p.Yo && !v.micro)
		teselas = append(teselas, ts)
	}
	// El último que habló: el primero que habla ahora, y si nadie habla, el de antes
	// (la señal del SFU se vacía en cada pausa).
	for _, p := range ps {
		if p.Hablando {
			v.ultimoHablante = p.Identidad
			break
		}
	}
	presentes := make([]string, len(teselas))
	for i, ts := range teselas {
		presentes[i] = ts.ID()
		id := ts.ID()
		ts.Fijada = v.Fijado == id
		ts.AlFijar = func() { v.AlternarFijado(id) }
	}
	v.protagonista = Protagonista(Escena{
		Modo: v.Modo, Fijado: v.Fijado, UltimoHablante: v.ultimoHablante,
		Compartiendo: compartiendo, Presentes: presentes,
	})
	teselas = OrdenarPorProtagonista(teselas, v.protagonista)
	v.teselas = teselas
	objs := make([]fyne.CanvasObject, len(teselas))
	for i, ts := range teselas {
		objs[i] = ts
	}
	v.rejilla.Objects = objs
	v.rejilla.Refresh()

	claveMicro, glifoMicro, varMicro := "meet.unmuteMic", iconos.MicrofonoApagado, ui.Peligro
	if v.micro {
		claveMicro, glifoMicro, varMicro = "meet.muteMic", iconos.Microfono, ui.Secundario
	}
	micro := ui.NuevoBoton("", v.AlternarMicro).Con(varMicro, ui.TallaSM).ConIcono(glifoMicro, 18)
	micro.Rotulo = t.T(claveMicro, nil)
	// La cámara: el color dice el estado (rojo apagada), el icono lo que es y el nombre
	// accesible la acción, como la web. Sin cámara que abrir, inerte.
	claveCam, varCam := "meet.cameraOn", ui.Peligro
	if v.camara {
		claveCam, varCam = "meet.cameraOff", ui.Secundario
	}
	cam := ui.NuevoBoton("", v.AlternarCamara).Con(varCam, ui.TallaSM).ConIcono(iconos.Camara, 18)
	cam.Rotulo, cam.Inerte = t.T(claveCam, nil), !v.ConCamara
	botones := []fyne.CanvasObject{micro, cam}
	if v.ConPantalla {
		clavePan, varPan := "meet.shareScreen", ui.Secundario
		if v.pantalla {
			clavePan, varPan = "meet.stopSharing", ui.Primario
		}
		pantalla := ui.NuevoBoton("", v.AlternarPantalla).Con(varPan, ui.TallaSM).ConIcono(iconos.Pantalla, 18)
		pantalla.Rotulo = t.T(clavePan, nil)
		botones = append(botones, pantalla)
	}
	botones = append(botones, v.botonesExtra()...)
	salir := ui.NuevoBoton(t.T("meet.leave", nil), func() {
		if v.AlColgar != nil {
			v.AlColgar()
		}
	}).Con(ui.Peligro, ui.TallaSM).ConIcono(iconos.Salir, 16)
	botones = append(botones, salir)
	v.barra.Objects = botones
	v.barra.Refresh()
	v.componerExtras(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.Layout(v.Size())
}

// ---- La tesela ----

// TeselaLlamada es una persona (o su pantalla) en la rejilla: `rounded-lg border
// bg-neutral-900`, la inicial en su disco `h-16 w-16` debajo del vídeo, el anillo y la
// píldora «Hablando» de quien habla, y el nombre abajo a la izquierda.
type TeselaLlamada struct {
	widget.BaseWidget
	T *i18n.Translator
	llamada.Participante
	Compartida   bool
	MicroCerrado bool
	// Fijada es la tesela fijada a mano; AlFijar la fija o la suelta (nil: sin botón).
	Fijada  bool
	AlFijar func()

	encima bool
	fijar  *BotonFijar

	// borde y dentro son la caja: dos rectángulos (ver ui.Caja) y no ui.Caja porque el
	// anillo de quien habla ENGORDA el borde (`border` + `shadow 0 0 0 2px`), y ui.Caja
	// tiene el grosor fijo.
	borde, dentro *canvas.Rectangle
	disco         *canvas.Circle
	letra         *canvas.Text
	imagen        *canvas.Image
	chip          *canvas.Rectangle
	nombre        *canvas.Text
	habla         *canvas.Rectangle
	hablaT        *canvas.Text
}

// NuevaTeselaLlamada crea la tesela de un participante; con `compartida`, la de su
// pantalla.
func NuevaTeselaLlamada(t *i18n.Translator, p llamada.Participante, compartida bool) *TeselaLlamada {
	ts := &TeselaLlamada{T: t, Participante: p, Compartida: compartida}
	ts.ExtendBaseWidget(ts)
	return ts
}

// ID es la identidad de la tesela en el escenario: la de la persona, o la de su
// pantalla (`pantalla:<persona>`).
func (ts *TeselaLlamada) ID() string {
	if ts.Compartida {
		return idPantalla(ts.Identidad)
	}
	return ts.Identidad
}

// MouseIn, MouseMoved y MouseOut son el `group-hover` que enseña «Fijar».
func (ts *TeselaLlamada) MouseIn(*desktop.MouseEvent)    { ts.encima = true; ts.Refresh() }
func (ts *TeselaLlamada) MouseMoved(*desktop.MouseEvent) {}
func (ts *TeselaLlamada) MouseOut()                      { ts.encima = false; ts.Refresh() }

// VeFijar dice si el botón de fijar se enseña: al pasar por encima, con el foco, si ya
// está fijada, y siempre en el móvil.
func (ts *TeselaLlamada) VeFijar() bool {
	if ts.AlFijar == nil {
		return false
	}
	return ts.encima || ts.Fijada || (ts.fijar != nil && ts.fijar.Activo()) || ui.EsTactil()
}

// AccessibilityLabel es el nombre, con «(silenciado)» si toca.
func (ts *TeselaLlamada) AccessibilityLabel() string { return ts.rotulo() }

// AccessibilityRole implementa fyne.Accessible.
func (ts *TeselaLlamada) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

func (ts *TeselaLlamada) rotulo() string {
	nombre := ts.Nombre
	if nombre == "" {
		nombre = ts.Identidad
	}
	if ts.Yo {
		nombre += " (" + ts.T.T("meet.you", nil) + ")"
	}
	if ts.MicroCerrado {
		nombre += " " + ts.T.T("meet.mutedSuffix", nil)
	}
	return nombre
}

// fotograma es lo que la tesela enseña: la pantalla compartida o la cámara.
func (ts *TeselaLlamada) fotograma(p llamada.Participante) *canvas.Image {
	img := p.Video
	if ts.Compartida {
		img = p.Compartida
	}
	if img == nil {
		return nil
	}
	c := canvas.NewImageFromImage(img)
	c.FillMode = canvas.ImageFillContain
	c.ScaleMode = canvas.ImageScaleFastest
	return c
}

// PonerImagen cambia el fotograma sin rehacer la tesela.
func (ts *TeselaLlamada) PonerImagen(p llamada.Participante) {
	ts.Video, ts.Participante.Compartida = p.Video, p.Compartida
	img := p.Video
	if ts.Compartida {
		img = p.Compartida
	}
	if ts.imagen == nil || img == nil {
		ts.Refresh()
		return
	}
	ts.imagen.Image = img
	ts.imagen.Refresh()
}

// CreateRenderer compone la tesela.
func (ts *TeselaLlamada) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	blanco := tema.Color("neutral-0", v)
	ts.borde = canvas.NewRectangle(conAlfa(blanco, 0x1a))
	ts.borde.CornerRadius = tema.RadioLG
	ts.dentro = canvas.NewRectangle(tema.Color("neutral-900", v))
	ts.disco = canvas.NewCircle(conAlfa(blanco, 0x1a))
	ts.letra = ui.Texto(inicial(ts.Nombre, ts.Identidad), tema.TextoXL, ui.SemiNegrita, conAlfa(blanco, 0xcc))
	ts.chip = canvas.NewRectangle(conAlfa(tema.Color("neutral-1000", v), 0xb3))
	ts.chip.CornerRadius = 999
	ts.nombre = ui.Texto(ts.rotulo(), tema.TextoXS, ui.Medio, blanco)
	ts.habla = canvas.NewRectangle(tema.Color("primary", v)) // el rol, con su rótulo `on-primary`
	ts.habla.CornerRadius = 999
	ts.hablaT = ui.Texto(ts.T.T("meet.speaking", nil), tema.Texto2XS, ui.SemiNegrita, tema.Color("on-primary", v))
	if ts.AlFijar != nil {
		clave := "meet.pin"
		if ts.Fijada {
			clave = "meet.unpin"
		}
		ts.fijar = NuevoBotonFijar(ts.T.T(clave, nil), ts.Nombre, ts.AlFijar)
		ts.fijar.AlCambio = ts.Refresh
	}
	r := &teselaLlamadaRenderer{ts: ts}
	r.Refresh()
	return r
}

// hablaAhora es el anillo: la pantalla compartida no «habla».
func (ts *TeselaLlamada) hablaAhora() bool { return ts.Hablando && !ts.Compartida }

// grosor es el borde de 1 px, o 3 con el anillo de 2 px por fuera.
func (ts *TeselaLlamada) grosor() float32 {
	if ts.hablaAhora() {
		return AnilloHablando + tema.Borde
	}
	return tema.Borde
}

func inicial(nombre, identidad string) string {
	if nombre == "" {
		nombre = identidad
	}
	for _, c := range nombre {
		return string([]rune{c})
	}
	return "?"
}

type teselaLlamadaRenderer struct{ ts *TeselaLlamada }

func (r *teselaLlamadaRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }
func (r *teselaLlamadaRenderer) Destroy()           {}

func (r *teselaLlamadaRenderer) Objects() []fyne.CanvasObject {
	ts := r.ts
	objs := []fyne.CanvasObject{ts.borde, ts.dentro, ts.disco, ts.letra}
	if ts.imagen != nil {
		objs = append(objs, ts.imagen)
	}
	objs = append(objs, ts.chip, ts.nombre)
	if ts.hablaAhora() {
		objs = append(objs, ts.habla, ts.hablaT)
	}
	if ts.VeFijar() {
		objs = append(objs, ts.fijar)
	}
	return objs
}

func (r *teselaLlamadaRenderer) Layout(t fyne.Size) {
	ts := r.ts
	g := ts.grosor()
	ts.borde.Resize(t)
	ts.dentro.Move(fyne.NewPos(g, g))
	ts.dentro.Resize(fyne.NewSize(max(t.Width-2*g, 0), max(t.Height-2*g, 0)))
	ts.dentro.CornerRadius = max(tema.RadioLG-g, 0)
	d := tema.Espacio(16)
	ts.disco.Move(fyne.NewPos((t.Width-d)/2, (t.Height-d)/2))
	ts.disco.Resize(fyne.NewSize(d, d))
	lw := ui.Medir(ts.letra.Text, tema.TextoXL, ui.SemiNegrita).Width
	ui.ColocarTexto(ts.letra, tema.TextoXL, (t.Width-lw)/2, (t.Height-tema.TextoXL.Linea)/2)
	if ts.imagen != nil {
		ts.imagen.Move(ts.dentro.Position())
		ts.imagen.Resize(ts.dentro.Size())
	}
	// `bottom-2 left-2 px-2 py-0.5`, truncado a `100% - 1rem`.
	px, py := tema.Espacio(2), tema.Espacio(0.5)
	ui.Truncar(ts.nombre, ts.rotulo(), tema.TextoXS, ui.Medio, max(t.Width-tema.Espacio(4)-2*px, 0))
	nw := ui.Medir(ts.nombre.Text, tema.TextoXS, ui.Medio).Width
	ch := tema.TextoXS.Linea + 2*py
	ts.chip.Move(fyne.NewPos(tema.Espacio(2), t.Height-tema.Espacio(2)-ch))
	ts.chip.Resize(fyne.NewSize(nw+2*px, ch))
	ui.ColocarTexto(ts.nombre, tema.TextoXS, tema.Espacio(2)+px, t.Height-tema.Espacio(2)-ch+py)
	hw := ui.Medir(ts.hablaT.Text, tema.Texto2XS, ui.SemiNegrita).Width
	hh := tema.Texto2XS.Linea + 2*py
	ts.habla.Move(fyne.NewPos(tema.Espacio(2), tema.Espacio(2)))
	ts.habla.Resize(fyne.NewSize(hw+2*px, hh))
	ui.ColocarTexto(ts.hablaT, tema.Texto2XS, tema.Espacio(2)+px, tema.Espacio(2)+py)
	if ts.fijar != nil {
		m := ts.fijar.MinSize()
		ts.fijar.Move(fyne.NewPos(t.Width-tema.Espacio(2)-m.Width, tema.Espacio(2)))
		ts.fijar.Resize(m)
	}
}

func (r *teselaLlamadaRenderer) Refresh() {
	ts := r.ts
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ts.borde.FillColor = conAlfa(tema.Color("neutral-0", v), 0x1a)
	if ts.hablaAhora() {
		ts.borde.FillColor = tema.Color("primary-500", v)
	}
	ts.borde.Refresh()
	ts.dentro.FillColor = tema.Color("neutral-900", v)
	ts.dentro.Refresh()
	ts.imagen = ts.fotograma(ts.Participante)
	ts.nombre.Text = ts.rotulo()
	r.Layout(ts.Size())
}

// conAlfa es el `/10`, `/70`… de Tailwind sobre un color opaco.
func conAlfa(c color.NRGBA, a uint8) color.NRGBA {
	c.A = uint8(math.Round(float64(c.A) * float64(a) / 255))
	return c
}

// ---- En la Sala ----

// Conector abre la conexión con el SFU; el de la app es `llamada.Entrar` con la tarjeta
// del sistema (cmd/rooms), y los guardianes ponen un doble.
type Conector func(url, token string, o llamada.Opciones) (EnCurso, error)

// LlamarEnCanal es «Llamar» de la cabecera: la sala del canal (la crea o la reutiliza el
// servicio) con su puerta, si la han echado.
func (sa *Sala) LlamarEnCanal() {
	ll, ok := sa.S.(Llamador)
	if !ok || sa.Conectar == nil || sa.abierto == "" {
		return
	}
	canal, titulo := sa.abierto, sa.P.Canal.Cabecera.Canal.Nombre
	sa.abrirLlamada(titulo, canal,
		func(ctx context.Context) (api.RoomsCall, error) { return ll.CallChannel(ctx, canal, titulo, "") },
		func(ctx context.Context) error { return ll.KnockChannel(ctx, canal) })
}

// EntrarEnReunion entra en una reunión suelta (el panel «Reuniones» o un enlace
// `?rooms=`). Devuelve false si la llamada nativa no está, y entonces quien llama la
// abre en la web.
func (sa *Sala) EntrarEnReunion(id, titulo string) bool {
	ll, ok := sa.S.(Llamador)
	re, conPuerta := sa.S.(Reunidor)
	if !ok || !conPuerta || sa.Conectar == nil {
		return false
	}
	clave := sa.claves[id]
	sa.abrirLlamada(titulo, "",
		func(ctx context.Context) (api.RoomsCall, error) { return ll.JoinMeeting(ctx, id, "", clave) },
		func(ctx context.Context) error { return re.KnockMeeting(ctx, id, clave) })
	return true
}

// abrirLlamada abre la ventana de la llamada con su antesala y entra al pulsar
// «Entrar». Con la puerta echada ofrece llamar y, llamado, reintenta solo cada
// PausaPuerta —la señal de que te han abierto es que entras—. Cerrar la ventana cuelga
// LIMPIO: `Colgar` avisa al SFU en el acto (el `pagehide` de la web).
//
//nolint:gocognit,gocyclo // Encadena la antesala con su previa, la puerta, la conexión y la vista.
func (sa *Sala) abrirLlamada(titulo, canal string, obtener func(context.Context) (api.RoomsCall, error), llamar func(context.Context) error) {
	if sa.ventanaLlamada != nil {
		sa.ventanaLlamada.RequestFocus() // una llamada a la vez, como en la web
		return
	}
	t := sa.P.T
	w := fyne.CurrentApp().NewWindow(titulo)
	sa.ventanaLlamada = w
	a := NuevaAntesala(t, titulo)
	// La cámara empieza APAGADA: se enciende cuando la persona lo pide, que es cuando
	// se pide el permiso (ADR 0046). Sin cámara que abrir, el interruptor no hace nada.
	a.Camara, a.SinCamara = false, sa.Camara == nil
	if sa.PedirMicro != nil {
		_ = sa.PedirMicro() // negado, se entra a escuchar
	}
	var (
		enCurso EnCurso
		vista   *VistaLlamada
		parar   func()
		vueltas int
		entrar  func()
	)
	pa := &previaAntesala{sa: sa, a: a, w: w}
	pa.preparar()
	w.SetOnClosed(func() {
		pa.soltar()
		if parar != nil {
			parar()
		}
		if enCurso != nil {
			enCurso.Colgar()
		}
		sa.ventanaLlamada, sa.llamada, sa.canalLlamada = nil, nil, ""
	})
	a.AlCancelar = w.Close

	llamarPuerta := func() {
		a.Aviso, a.RotuloAccion, a.AlAccion = t.T("meet.knocked", nil), "", nil
		a.Refresh()
		pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, llamar(ctx) },
			func(_ struct{}, err error) {
				var e *api.Error
				switch {
				case err == nil:
					vueltas, parar = 1, sa.Esperar(PausaPuerta, entrar)
				case errors.As(err, &e) && e.Status == http.StatusConflict:
					entrar() // la abrieron entretanto
				default:
					// Si el aviso no sale, el botón vuelve: nadie espera una respuesta que
					// no se llegó a pedir.
					a.Aviso, a.RotuloAccion = t.T("meet.roomLockedNotice", nil), t.T("meet.knock", nil)
					a.Refresh()
				}
			})
	}
	entrar = func() {
		a.Entrando = true
		a.Refresh()
		pedir(sa, obtener, func(c api.RoomsCall, err error) {
			switch {
			case api.ErrSalaCerrada(err):
				a.Entrando = false
				if vueltas > 0 && vueltas < IntentosPuerta {
					vueltas++
					parar = sa.Esperar(PausaPuerta, entrar)
				} else {
					vueltas = 0 // sin llamar, o rendidos: el botón vuelve
					a.Aviso, a.RotuloAccion, a.AlAccion = t.T("meet.roomLockedNotice", nil), t.T("meet.knock", nil), llamarPuerta
				}
				a.Refresh()
				return
			case err != nil:
				aviso := t.T("meet.connectFailed", nil)
				// Una reunión suelta solo admite a quien la convocó, a quien ya estuvo
				// dentro y a quien trae su enlace (la web, en ChatApp.tsx, igual).
				if e := (*api.Error)(nil); errors.As(err, &e) && e.Status == http.StatusForbidden {
					aviso = t.T("meetings.needLink", nil)
				}
				a.Entrando, a.Aviso, a.RotuloAccion = false, aviso, ""
				a.Refresh()
				return
			}
			micro, conCamara := a.Micro, a.Camara
			pa.soltar() // la llamada abre su cámara y su tarjeta
			pedir(sa, func(context.Context) (EnCurso, error) {
				return sa.Conectar(c.LiveKitURL, c.Token, llamada.Opciones{
					Micro:       micro,
					Camara:      sa.Camara,
					Pantalla:    sa.Pantalla,
					AlCambiar:   func() { fyne.Do(func() { refrescarSi(vista) }) },
					AlFotograma: func() { fyne.Do(func() { videoSi(vista) }) },
					AlSenal: func(q string, sn llamada.Senal) {
						fyne.Do(func() {
							if vista == nil {
								return
							}
							vista.Senal(q, sn)
							sa.Esperar(DuracionReaccion+100*time.Millisecond, func() { vista.caducar(time.Now()) })
						})
					},
					AlSalir: func(string) { fyne.Do(w.Close) },
				})
			}, func(l EnCurso, err error) {
				if err != nil {
					a.Entrando, a.Aviso = false, t.T("meet.connectFailed", nil)
					a.Refresh()
					return
				}
				if sa.ventanaLlamada != w {
					l.Colgar() // cerraron la ventana mientras conectaba
					return
				}
				enCurso = l
				vista = NuevaVistaLlamada(t, titulo, l, micro)
				vista.AlColgar = w.Close
				vista.ConCamara, vista.ConPantalla = sa.Camara != nil, sa.Pantalla != nil
				vista.Lanzar = sa.Lanzar
				sa.prepararBarra(vista, c, canal, l)
				if conCamara {
					vista.AlternarCamara() // se entra como se eligió en la antesala
				}
				sa.llamada = vista
				w.SetContent(vista)
				if ll, ok := sa.S.(Llamador); ok && canal != "" {
					// La tarjeta del historial, como `postCallMessage`: si no sale, la
					// llamada sigue.
					pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, ll.PostCall(ctx, canal) },
						func(struct{}, error) {})
				}
			})
		})
	}
	a.AlEntrar = entrar
	w.SetContent(a)
	w.Resize(fyne.NewSize(AnchoAntesala+2*CanalonAntesala, 600))
	w.Show()
}

func refrescarSi(v *VistaLlamada) {
	if v != nil {
		v.Refresh()
	}
}

func videoSi(v *VistaLlamada) {
	if v != nil {
		v.RefrescarVideo()
	}
}

// EntrarPorEnlace es el enlace de una reunión (`?rooms=<id>`) que llega a la app —el App
// Link, el Universal Link o la página de la web que abre la app—: abre su llamada. Si la
// sala aún no ha arrancado (el enlace llegó con la app cerrada y sin sesión), espera a
// Arrancar; sin llamada nativa, se abre en la web como antes.
//
// `clave` es la que trae el enlace (la de una reunión suelta).
func (sa *Sala) EntrarPorEnlace(id, clave string) {
	sa.recordarClave(id, clave)
	if sa.ctx == nil || !sa.arrancada {
		sa.reunionPendiente = id
		return
	}
	if !sa.EntrarEnReunion(id, "") {
		sa.abrirURL(sa.EnlaceReunion(id))
	}
}

// prepararBarra da a la vista el chat del canal, la moderación, el enlace y los
// dispositivos.
func (sa *Sala) prepararBarra(v *VistaLlamada, c api.RoomsCall, canal string, l EnCurso) {
	v.Modera = c.Role == "host" || c.Room.CreatedBy == sa.Yo || sa.adminEquipo
	v.Bloqueada = c.Room.Locked
	v.Mod = sa.moderacionDe(c.Room.ID)
	if sa.Copiar != nil {
		v.AlCopiarEnlace = func() { sa.Copiar(sa.EnlaceReunion(c.Room.ID)) }
	}
	if sa.Dispositivos != nil {
		v.AlDispositivos = func() { sa.abrirDispositivos(v, l) }
	}
	if canal == "" {
		return // una reunión suelta no tiene canal detrás: no hay chat
	}
	v.ConChat = true
	v.AlEnviarChat = func(texto string) {
		pedir(sa, func(ctx context.Context) (api.RoomsMessage, error) { return sa.S.Post(ctx, canal, texto) },
			func(m api.RoomsMessage, err error) {
				if err == nil {
					v.Recibir(sa.mensajeChat(m))
				}
			})
	}
	pedir(sa, func(ctx context.Context) ([]api.RoomsMessage, error) {
		return sa.S.Messages(ctx, canal, time.Time{}, 30)
	},
		func(ms []api.RoomsMessage, err error) {
			if err != nil {
				return
			}
			for _, m := range ms {
				if m.Type != "call" && m.ParentID == nil {
					v.Recibir(sa.mensajeChat(m))
				}
			}
		})
	sa.canalLlamada = canal
}

// chatDeLaLlamada lleva al chat de la llamada lo que llega en vivo a su canal.
func (sa *Sala) chatDeLaLlamada(ev api.RoomsEvent) {
	if sa.llamada == nil || ev.ChannelID != sa.canalLlamada || ev.Type != "message" {
		return
	}
	var m api.RoomsMessage
	if json.Unmarshal(ev.Payload, &m) == nil && m.ID != "" && m.Type != "call" && m.ParentID == nil {
		sa.llamada.Recibir(sa.mensajeChat(m))
	}
}

// Opcion es un dispositivo que se puede elegir.
type Opcion struct{ ID, Nombre string }

// SelectorDispositivo es la etiqueta y el `<select>` de un dispositivo, con
// «Predeterminado del sistema» primero; lo usan la antesala y «Configuración de
// dispositivos». `tinta` es el token de la etiqueta (la antesala es siempre oscura).
func SelectorDispositivo(t *i18n.Translator, etiqueta string, ops []Opcion, actual, tinta string, alElegir func(id string)) fyne.CanvasObject {
	nombres := make([]string, 0, 1+len(ops))
	nombres = append(nombres, t.T("meet.prejoin.systemDefault", nil))
	elegida := 0
	for i, o := range ops {
		nombres = append(nombres, o.Nombre)
		if o.ID == actual {
			elegida = i + 1
		}
	}
	s := ui.NuevoSelector(nombres, func(i int) {
		if alElegir == nil {
			return
		}
		if i <= 0 || i > len(ops) {
			alElegir("")
			return
		}
		alElegir(ops[i-1].ID)
	})
	s.Elegida, s.Rotulo, s.Talla = elegida, etiqueta, ui.CampoSM
	return container.New(ui.Columna{Hueco: tema.Espacio(1)}, ui.NuevoRotulo(etiqueta, tema.TextoXS, ui.Medio, tinta), s)
}

// abrirDispositivos es «Configuración de dispositivos»: el micro, el altavoz y la
// cámara. Cambiarlos no saca de la sala.
func (sa *Sala) abrirDispositivos(v *VistaLlamada, l EnCurso) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(v)
	if lienzo == nil {
		return
	}
	t := sa.P.T
	micros, altavoces, camaras := sa.Dispositivos()
	micro, altavoz, cam := sa.MicroElegido, sa.AltavozElegido, sa.CamaraElegida
	fm := SelectorDispositivo(t, t.T("meet.prejoin.microphone", nil), micros, micro, "text", func(id string) { micro = id })
	fa := SelectorDispositivo(t, t.T("meet.prejoin.speaker", nil), altavoces, altavoz, "text", func(id string) { altavoz = id })
	fc := SelectorDispositivo(t, t.T("meet.prejoin.camera", nil), camaras, cam, "text", func(id string) { cam = id })
	var d *ui.Dialogo
	listo := ui.NuevoBoton(t.T("meet.closePanel", nil), func() {
		d.Cerrar()
		sa.aplicarDispositivos(v, l, micro, altavoz, cam)
	}).Con(ui.Primario, ui.TallaMD)
	d = ui.NuevoDialogo(t.T("meet.devices", nil), container.New(ui.Columna{Hueco: tema.Espacio(3)}, fm, fa, fc)).Con(ui.DialogoMD, listo)
	d.Mostrar(lienzo)
}

// aplicarDispositivos cambia a los elegidos sin salir de la sala: el audio se reabre y
// la cámara, si está encendida, se apaga y se enciende con la nueva (los demás la ven en
// el siguiente fotograma clave).
func (sa *Sala) aplicarDispositivos(v *VistaLlamada, l EnCurso, micro, altavoz, cam string) {
	cambiaAudio := micro != sa.MicroElegido || altavoz != sa.AltavozElegido
	cambiaCam := cam != sa.CamaraElegida && v.camara
	sa.MicroElegido, sa.AltavozElegido, sa.CamaraElegida = micro, altavoz, cam
	sa.Lanzar(func() {
		if cambiaAudio && sa.AudioDe != nil {
			_ = l.CambiarAudio(sa.AudioDe(micro, altavoz))
		}
		if cambiaCam {
			_ = l.Camara(false)
			_ = l.Camara(true)
		}
	})
}

// ColgarLlamada cierra la llamada en curso, si la hay. Lo llama Parar: salir de la app
// no deja a nadie colgado en el SFU.
func (sa *Sala) ColgarLlamada() {
	if sa.ventanaLlamada != nil {
		sa.ventanaLlamada.Close()
	}
}
