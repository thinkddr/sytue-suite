// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"image"
	"net/http"
	"strconv"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// conLlamada es el doble del servicio con las rutas de la llamada.
type conLlamada struct {
	*conReuniones
	// cerradas son las veces que el join contesta «room_locked» antes de dejar pasar.
	cerradas int
}

func (s *conLlamada) join(id string) (api.RoomsCall, error) {
	if s.cerradas > 0 {
		s.cerradas--
		return api.RoomsCall{}, &api.Error{Status: http.StatusConflict, Body: `{"error":{"code":"room_locked"}}`}
	}
	return api.RoomsCall{Room: api.RoomsMeeting{ID: id}, Token: "jwt-" + id, LiveKitURL: "wss://sfu.sytue.eu"}, nil
}

func (s *conLlamada) JoinMeeting(_ context.Context, id, _, _ string) (api.RoomsCall, error) {
	s.apuntar("JoinMeeting", id)
	return s.join(id)
}

func (s *conLlamada) CallChannel(_ context.Context, canal, titulo, _ string) (api.RoomsCall, error) {
	s.apuntar("CallChannel", canal, titulo)
	return s.join("sala-" + canal)
}

func (s *conLlamada) KnockChannel(_ context.Context, canal string) error {
	s.apuntar("KnockChannel", canal)
	return nil
}

func (s *conLlamada) PostCall(_ context.Context, canal string) error {
	s.apuntar("PostCall", canal)
	return nil
}

// enCursoFalsa es la llamada conectada de los guardianes.
type enCursoFalsa struct {
	ps              []llamada.Participante
	micro, cam, pan []bool
	colgada         int
	grabando        bool
	senales         []string
	audios          []llamada.Dispositivo
}

func (e *enCursoFalsa) Camara(a bool) error { e.cam = append(e.cam, a); return nil }
func (e *enCursoFalsa) Grabando() bool      { return e.grabando }
func (e *enCursoFalsa) CambiarAudio(d llamada.Dispositivo) error {
	e.audios = append(e.audios, d)
	return nil
}

func (e *enCursoFalsa) Senal(s llamada.Senal) error {
	e.senales = append(e.senales, string(s.Codificar()))
	if s.Tipo == "mano" && len(e.ps) > 0 {
		e.ps[0].ManoAlzada = s.Alzada
	}
	return nil
}
func (e *enCursoFalsa) Pantalla(a bool) error { e.pan = append(e.pan, a); return nil }

// camaraFalsa es una cámara que da un fotograma gris por lectura.
type camaraFalsa struct{ cerrada bool }

func (c *camaraFalsa) Leer() (*image.YCbCr, bool) { return camara.I420(64, 36), !c.cerrada }
func (c *camaraFalsa) Cerrar()                    { c.cerrada = true }

func (e *enCursoFalsa) Participantes() []llamada.Participante { return e.ps }
func (e *enCursoFalsa) Micro(a bool) error                    { e.micro = append(e.micro, a); return nil }
func (e *enCursoFalsa) Reconectando() bool                    { return false }
func (e *enCursoFalsa) PuedePublicar() bool                   { return true }
func (e *enCursoFalsa) Colgar()                               { e.colgada++ }

