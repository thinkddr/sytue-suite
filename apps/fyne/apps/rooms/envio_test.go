// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// ---- El doble: lo que se añadió al Servicio con las opciones de envío ----

func (s *servicioFalso) PostWith(ctx context.Context, c, texto string, o api.RoomsSend) (api.RoomsMessage, error) {
	s.envios = append(s.envios, o)
	return s.Post(ctx, c, texto) // apunta «Post(c,texto)», como antes
}

func (s *servicioFalso) ReplyWith(ctx context.Context, raiz, texto string, o api.RoomsSend) (api.RoomsMessage, error) {
	s.envios = append(s.envios, o)
	return s.Reply(ctx, raiz, texto)
}

func (s *servicioFalso) Schedule(_ context.Context, c, texto string, _ []string, cuando time.Time) (api.RoomsScheduled, error) {
	s.apuntar("Schedule", c, texto, cuando.UTC().Format(time.RFC3339))
	return api.RoomsScheduled{ID: "p9", ChannelID: c, Body: texto, SendAt: cuando}, s.errAccion
}

func (s *servicioFalso) Reschedule(_ context.Context, id string, cuando time.Time) (api.RoomsScheduled, error) {
	s.apuntar("Reschedule", id, cuando.UTC().Format(time.RFC3339))
	return api.RoomsScheduled{ID: id, SendAt: cuando}, s.errAccion
}

func (s *servicioFalso) Ack(_ context.Context, m string) (int, error) {
	s.apuntar("Ack", m)
	return 7, s.errAccion
}

func (s *servicioFalso) Readers(_ context.Context, m string) ([]api.RoomsReader, int, error) {
	s.apuntar("Readers", m)
	return s.lectores, 12, s.errAccion
}

// ---- Los guardianes ----

// Las @menciones que viajan son las del roster cuyo nombre sale literal tras la «@», sin
// repetir; la prioridad y el acuse salen de la caja, y la caja vuelve a un envío normal.
//
// VISTO EN ROJO (2026-09-23, en el frontal): quitando `c.QuitarOpciones()` de `Vaciar`,
// el segundo envío sale también urgente y con acuse.
func TestLasOpcionesViajanConElEnvio(t *testing.T) {
	s := servicioConDos()
	s.roster = []api.RoomsMember{{Subject: "ana", Name: "Ana Ruiz"}, {Subject: "beto", Name: "Beto"}, {Subject: "yo", Name: "Yo"}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor

	c.PonerPrioridad(Urgente)
	c.PonerAcuse(true)
	sa.Enviar("hola @Ana Ruiz y @Ana Ruiz, ¿y @Carlos?")
	sa.Enviar("otra")
	if len(s.envios) != 2 {
		t.Fatalf("salieron %d envíos: %v", len(s.envios), s.llamadas)
	}
	if o := s.envios[0]; !slices.Equal(o.Mentions, []string{"ana"}) || o.Priority != "urgent" || !o.RequiresAck {
		t.Errorf("el primer envío lleva %+v", o)
	}
	if o := s.envios[1]; len(o.Mentions) != 0 || o.Priority != "" || o.RequiresAck {
		t.Errorf("el segundo arrastra las opciones del primero: %+v", o)
	}
}

// Con hora puesta, Intro PROGRAMA: no publica, avisa y deja la caja limpia. Y la franja
// crece lo que mide la fila de fichas mientras la hora está puesta.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin la rama de `ProgramarA` en Enviar, sale
// «Post(c1,luego)» y no «Schedule».
func TestConHoraElEnvioSeProgramaYNoSePublica(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	sin := c.Medida().Height

	cuando := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	c.PonerProgramado(cuando)
	if got := c.Medida().Height - sin; got != LadoQuitarFicha+AireFichas {
		t.Errorf("con la ficha de la hora la franja crece %v y la fila mide %v", got, LadoQuitarFicha+AireFichas)
	}
	if c.enviar.Rotulo != sa.P.T.T("composer.schedule", nil) {
		t.Errorf("el botón dice %q con hora puesta", c.enviar.Rotulo)
	}
	c.PonerTexto("luego")
	c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !s.pidio("Schedule(c1,luego,2026-09-24T09:00:00Z)") || s.pidio("Post(c1,luego)") {
		t.Fatalf("con hora puesta se pidió %v", s.llamadas)
	}
	if c.Texto() != "" || !c.ProgramarA.IsZero() || c.Medida().Height != sin {
		t.Errorf("tras programar la caja queda con %q, hora %v y alto %v", c.Texto(), c.ProgramarA, c.Medida().Height)
	}

	s.errAccion = errors.New("caído")
	c.PonerProgramado(cuando)
	c.PonerTexto("sin red")
	c.area.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if c.Texto() != "sin red" || !c.ProgramarA.Equal(cuando) {
		t.Errorf("con el servicio caído se pierde lo escrito o la hora: %q, %v", c.Texto(), c.ProgramarA)
	}
}

// Los atajos son mañana y el PRÓXIMO lunes a las 9:00: un lunes, el de la semana que
// viene, y nunca hoy.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `n := 0` en el bucle del lunes, un
// lunes da ese mismo lunes.
func TestLosAtajosDeProgramarSonMananaYElProximoLunes(t *testing.T) {
	for _, c := range []struct {
		hoy           time.Time
		manana, lunes time.Time
	}{
		{time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC), time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC), time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)},
		{time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC), time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)},
		{time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC), time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC), time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)},
	} {
		m, l := PresetsProgramar(c.hoy)
		if !m.Equal(c.manana) || !l.Equal(c.lunes) {
			t.Errorf("desde %v: mañana %v y lunes %v; quiero %v y %v", c.hoy, m, l, c.manana, c.lunes)
		}
	}
}

