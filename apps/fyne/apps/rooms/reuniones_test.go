// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// conReuniones es el doble del servicio con las salas sueltas.
type conReuniones struct {
	*servicioFalso
	reuniones []api.RoomsMeeting
	// errLlamar es lo que contesta KnockMeeting.
	errLlamar error
}

func (s *conReuniones) KnockMeeting(_ context.Context, id, _ string) error {
	s.apuntar("KnockMeeting", id)
	return s.errLlamar
}

func (s *conReuniones) Meetings(context.Context) ([]api.RoomsMeeting, error) {
	s.apuntar("Meetings")
	return s.reuniones, nil
}

func (s *conReuniones) CreateMeeting(_ context.Context, titulo string) (api.RoomsMeeting, error) {
	s.apuntar("CreateMeeting", titulo)
	return api.RoomsMeeting{ID: "11111111-2222-3333-4444-555555555555", Title: titulo}, nil
}

func (s *conReuniones) CloseMeeting(_ context.Context, id string) error {
	s.apuntar("CloseMeeting", id)
	return nil
}

func (s *conReuniones) MeetingParticipants(_ context.Context, id string) ([]api.RoomsMeetingParticipant, error) {
	s.apuntar("MeetingParticipants", id)
	return []api.RoomsMeetingParticipant{{ID: "p1", Identity: "ana", Name: "Ana"}}, nil
}

func (s *conReuniones) MeetingRecordings(_ context.Context, id string) ([]api.RoomsMeetingRecording, error) {
	s.apuntar("MeetingRecordings", id)
	return []api.RoomsMeetingRecording{{ID: "g1", Status: "completed", SizeBytes: 2048, DurationSec: 90}}, nil
}

func (s *conReuniones) RecordingURL(_ context.Context, id, g string) (string, error) {
	s.apuntar("RecordingURL", id, g)
	return "https://s3.eu/" + g, nil
}

func (s *conReuniones) RemoveMeetingParticipant(_ context.Context, id, quien string) error {
	s.apuntar("RemoveMeetingParticipant", id, quien)
	return nil
}

// `meetingIdFromInput`, `enlaceParaCompartir` y `humanDuration`.
func TestLoQuePideElPanelDeReuniones(t *testing.T) {
	id := "0f8fad5b-d9cb-469f-a165-70867728950e"
	for crudo, quiero := range map[string]string{
		id: id, "  " + id + " ": id, "https://rooms.sytue.eu/?rooms=" + id: id, "https://rooms.sytue.eu/rooms/?meet=" + id: id,
		"https://rooms.sytue.eu/?rooms=hola": "", "hola": "", "": "",
	} {
		if got := ReunionDeEnlace(crudo); got != quiero {
			t.Errorf("ReunionDeEnlace(%q) = %q; quiero %q", crudo, got, quiero)
		}
	}
	if got := EnlaceCompartir("whatsapp", "Únete:", "https://x.eu/?rooms=1", ""); got != "https://wa.me/?text=%C3%9Anete%3A+https%3A%2F%2Fx.eu%2F%3Frooms%3D1" {
		t.Errorf("whatsapp: %s", got)
	}
	if got := EnlaceCompartir("email", "Únete", "https://x.eu", "Guardia semanal"); got != "mailto:?subject=Guardia%20semanal&body=%C3%9Anete%0Ahttps:%2F%2Fx.eu" {
		t.Errorf("correo: %s", got)
	}
	tr := traductor(t)
	for seg, quiero := range map[int64]string{30: "menos de 1 min", 90: "2 min", 3600: "1 h", 3900: "1 h 5 min"} {
		if got := DuracionLegible(tr, seg); got != quiero {
			t.Errorf("DuracionLegible(%d) = %q; quiero %q", seg, got, quiero)
		}
	}
}