// «Llamar» en un canal entra en la llamada NATIVA con el token del servicio —la sala
// del canal, `CallChannel`—, con el micro como se dejó en la antesala; deja la tarjeta
// en el historial; pinta una tesela por persona (y la pantalla compartida delante, a lo
// ancho); y cerrar la ventana cuelga LIMPIO. Con la puerta echada, «Pedir entrar» llama
// y se reintenta solo cada 8 s hasta que abren.
//
// VISTO EN ROJO (2026-09-25, en el frontal): quitando `enCurso.Colgar()` del cierre de
// la ventana, la llamada sigue viva; y sin reprogramar `entrar` tras llamar a la puerta,
// no se vuelve a intentar.
func TestLaLlamadaNativaDeUnCanal(t *testing.T) {
	base := servicioConDos()
	s := &conLlamada{conReuniones: &conReuniones{servicioFalso: base}, cerradas: 2}
	sa, relojes, _ := salaEn(t, base, 1280)
	sa.S = s
	en := &enCursoFalsa{ps: []llamada.Participante{
		{Identidad: "yo", Nombre: "Yo", Yo: true},
		{Identidad: "ana", Nombre: "Ana Ruiz", Hablando: true, Compartida: image.NewRGBA(image.Rect(0, 0, 16, 9))},
		{Identidad: "beto", Nombre: "Beto", MicroCerrado: true},
	}}
	var conectado []string
	sa.Conectar = func(_ context.Context, call api.RoomsCall, o llamada.Opciones) (EnCurso, error) {
		u, token := call.LiveKitURL, call.Token
		conectado = append(conectado, u, token)
		if o.Micro {
			t.Error("se entra con el micro abierto y en la antesala se cerró")
		}
		return en, nil
	}
	sa.Arrancar(context.Background())
	sa.Elegir(sa.P.Lateral.Canales[0])
	tr := sa.P.T

	sa.P.Canal.Cabecera.AlLlamar()
	w := sa.ventanaLlamada
	if w == nil {
		t.Fatal("«Llamar» no abre la ventana de la llamada")
	}
	a, ok := w.Content().(*Antesala)
	if !ok {
		t.Fatalf("la ventana abre con %T y no con la antesala", w.Content())
	}
	a.AlCambiarMicro()
	a.AlEntrar()
	if !s.pidio("CallChannel(c1,equipo)") || a.Aviso != tr.T("meet.roomLockedNotice", nil) || a.AlAccion == nil {
		t.Fatalf("la puerta echada: %v · aviso %q", s.llamadas, a.Aviso)
	}
	a.AlAccion()
	if !s.pidio("KnockChannel(c1)") || a.Aviso != tr.T("meet.knocked", nil) {
		t.Fatalf("llamar a la puerta: %v · aviso %q", s.llamadas, a.Aviso)
	}
	for range 2 {
		reintento := relojes[PausaPuerta]
		if reintento == nil {
			t.Fatal("llamado, no se vuelve a intentar entrar")
		}
		delete(relojes, PausaPuerta)
		reintento()
	}
	if len(conectado) != 2 || conectado[0] != "wss://sfu.sytue.eu" || conectado[1] != "jwt-sala-c1" {
		t.Fatalf("se conecta con %v", conectado)
	}
	v, ok := w.Content().(*VistaLlamada)
	if !ok {
		t.Fatalf("tras entrar la ventana tiene %T", w.Content())
	}
	if !s.pidio("PostCall(c1)") {
		t.Error("la llamada no deja su tarjeta en el historial")
	}
	test.WidgetRenderer(v)
	w.Resize(fyne.NewSize(1280, 720))
	if len(v.teselas) != 4 || !v.teselas[0].Compartida || v.teselas[0].Identidad != "ana" {
		t.Fatalf("teselas: %d, la primera %+v", len(v.teselas), v.teselas[0].Participante)
	}
	if g, p := v.teselas[0].Size(), v.teselas[1].Size(); g.Width <= p.Width || g.Height <= p.Height {
		t.Errorf("la compartida mide %v y una persona %v: tendría que ir en grande", g, p)
	}
	if !v.teselas[1].MicroCerrado || !v.teselas[3].MicroCerrado || v.teselas[2].MicroCerrado {
		t.Errorf("silenciados: yo %v, ana %v, beto %v", v.teselas[1].MicroCerrado, v.teselas[2].MicroCerrado, v.teselas[3].MicroCerrado)
	}
	v.AlternarMicro()
	if len(en.micro) != 1 || !en.micro[0] || !v.Micro() {
		t.Errorf("el micro de la barra pide %v", en.micro)
	}

	w.Close()
	if en.colgada != 1 || sa.ventanaLlamada != nil {
		t.Errorf("cerrar la ventana: colgada %d veces, ventana %v", en.colgada, sa.ventanaLlamada)
	}
}