// El «!» abre su panel con la prioridad vigente marcada, y elegir en él pone la ficha
// sobre la caja; su aspa la quita.
func TestElPanelDePrioridadPoneYQuitaLaFicha(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Canal.Compositor
	prio := c.botones[len(c.botones)-2].(*ui.BotonIcono)
	test.Tap(prio)
	if c.emergente == nil || !c.emergente.Abierto() {
		t.Fatal("el «!» no abre su panel")
	}
	radios := eleccionesEn(c.emergente)
	if len(radios) != 4 || !radios[0].Marcada {
		t.Fatalf("el panel trae %d controles y el primero (normal) marcado=%v", len(radios), len(radios) > 0 && radios[0].Marcada)
	}
	test.Tap(radios[2]) // urgente
	test.Tap(radios[3]) // pedir acuse
	if c.Prioridad != Urgente || !c.Acuse || len(c.objFichas) != 2 {
		t.Fatalf("elegir en el panel deja prioridad %v, acuse %v y %d fichas", c.Prioridad, c.Acuse, len(c.objFichas))
	}
	f := c.objFichas[0].(*FichaOpcion)
	if f.Rotulo != strings.ToUpper(sa.P.T.T("priority.urgent", nil)) || f.Fondo != "danger-bg" {
		t.Errorf("la ficha de la prioridad es %+v", f)
	}
	test.Tap(f.quitar)
	if c.Prioridad != Normal || len(c.objFichas) != 1 {
		t.Errorf("el aspa deja prioridad %v con %d fichas", c.Prioridad, len(c.objFichas))
	}
	c.emergente.Cerrar()
}

// eleccionesEn son las casillas y radios del panel, en orden.
func eleccionesEn(o fyne.CanvasObject) []*ui.Eleccion {
	var out []*ui.Eleccion
	var bajar func(fyne.CanvasObject)
	bajar = func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *ui.Eleccion:
			out = append(out, v)
		case *fyne.Container:
			for _, h := range v.Objects {
				bajar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(v).Objects() {
				bajar(h)
			}
		}
	}
	bajar(o)
	return out
}

// «Reprogramar» mueve la fila a su hora nueva y reordena; una hora pasada no se manda.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el `SortStableFunc`, el programado que
// pasa a ser el primero se queda el segundo.
func TestReprogramarMueveLaFilaYNoAceptaElPasado(t *testing.T) {
	s := servicioConDos()
	s.programados = []api.RoomsScheduled{
		{ID: "p1", ChannelID: "c1", Body: "uno", SendAt: time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)},
		{ID: "p2", ChannelID: "c1", Body: "dos", SendAt: time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)},
	}
	sa, _ := salaDePrueba(t, s)
	sa.Ahora = func() time.Time { return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) }
	sa.Arrancar(context.Background())
	sa.AbrirPanel(PanelProgramado)
	l := sa.lista
	if len(l.Programados) != 2 {
		t.Fatalf("el panel trae %d programados", len(l.Programados))
	}
	nueva := time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)
	sa.Reprogramar(l.Programados[1], nueva)
	if !s.pidio("Reschedule(p2,2026-09-23T15:00:00Z)") || l.Programados[0].ID != "p2" || !l.Programados[0].Cuando.Equal(nueva) {
		t.Errorf("reprogramar dejó %+v y pidió %v", l.Programados, s.llamadas)
	}
	sa.Reprogramar(l.Programados[0], time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC))
	if s.veces("Reschedule(p2,2026-09-22T09:00:00Z)") != 0 {
		t.Error("se reprogramó al pasado")
	}
}

// «Confirmar» marca el acuse ya, sube la cuenta y la ajusta al total del servicio; si el
// servicio dice que no, se deshace.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el `aplicar` del error, el mensaje se
// queda «Confirmado» con el servicio caído.
func TestConfirmarElAcuse(t *testing.T) {
	s := servicioConDos()
	s.mensajes["c1"][0].RequiresAck, s.mensajes["c1"][0].AckCount = true, 2
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	f := filaDe(t, h, "m1")
	test.WidgetRenderer(f)
	if len(f.acuse) != 2 {
		t.Fatalf("la fila del acuse trae %d piezas y son «Confirmar» y «2 acuses»", len(f.acuse))
	}
	test.Tap(f.acuse[0].(*ui.Boton))
	if m := h.Mensajes[0]; !m.Acusado || m.Acuses != 7 || !s.pidio("Ack(m1)") {
		t.Errorf("confirmar deja %+v", m)
	}
	s.mensajes["c1"][0].Acked, s.mensajes["c1"][0].AckCount = false, 2
	sa.mensajes[0].Acked, sa.mensajes[0].AckCount = false, 2
	sa.pintarMensajes()
	s.errAccion = errors.New("caído")
	sa.Acusar(h.Mensajes[0])
	if m := h.Mensajes[0]; m.Acusado || m.Acuses != 2 {
		t.Errorf("con el servicio caído el acuse queda %v con %d", m.Acusado, m.Acuses)
	}
}