// El panel lista las reuniones en marcha; convocar sin invitados crea la sala y la abre
// en el navegador; con invitados abre el directo con ellos; pegar un enlace de Zoom lo
// abre tal cual, uno sin reunión avisa; «Copiar el enlace» copia el de convocar y
// vuelve a su rótulo a los dos segundos; «Detalles» trae quién está y lo grabado.
//
// VISTO EN ROJO (2026-09-24, en el frontal): con AlCrear llamando siempre a
// CreateMeeting, convocar con invitados no abre el directo.
func TestElPanelDeReuniones(t *testing.T) {
	base := servicioConDos()
	s := &conReuniones{servicioFalso: base, reuniones: []api.RoomsMeeting{{ID: "r1", Title: "Guardia", StartedAt: new(time.Now())}}}
	sa, relojes := salaDePrueba(t, base)
	sa.S, sa.Apex = s, "sytue.eu"
	var abiertos []string
	sa.AbrirEnlace = func(u *url.URL) error { abiertos = append(abiertos, u.String()); return nil }
	var copiado string
	sa.Copiar = func(x string) { copiado = x }
	sa.Arrancar(context.Background())

	sa.AbrirPanel(PanelReuniones)
	h := sa.reuniones
	if h == nil || sa.P.Lateral.Panel != PanelReuniones || h.Estado != Listo || len(h.Reuniones) != 1 {
		t.Fatalf("el panel no se abre con su lista: %+v", h)
	}
	test.WidgetRenderer(h)

	_, _, enlace := h.Campos()
	enlace.PonerTexto("hola")
	h.EntrarPorEnlace()
	if !h.ErrorEnlace {
		t.Error("un texto sin reunión no avisa")
	}
	enlace.PonerTexto("https://acme.zoom.us/j/123")
	h.EntrarPorEnlace()
	if len(abiertos) != 1 || abiertos[0] != "https://acme.zoom.us/j/123" {
		t.Errorf("el enlace de Zoom abre %v", abiertos)
	}

	h.AlCopiar("r1")
	if copiado != "https://rooms.sytue.eu/?rooms=r1" || h.Copiado != "r1" {
		t.Errorf("copia %q", copiado)
	}
	relojes[2*time.Second]()
	if h.Copiado != "" {
		t.Error("«Enlace copiado» no vuelve a su rótulo")
	}

	h.AlDetalle("r1")
	if len(h.Participantes) != 1 || len(h.Grabaciones) != 1 || h.CargandoDetalle {
		t.Errorf("detalle: %+v %+v", h.Participantes, h.Grabaciones)
	}
	h.AlDescargar("r1", "g1")
	if abiertos[len(abiertos)-1] != "https://s3.eu/g1" {
		t.Errorf("la descarga abre %v", abiertos)
	}

	h.Invitar(Persona{ID: "beto", Nombre: "Beto"})
	h.Invitar(Persona{ID: "beto", Nombre: "Beto"})
	if len(h.Invitados) != 1 {
		t.Fatalf("invitados: %+v", h.Invitados)
	}
	h.Crear()
	directo := slices.ContainsFunc(s.llamadas, func(l string) bool { return strings.HasPrefix(l, "OpenDirect(") && strings.HasSuffix(l, ",beto)") })
	if !directo || s.pidio("CreateMeeting(Guardia)") || sa.abierto != "dm-beto" || sa.reuniones != nil {
		t.Fatalf("convocar con invitados: %v (abierto %q)", s.llamadas, sa.abierto)
	}

	sa.AbrirPanel(PanelReuniones)
	h = sa.reuniones
	titulo, _, _ := h.Campos()
	titulo.PonerTexto(" Guardia ")
	h.Crear()
	if !s.pidio("CreateMeeting(Guardia)") || abiertos[len(abiertos)-1] != "https://rooms.sytue.eu/?rooms=11111111-2222-3333-4444-555555555555" || sa.panel != "" {
		t.Errorf("convocar sin invitados: %v, abre %v", s.llamadas, abiertos)
	}
}

// Una reunión con la puerta echada no se abre: sale el aviso con «Pedir entrar», que
// llama a la puerta y lo dice; si entretanto la han abierto (409), se entra. La que
// cerró uno mismo se abre sin más, como en el servicio. Y la banda lleva «Reintentar».
//
// VISTO EN ROJO (2026-09-24, en el frontal, saboteado a mano): sin mirar `Locked` en
// entrar, la bloqueada se abre en la web y no sale ningún aviso.
func TestLaPuertaCerradaDeUnaReunion(t *testing.T) {
	base := servicioConDos()
	s := &conReuniones{servicioFalso: base, reuniones: []api.RoomsMeeting{
		{ID: "r1", Title: "Guardia", Locked: true, CreatedBy: "ana"},
		{ID: "r2", Title: "Mía", Locked: true, CreatedBy: "yo"},
	}}
	sa, _ := salaDePrueba(t, base)
	sa.S, sa.Apex = s, "sytue.eu"
	var abiertos []string
	sa.AbrirEnlace = func(u *url.URL) error { abiertos = append(abiertos, u.String()); return nil }
	sa.Arrancar(context.Background())
	sa.adminEquipo = false
	tr := sa.P.T

	sa.AbrirPanel(PanelReuniones)
	sa.reuniones.AlUnirse("r2")
	if len(abiertos) != 1 || !strings.HasSuffix(abiertos[0], "?rooms=r2") {
		t.Fatalf("la mía abre %v", abiertos)
	}
	sa.AbrirPanel(PanelReuniones)
	h := sa.reuniones
	test.WidgetRenderer(h)
	h.AlReintentar = func() { s.apuntar("Reintentar") }
	b, ok := h.cabecera.Extra.(*BotonBanda)
	if !ok || b.Glifo != nil || b.Rotulo != tr.T("action.retry", nil) || h.cabecera.HuecoExtra != tema.Espacio(0.5) {
		t.Fatalf("la banda lleva %#v", h.cabecera.Extra)
	}
	b.AlPulsar()
	if !s.pidio("Reintentar()") {
		t.Error("«Reintentar» de la banda no recarga")
	}

	h.AlUnirse("r1")
	avisos := sa.P.Avisos.Avisos()
	if len(abiertos) != 1 || len(avisos) == 0 {
		t.Fatalf("la bloqueada abre %v", abiertos)
	}
	a := avisos[len(avisos)-1]
	if a.Titulo != tr.T("meet.roomLockedNotice", nil) || a.RotuloAccion != tr.T("meet.knock", nil) {
		t.Fatalf("el aviso es %q / %q", a.Titulo, a.RotuloAccion)
	}
	a.AlPulsarAccion()
	avisos = sa.P.Avisos.Avisos()
	if !s.pidio("KnockMeeting(r1)") || avisos[len(avisos)-1].Titulo != tr.T("meet.knocked", nil) {
		t.Errorf("llamar: %v", s.llamadas)
	}
	s.errLlamar = &api.Error{Status: http.StatusConflict}
	a.AlPulsarAccion()
	if len(abiertos) != 2 || !strings.HasSuffix(abiertos[1], "?rooms=r1") {
		t.Errorf("con la puerta ya abierta se abre %v", abiertos)
	}
}