// Una reunión del panel entra en nativo y, sin llamada nativa, sigue abriéndose en la
// web como antes.
func TestLaReunionEntraEnNativo(t *testing.T) {
	base := servicioConDos()
	s := &conLlamada{conReuniones: &conReuniones{servicioFalso: base, reuniones: []api.RoomsMeeting{{ID: "r1", Title: "Guardia"}}}}
	sa, _ := salaDePrueba(t, base)
	sa.S = s
	if sa.EntrarEnReunion("r1", "Guardia") {
		t.Fatal("sin Conectar entra en nativo")
	}
	en := &enCursoFalsa{}
	sa.Conectar = func(context.Context, api.RoomsCall, llamada.Opciones) (EnCurso, error) { return en, nil }
	sa.Arrancar(context.Background())
	sa.AbrirPanel(PanelReuniones)
	sa.reuniones.AlUnirse("r1")
	if sa.ventanaLlamada == nil {
		t.Fatal("«Unirse» no abre la llamada nativa")
	}
	sa.ventanaLlamada.Content().(*Antesala).AlEntrar()
	if !s.pidio("JoinMeeting(r1)") {
		t.Errorf("entrar pide %v", s.llamadas)
	}
	sa.Parar()
	if en.colgada != 1 {
		t.Error("parar la sala no cuelga la llamada")
	}
}

// El enlace de una reunión que llega con la app recién abierta (el App Link, el
// Universal Link o la página de la web) espera a que la sala arranque y entonces abre su
// llamada; con la sala ya en marcha, en el acto.
//
// VISTO EN ROJO (2026-09-25, en el frontal): sin mirar `reunionPendiente` al final de
// Arrancar, el enlace de un arranque en frío se pierde.
func TestElEnlaceDeUnaReunionAbreLaLlamada(t *testing.T) {
	base := servicioConDos()
	s := &conLlamada{conReuniones: &conReuniones{servicioFalso: base}}
	sa, _ := salaDePrueba(t, base)
	sa.S = s
	sa.Conectar = func(context.Context, api.RoomsCall, llamada.Opciones) (EnCurso, error) { return &enCursoFalsa{}, nil }
	sa.EntrarPorEnlace("r9", "")
	if sa.ventanaLlamada != nil {
		t.Fatal("con la sala sin arrancar ya abre la llamada")
	}
	sa.Arrancar(context.Background())
	if sa.ventanaLlamada == nil {
		t.Fatal("arrancada la sala, el enlace pendiente no abre la llamada")
	}
	sa.ventanaLlamada.Content().(*Antesala).AlEntrar()
	if !s.pidio("JoinMeeting(r9)") {
		t.Errorf("entrar pide %v", s.llamadas)
	}
}