// La fila de un programado lleva «Reprogramar» delante de los botones y parte línea
// cuando no caben, que es lo que hace `flex-wrap`: a lo ancho es una línea y en el
// panel de un teléfono, dos.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `colocarEnvuelto` sin partir línea,
// los tres salen en una fila de 28 también a 240 y «Cancelar» se sale del panel.
func TestLaFilaDeUnProgramadoParteLosControles(t *testing.T) {
	m := Programado{ID: "p1", Canal: "c1", Autor: "Ana", Cuerpo: "hola", Cuando: time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)}
	f := NuevaFilaProgramado(traductor(t), "es", m, "equipo", nil, nil)
	var reprogramado time.Time
	f.AlReprogramar = func(_ Programado, t time.Time) { reprogramado = t }
	test.WidgetRenderer(f)
	if f.reprogramar == nil || f.reprogramar.Fecha.Texto() != "24/09/2026" {
		t.Fatal("la fila no lleva «Reprogramar» con la hora que tiene")
	}
	una := f.altoBotones(1000)
	if dos := f.altoBotones(240); dos <= una {
		t.Errorf("a 240 los controles miden %v, igual que a lo ancho (%v): no parten línea", dos, una)
	}
	f.reprogramar.Hora.PonerTexto("10:15")
	f.reprogramar.Hora.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if reprogramado.Hour() != 10 || reprogramado.Minute() != 15 {
		t.Errorf("Intro reprograma a %v", reprogramado)
	}
}

func (s *servicioFalso) UpdateChannel(_ context.Context, c string, cambio api.RoomsChannelPatch) (api.RoomsChannel, error) {
	v := ""
	if cambio.Topic != nil {
		v = "tema=" + *cambio.Topic
	}
	if cambio.Purpose != nil {
		v = "proposito=" + *cambio.Purpose
	}
	s.apuntar("UpdateChannel", c, v)
	return api.RoomsChannel{ID: c}, s.errAccion
}

func (s *servicioFalso) LeaveChannel(_ context.Context, c string) error {
	s.apuntar("LeaveChannel", c)
	return s.errAccion
}

func (s *servicioFalso) AddTeamMember(_ context.Context, e, p, n string) error {
	s.apuntar("AddTeamMember", e, p, n)
	return s.errAccion
}

func (s *servicioFalso) RemoveTeamMember(_ context.Context, e, p string) error {
	s.apuntar("RemoveTeamMember", e, p)
	return s.errAccion
}

func (s *servicioFalso) Commands(context.Context) ([]api.RoomsCommand, error) {
	s.apuntar("Commands")
	return s.comandos, nil
}

func (s *servicioFalso) ExecuteCommand(_ context.Context, c, linea string) (api.RoomsCommandResult, error) {
	s.apuntar("ExecuteCommand", c, linea)
	return api.RoomsCommandResult{Text: "hecho", ResponseType: "ephemeral"}, s.errAccion
}

func (s *servicioFalso) NotifyPrefs(context.Context) (api.RoomsNotifyPrefs, error) {
	s.apuntar("NotifyPrefs")
	return s.prefs, nil
}

func (s *servicioFalso) PutNotifyPrefs(_ context.Context, p api.RoomsNotifyPrefs) (api.RoomsNotifyPrefs, error) {
	s.apuntar("PutNotifyPrefs", strings.Join(p.Keywords, "+"), HoraDeMinutos(p.DNDStart), HoraDeMinutos(p.DNDEnd))
	return p, s.errAccion
}

func (s *servicioFalso) VoteCard(_ context.Context, m, v string, poner bool) ([]api.RoomsCardVote, error) {
	s.apuntar("VoteCard", m, v, map[bool]string{true: "sí", false: "no"}[poner])
	return s.votos, s.errAccion
}

// La ficha de «Se enviará» lleva el día de la semana corto (`formatSchedule`) y la hora
// en dos cifras; las filas de los paneles, sin el día de la semana.
func TestLaFechaDeUnProgramado(t *testing.T) {
	jueves := time.Date(2026, 9, 24, 9, 5, 0, 0, time.Local)
	if got := RotuloProgramado("es", jueves); !strings.HasPrefix(got, "jue") || !strings.HasSuffix(got, "09:05") {
		t.Errorf("la ficha dice %q", got)
	}
	if got := RotuloCuando("es", jueves); strings.HasPrefix(got, "jue") || !strings.HasSuffix(got, "09:05") {
		t.Errorf("la fila dice %q", got)
	}
}