// La cámara: en la antesala se enciende a petición (y entonces se ve la previa, que se
// suelta al entrar), se entra con ella como se eligió, y en la llamada su botón la
// apaga y la enciende. Compartir la pantalla sale solo si hay cómo.
//
// VISTO EN ROJO (2026-09-26, en el frontal): sin `soltarPrevia()` al entrar, la cámara de
// la previa se queda abierta con la de la llamada.
func TestLaCamaraDeLaLlamada(t *testing.T) {
	base := servicioConDos()
	s := &conLlamada{conReuniones: &conReuniones{servicioFalso: base}}
	sa, relojes, _ := salaEn(t, base, 1280)
	sa.S = s
	en := &enCursoFalsa{ps: []llamada.Participante{{Identidad: "yo", Yo: true}}}
	sa.Conectar = func(_ context.Context, _ api.RoomsCall, o llamada.Opciones) (EnCurso, error) {
		if o.Camara == nil || o.Pantalla == nil {
			t.Error("la llamada no recibe con qué abrir la cámara y la pantalla")
		}
		return en, nil
	}
	cam := &camaraFalsa{}
	sa.Camara = func() (camara.Fuente, error) { return cam, nil }
	sa.Pantalla = func() (camara.Fuente, error) { return &camaraFalsa{}, nil }
	sa.Arrancar(context.Background())
	if !sa.EntrarEnReunion("r1", "Guardia") {
		t.Fatal("no entra")
	}
	a := sa.ventanaLlamada.Content().(*Antesala)
	if a.Camara || a.SinCamara {
		t.Fatalf("la antesala empieza con la cámara %v (sin cámara %v)", a.Camara, a.SinCamara)
	}
	a.AlCambiarCamara()
	if !a.Camara || a.Previa() == nil || relojes[time.Second/15] == nil {
		t.Fatalf("encendida en la antesala: cámara %v, previa %v", a.Camara, a.Previa())
	}
	a.AlEntrar()
	if !cam.cerrada {
		t.Error("al entrar, la cámara de la previa sigue abierta")
	}
	v := sa.ventanaLlamada.Content().(*VistaLlamada)
	if len(en.cam) != 1 || !en.cam[0] || !v.Camara() {
		t.Fatalf("se entra con la cámara pedida y la llamada recibe %v", en.cam)
	}
	test.WidgetRenderer(v)
	// Micro, cámara, compartir, participantes, reaccionar, «…» y salir (sin chat: es
	// una reunión suelta).
	if n := len(v.barra.Objects); n != 7 {
		t.Errorf("la barra lleva %d botones y son siete", n)
	}
	v.AlternarCamara()
	v.AlternarPantalla()
	if len(en.cam) != 2 || en.cam[1] || v.Camara() || len(en.pan) != 1 || !en.pan[0] || !v.Pantalla() {
		t.Errorf("cámara %v, pantalla %v", en.cam, en.pan)
	}
	sa.Pantalla = nil
	v.ConPantalla = false
	v.Refresh()
	if n := len(v.barra.Objects); n != 6 {
		t.Errorf("sin pantalla que compartir la barra lleva %d botones", n)
	}
}

// conModeracion es el doble con las rutas de moderar.
type conModeracion struct{ *conLlamada }

func (s *conModeracion) MuteMeetingParticipant(_ context.Context, id, q string) error {
	s.apuntar("Mute", id, q)
	return nil
}

func (s *conModeracion) SetMeetingSpeech(_ context.Context, id, q string, puede bool) error {
	s.apuntar("Speech", id, q, strconv.FormatBool(puede))
	return nil
}

func (s *conModeracion) MuteAllMeeting(_ context.Context, id string) (int, error) {
	s.apuntar("MuteAll", id)
	return 2, nil
}

func (s *conModeracion) LockMeeting(_ context.Context, id string, cerrar bool) error {
	s.apuntar("Lock", id, strconv.FormatBool(cerrar))
	return nil
}

func (s *conModeracion) MeetingKnocks(_ context.Context, id string) ([]api.RoomsKnock, error) {
	s.apuntar("Knocks", id)
	return []api.RoomsKnock{{Identity: "u9", Name: "Eva"}}, nil
}

func (s *conModeracion) AnswerMeetingKnock(_ context.Context, id, q string, admitir bool) error {
	s.apuntar("Answer", id, q, strconv.FormatBool(admitir))
	return nil
}

func (s *conModeracion) RecordMeeting(_ context.Context, id string, grabar bool) error {
	s.apuntar("Record", id, strconv.FormatBool(grabar))
	return nil
}

func TestLaLlamadaCifradaNoOfreceGrabacionDelServidor(t *testing.T) {
	base := servicioConDos()
	sa, _, _ := salaEn(t, base, 1280)
	s := &conModeracion{conLlamada: &conLlamada{conReuniones: &conReuniones{servicioFalso: base}}}
	sa.S = s
	en := &enCursoFalsa{}
	v := NuevaVistaLlamada(sa.P.T, "Equipo", en, false)
	v.Lanzar = func(fn func()) { fn() }
	call := api.RoomsCall{Room: api.RoomsMeeting{ID: "call"}, Role: "host", Encryption: &api.RoomsCallEncryption{CallID: "call"}}
	sa.prepararBarra(v, call, "", en)
	for _, item := range v.elementosMas() {
		if element, ok := item.(*ui.ElementoMenu); ok && element.Texto == sa.P.T.T("meet.startRecording", nil) {
			t.Fatal("encrypted media offered server recording")
		}
	}
	v.AlternarGrabacion()
	if s.pidio("Record(call,true)") {
		t.Fatal("encrypted media requested server recording")
	}
}

// El resto de la barra: el chat es el del canal (carga lo último, lo que se escribe va
// al canal y lo que llega en vivo entra), el anfitrión modera (silenciar, quitar la
// palabra, cerrar la puerta y ver quién espera, abrirle, grabar), las reacciones flotan
// y caducan, la mano se levanta, y «Configuración de dispositivos» cambia la tarjeta
// sin salir de la sala.
//
// VISTO EN ROJO (2026-09-26, en el frontal): sin `sa.chatDeLaLlamada(ev)` en alLlegar,
// lo que escriben los demás no entra en el chat; y con `puede` sin negar al quitar la
// palabra, el botón la devolvía.
func TestLaBarraDeLaLlamada(t *testing.T) {
	base := servicioConDos()
	s := &conModeracion{&conLlamada{conReuniones: &conReuniones{servicioFalso: base}}}
	sa, relojes, _ := salaEn(t, base, 1280)
	sa.S = s
	en := &enCursoFalsa{ps: []llamada.Participante{{Identidad: "yo", Yo: true}, {Identidad: "ana", Nombre: "Ana"}}}
	sa.Conectar = func(context.Context, api.RoomsCall, llamada.Opciones) (EnCurso, error) { return en, nil }
	sa.Dispositivos = func() (m, a, c []Opcion) {
		return []Opcion{{"m1", "Micro USB"}}, []Opcion{{"a1", "Cascos"}}, []Opcion{{"c1", "Webcam"}}
	}
	var abiertos []string
	sa.AudioDe = func(m, a string) llamada.Dispositivo {
		abiertos = append(abiertos, m+"/"+a)
		return nil
	}
	sa.Arrancar(context.Background())
	sa.Elegir(sa.P.Lateral.Canales[0])
	sa.P.Canal.Cabecera.AlLlamar()
	sa.ventanaLlamada.Content().(*Antesala).AlEntrar()
	v := sa.ventanaLlamada.Content().(*VistaLlamada)
	test.WidgetRenderer(v)
	if !v.ConChat || !s.pidio("Messages(c1)") {
		t.Fatalf("la llamada del canal no trae su chat: %v", s.llamadas)
	}
	// Lo que llega en vivo al canal entra en el chat de la llamada.
	sa.alLlegar(api.RoomsEvent{
		Type: "message", ChannelID: "c1",
		Payload: []byte(`{"id":"m9","channel_id":"c1","author_sub":"ana","author_name":"Ana","body":"llego"}`),
	})
	if n := len(v.Chat); n == 0 || v.Chat[n-1].Texto != "llego" {
		t.Fatalf("el mensaje en vivo no entra: %+v", v.Chat)
	}
	v.AbrirPanel(PanelChat)
	v.entradaChat.PonerTexto("voy")
	v.EnviarChat()
	if !s.pidio("Post(c1,voy)") {
		t.Errorf("escribir en la llamada no va al canal: %v", s.llamadas)
	}

	// Quien convocó modera (el join le dio el rol host en el doble: CreatedBy == yo).
	v.Modera = true
	v.AbrirPanel(PanelParticipantes)
	_ = v.Mod.Silenciar("ana")
	v.AlternarPalabra("ana")
	v.AlternarPalabra("ana")
	v.AlternarBloqueo()
	if !v.Bloqueada || !s.pidio("Lock(sala-c1,true)") || !s.pidio("Knocks(sala-c1)") || len(v.Cola) != 1 {
		t.Fatalf("cerrar la puerta: bloqueada %v, cola %v, %v", v.Bloqueada, v.Cola, s.llamadas)
	}
	v.responder("u9", true)()
	v.AlternarGrabacion()
	for _, quiero := range []string{"Mute(sala-c1,ana)", "Speech(sala-c1,ana,false)", "Speech(sala-c1,ana,true)", "Answer(sala-c1,u9,true)", "Record(sala-c1,true)"} {
		if !s.pidio(quiero) {
			t.Errorf("no se pidió %s: %v", quiero, s.llamadas)
		}
	}
	if len(v.Cola) != 0 {
		t.Error("abierta la puerta, sigue en la cola")
	}

	// Reacciones y mano.
	v.Reaccionar("🎉")
	v.Senal("ana", llamada.Reaccion("👏"))
	if len(v.reacciones) != 1 || v.flotObj == nil {
		t.Fatalf("la reacción no flota: %+v", v.reacciones)
	}
	v.caducar(time.Now().Add(DuracionReaccion + time.Second))
	if len(v.reacciones) != 0 {
		t.Error("la reacción no caduca")
	}
	v.AlternarMano()
	if len(en.senales) != 2 || en.senales[1] != `{"t":"mano","alzada":true}` || !v.manoPropia() {
		t.Errorf("señales %v", en.senales)
	}

	// Dispositivos: el diálogo sale, y elegir otro micro reabre el audio y otra cámara,
	// con la cámara encendida, la vuelve a abrir.
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(v)
	antes := len(lienzo.Overlays().List())
	sa.abrirDispositivos(v, en)
	if len(lienzo.Overlays().List()) != antes+1 {
		t.Error("«Configuración de dispositivos» no abre su diálogo")
	}
	v.camara = true
	sa.aplicarDispositivos(v, en, "m1", "", "c1")
	// La primera tarjeta abierta es la del medidor de la antesala (`/`, la del sistema).
	if len(abiertos) != 2 || abiertos[1] != "m1/" || len(en.audios) != 1 || sa.CamaraElegida != "c1" ||
		len(en.cam) < 2 || en.cam[len(en.cam)-2] || !en.cam[len(en.cam)-1] {
		t.Errorf("dispositivos: tarjeta %v, audios %d, cámara %v", abiertos, len(en.audios), en.cam)
	}
	_ = relojes
}

// El escenario: una pantalla compartida va en grande; fijar a alguien le gana; en vista
// de orador va en grande el último que habló y se queda al callarse; soltar vuelve al
// mosaico. Y «Fijar» sale al pasar por encima y siempre en la fijada.
//
// VISTO EN ROJO (2026-09-27, en el frontal): con RefrescarVideo por posición, la
// imagen de Ana acababa en la tesela de su pantalla compartida.
func TestElEscenarioDeLaLlamada(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 9))
	en := &enCursoFalsa{ps: []llamada.Participante{
		{Identidad: "yo", Nombre: "Yo", Yo: true},
		{Identidad: "ana", Nombre: "Ana"},
		{Identidad: "beto", Nombre: "Beto", Compartida: img},
	}}
	v := NuevaVistaLlamada(traductor(t), "Guardia", en, true)
	w := test.NewTempWindow(t, v)
	w.Resize(fyne.NewSize(1280, 720))
	if got := v.Protagonista(); got != idPantalla("beto") {
		t.Fatalf("con Beto compartiendo, en grande va %q", got)
	}
	v.AlternarFijado("ana")
	if v.Protagonista() != "ana" || v.teselas[0].Identidad != "ana" || !v.teselas[0].Fijada {
		t.Fatalf("fijada Ana, en grande va %q", v.Protagonista())
	}
	if g, p := v.teselas[0].Size(), v.teselas[1].Size(); g.Width <= p.Width {
		t.Errorf("la fijada mide %v y otra %v", g, p)
	}
	if !v.teselas[0].VeFijar() || v.teselas[1].VeFijar() {
		t.Error("«Soltar» no se ve en la fijada, o «Fijar» se ve sin pasar por encima")
	}
	v.teselas[1].MouseIn(nil)
	if !v.teselas[1].VeFijar() {
		t.Error("al pasar por encima no sale «Fijar»")
	}
	v.AlternarFijado("ana") // soltar
	en.ps[2].Compartida = nil
	v.Refresh()
	if v.Protagonista() != "" {
		t.Fatalf("sin nada que destacar, en grande va %q", v.Protagonista())
	}
	v.AlternarModo()
	en.ps[1].Hablando = true
	v.Refresh()
	en.ps[1].Hablando = false
	v.Refresh()
	if v.Protagonista() != "ana" {
		t.Errorf("en vista de orador, callada Ana, en grande va %q", v.Protagonista())
	}
	// Los fotogramas van por identidad.
	en.ps[2].Compartida = img
	v.Refresh()
	en.ps[1].Video = image.NewRGBA(image.Rect(0, 0, 32, 18))
	v.RefrescarVideo()
	for _, ts := range v.teselas {
		if ts.Identidad == "ana" && ts.Video == nil {
			t.Error("la imagen de Ana no llega a su tesela")
		}
		if ts.Compartida && ts.Identidad == "beto" && ts.imagen == nil {
			t.Error("la pantalla de Beto perdió su imagen")
		}
	}
}

// tarjetaFalsa es una tarjeta que, al abrirse, capta un tramo al volumen pedido.
type tarjetaFalsa struct {
	volumen  int16
	abiertas *[]string
	id       string
	cerrada  *bool
}

func (k tarjetaFalsa) Abrir(capturado, _ func([]int16)) (func(), error) {
	*k.abiertas = append(*k.abiertas, k.id)
	tramo := make([]int16, llamada.Tramo)
	for i := range tramo {
		tramo[i] = k.volumen
	}
	capturado(tramo)
	return func() { *k.cerrada = true }, nil
}

// La antesala de la web: el medidor sube con lo que capta el micro (y se suelta al
// apagarlo), los selectores salen con «El del sistema» primero, y elegir otro micro
// reabre el medidor con él.
//
// VISTO EN ROJO (2026-09-27, en el frontal): sin `p.medir()` al elegir micro, el
// medidor seguía con el micro de antes.
func TestLaAntesalaMideYEligeDispositivo(t *testing.T) {
	base := servicioConDos()
	s := &conLlamada{conReuniones: &conReuniones{servicioFalso: base}}
	sa, relojes, _ := salaEn(t, base, 1280)
	sa.S = s
	sa.Conectar = func(context.Context, api.RoomsCall, llamada.Opciones) (EnCurso, error) { return &enCursoFalsa{}, nil }
	sa.Dispositivos = func() (m, a, c []Opcion) {
		return []Opcion{{"m1", "Micro USB"}}, []Opcion{{"a1", "Cascos"}}, []Opcion{{"c1", "Webcam"}}
	}
	var abiertas []string
	cerrada := false
	sa.AudioDe = func(m, _ string) llamada.Dispositivo {
		return tarjetaFalsa{volumen: 12000, abiertas: &abiertas, id: m, cerrada: &cerrada}
	}
	sa.Arrancar(context.Background())
	sa.EntrarEnReunion("r1", "Guardia")
	a := sa.ventanaLlamada.Content().(*Antesala)
	test.WidgetRenderer(a)
	if len(a.Micros) != 1 || len(a.Altavoces) != 1 || len(abiertas) != 1 || abiertas[0] != "" {
		t.Fatalf("antesala: micros %v, altavoces %v, abiertas %v", a.Micros, a.Altavoces, abiertas)
	}
	pintar := relojes[ritmoAntesala]
	if pintar == nil {
		t.Fatal("el medidor no se pinta")
	}
	pintar()
	if a.Nivel < 0.7 {
		t.Errorf("con el micro a 12000 el medidor marca %.2f", a.Nivel)
	}
	a.AlElegirMicro("m1")
	if len(abiertas) != 2 || abiertas[1] != "m1" || !cerrada || sa.MicroElegido != "m1" {
		t.Errorf("elegir micro: abiertas %v, cerrada %v", abiertas, cerrada)
	}
	a.AlCambiarMicro()
	if a.Micro || a.Nivel != 0 {
		t.Errorf("apagado el micro, el medidor marca %.2f", a.Nivel)
	}
}
