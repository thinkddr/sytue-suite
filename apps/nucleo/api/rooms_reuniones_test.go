// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Las reuniones sueltas hablan con las rutas del servicio de vídeo, con sus nombres.
//
// VISTO EN ROJO (2026-09-24, en el frontal): con `json:"started"` en RoomsMeeting, la
// hora de inicio llega a cero y el primer caso falla.
func TestLasReunionesSueltas(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := io.ReadAll(r.Body)
		pedidas = append(pedidas, r.Method+" "+r.URL.RequestURI()+" "+string(c))
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/rooms":
			_, _ = io.WriteString(w, `{"rooms":[{"id":"r1","name":"sala-1","title":"","status":"active","started_at":"2026-09-24T08:00:00Z","locked":true}]}`)
		case "POST /api/v1/rooms":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"r2","name":"sala-2","title":"Guardia","status":"active"}`)
		case "GET /api/v1/rooms/r1/participants":
			_, _ = io.WriteString(w, `{"participants":[{"id":"p1","identity":"u1","name":"Ada"}]}`)
		case "GET /api/v1/rooms/r1/recordings":
			_, _ = io.WriteString(w, `{"recordings":[{"id":"g1","status":"completed","size_bytes":2048,"duration_sec":90}]}`)
		case "GET /api/v1/rooms/r1/recordings/g1":
			_, _ = io.WriteString(w, `{"url":"https://s3.eu/g1"}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	r, ctx := NewRooms(srv.URL, nil, nil), context.Background()
	ms, err := r.Meetings(ctx)
	if err != nil || len(ms) != 1 || ms[0].Titulo() != "sala-1" || ms[0].StartedAt == nil || !ms[0].Locked {
		t.Fatalf("reuniones: %+v, %v", ms, err)
	}
	if m, err := r.CreateMeeting(ctx, "Guardia"); err != nil || m.ID != "r2" || m.Titulo() != "Guardia" {
		t.Errorf("convocar: %+v, %v", m, err)
	}
	ps, err := r.MeetingParticipants(ctx, "r1")
	if err != nil || len(ps) != 1 || ps[0].Identity != "u1" {
		t.Errorf("participantes: %+v, %v", ps, err)
	}
	gs, err := r.MeetingRecordings(ctx, "r1")
	if err != nil || len(gs) != 1 || gs[0].Status != "completed" || gs[0].DurationSec != 90 {
		t.Errorf("grabaciones: %+v, %v", gs, err)
	}
	if u, err := r.RecordingURL(ctx, "r1", "g1"); err != nil || u != "https://s3.eu/g1" {
		t.Errorf("descarga: %q, %v", u, err)
	}
	if err := r.RemoveMeetingParticipant(ctx, "r1", "u1"); err != nil {
		t.Error(err)
	}
	if err := r.KnockMeeting(ctx, "r1", ""); err != nil {
		t.Error(err)
	}
	if err := r.CloseMeeting(ctx, "r1"); err != nil {
		t.Error(err)
	}
	quiero := []string{
		"GET /api/v1/rooms?limit=50&status=active ",
		`POST /api/v1/rooms {"title":"Guardia"}`,
		"GET /api/v1/rooms/r1/participants?present=1 ",
		"GET /api/v1/rooms/r1/recordings ",
		"GET /api/v1/rooms/r1/recordings/g1 ",
		"DELETE /api/v1/rooms/r1/participants/u1 ",
		"POST /api/v1/rooms/r1/knock ",
		"DELETE /api/v1/rooms/r1 ",
	}
	if strings.Join(pedidas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("pedidas:\n%s", strings.Join(pedidas, "\n"))
	}

	rutas := webref.Read(t, "services/rooms/internal/media/http/server.go")
	for _, ruta := range []string{
		`grupo.Get("/rooms", h.ListRooms)`, `grupo.Post("/rooms", h.CreateRoom)`,
		`api.Delete("/rooms/{id}", h.CloseRoom)`, `api.Get("/rooms/{id}/participants", h.ListParticipants)`,
		`api.Get("/rooms/{id}/recordings", h.ListRecordings)`, `api.Get("/rooms/{id}/recordings/{recID}", h.GetRecording)`,
		`api.Delete("/rooms/{id}/participants/{identity}", h.RemoveParticipant)`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("el servicio ya no sirve %s", ruta)
		}
	}
	dominio := webref.Read(t, "services/rooms/internal/media/domain/domain.go")
	for _, campo := range []string{"`json:\"started_at,omitempty\"`", "`json:\"identity\"`", "`json:\"size_bytes,omitempty\"`", "`json:\"duration_sec,omitempty\"`"} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("el dominio ya no lleva %s", campo)
		}
	}
	if h := webref.Read(t, "services/rooms/internal/media/http/handlers.go"); !strings.Contains(h, `map[string]any{"url": u}`) {
		t.Error("la descarga de una grabación ya no contesta {url}")
	}
}

// Entrar en la llamada son las rutas de `lib/meet.ts` de la web: join con el nombre,
// from-source + join para un canal, by-source + knock para su puerta, y la tarjeta
// `type=call`. La puerta echada se reconoce por su código, no por el 409 a secas.
//
// VISTO EN ROJO (2026-09-25, en el frontal): con `"kind": "calendar"` en CallChannel la
// sala del canal no se encuentra y el segundo caso falla; y quitando la comprobación de
// `room_locked`, un 409 de sala cerrada pasa por puerta echada.
func TestEntrarEnLaLlamada(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := io.ReadAll(r.Body)
		pedidas = append(pedidas, r.Method+" "+r.URL.RequestURI()+" "+string(c))
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/rooms/r1/join", "POST /api/v1/rooms/rc/join":
			_, _ = io.WriteString(w, `{"room":{"id":"rc","name":"sala-c","title":"general"},"token":"jwt","livekit_url":"wss://sfu.sytue.eu","identity":"u1","role":"speaker"}`)
		case "POST /api/v1/rooms/from-source":
			if !strings.Contains(string(c), `"kind":"chat"`) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, `{"id":"rc","name":"sala-c"}`)
		case "GET /api/v1/rooms/by-source":
			_, _ = io.WriteString(w, `{"id":"rc","name":"sala-c"}`)
		case "POST /api/v1/rooms/cerrada/join":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"room_locked","message":"locked"}}`)
		case "POST /api/v1/rooms/fin/join":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"conflict","message":"closed"}}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	r, ctx := NewRooms(srv.URL, nil, nil), context.Background()
	if c, err := r.JoinMeeting(ctx, "r1", "Ada", ""); err != nil || c.Token != "jwt" || c.LiveKitURL != "wss://sfu.sytue.eu" || c.Room.Titulo() != "general" {
		t.Errorf("join: %+v, %v", c, err)
	}
	if c, err := r.CallChannel(ctx, "c1", "general", "Ada"); err != nil || c.Token != "jwt" {
		t.Errorf("llamar en el canal: %+v, %v", c, err)
	}
	if err := r.KnockChannel(ctx, "c1"); err != nil {
		t.Error(err)
	}
	if err := r.PostCall(ctx, "c1"); err != nil {
		t.Error(err)
	}
	if _, err := r.JoinMeeting(ctx, "cerrada", "", ""); !ErrSalaCerrada(err) {
		t.Errorf("la puerta echada no se reconoce: %v", err)
	}
	if _, err := r.JoinMeeting(ctx, "fin", "", ""); err == nil || ErrSalaCerrada(err) {
		t.Errorf("una sala terminada pasa por puerta echada: %v", err)
	}
	quiero := []string{
		`POST /api/v1/rooms/r1/join {"display_name":"Ada"}`,
		`POST /api/v1/rooms/from-source {"kind":"chat","ref_id":"c1","title":"general"}`,
		`POST /api/v1/rooms/rc/join {"display_name":"Ada"}`,
		"GET /api/v1/rooms/by-source?kind=chat&ref_id=c1 ",
		"POST /api/v1/rooms/rc/knock ",
		`POST /api/v1/channels/c1/messages {"body":"","type":"call"}`,
	}
	if got := strings.Join(pedidas[:len(quiero)], "\n"); got != strings.Join(quiero, "\n") {
		t.Errorf("rutas:\n%s\nquiero:\n%s", got, strings.Join(quiero, "\n"))
	}
}

// Moderar son las rutas del servicio que la web llama por su BFF (`api/meeting.ts`).
//
// VISTO EN ROJO (2026-09-26, en el frontal): con POST para quitar la palabra, el
// servicio la DEVOLVÍA; y la cola de la puerta sin su sobre `knocks` llega vacía.
func TestModerarLaLlamada(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidas = append(pedidas, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/rooms/r1/participants/mute-all":
			_, _ = io.WriteString(w, `{"muted":3}`)
		case "GET /api/v1/rooms/r1/knocks":
			_, _ = io.WriteString(w, `{"knocks":[{"identity":"u9","name":"Eva"}]}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	r, ctx := NewRooms(srv.URL, nil, nil), context.Background()
	if n, err := r.MuteAllMeeting(ctx, "r1"); err != nil || n != 3 {
		t.Errorf("silenciar a todos: %d, %v", n, err)
	}
	if ks, err := r.MeetingKnocks(ctx, "r1"); err != nil || len(ks) != 1 || ks[0].Name != "Eva" {
		t.Errorf("la puerta: %+v, %v", ks, err)
	}
	for _, err := range []error{
		r.MuteMeetingParticipant(ctx, "r1", "u2"),
		r.SetMeetingSpeech(ctx, "r1", "u2", false),
		r.SetMeetingSpeech(ctx, "r1", "u2", true),
		r.LockMeeting(ctx, "r1", true),
		r.LockMeeting(ctx, "r1", false),
		r.AnswerMeetingKnock(ctx, "r1", "u9", true),
		r.AnswerMeetingKnock(ctx, "r1", "u9", false),
		r.RecordMeeting(ctx, "r1", true),
		r.RecordMeeting(ctx, "r1", false),
	} {
		if err != nil {
			t.Error(err)
		}
	}
	quiero := []string{
		"POST /api/v1/rooms/r1/participants/mute-all",
		"GET /api/v1/rooms/r1/knocks",
		"POST /api/v1/rooms/r1/participants/u2/mute",
		"DELETE /api/v1/rooms/r1/participants/u2/speech",
		"POST /api/v1/rooms/r1/participants/u2/speech",
		"POST /api/v1/rooms/r1/lock",
		"DELETE /api/v1/rooms/r1/lock",
		"POST /api/v1/rooms/r1/knocks/u9",
		"DELETE /api/v1/rooms/r1/knocks/u9",
		"POST /api/v1/rooms/r1/recording",
		"DELETE /api/v1/rooms/r1/recording",
	}
	if strings.Join(pedidas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("rutas:\n%s", strings.Join(pedidas, "\n"))
	}
}

// Reunión suelta (decidido por el dueño el 2026-10-16: «Invitados y quien tenga el
// enlace»): la clave del enlace viaja en la cabecera al entrar y al llamar, nunca en la
// ruta, y el enlace que se copia la lleva.
func TestLaClaveDelEnlaceViajaAlEntrarYAlLlamar(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vistas = append(vistas, r.URL.Path+" "+r.Header.Get(CabeceraClave))
		_, _ = io.WriteString(w, `{"room":{"id":"r1"},"token":"jwt"}`)
	}))
	defer srv.Close()
	r, ctx := NewRooms(srv.URL, nil, nil), context.Background()
	if _, err := r.JoinMeeting(ctx, "r1", "Ada", "k-1"); err != nil {
		t.Fatal(err)
	}
	if err := r.KnockMeeting(ctx, "r1", "k-1"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/api/v1/rooms/r1/join k-1", "/api/v1/rooms/r1/knock k-1"}; strings.Join(vistas, "|") != strings.Join(want, "|") {
		t.Fatalf("peticiones %v, quiero %v", vistas, want)
	}
	const sala = "0f8e2c64-8a6b-4c1e-9d3f-5b7a1c2e4d60"
	enlace := EnlaceReunion("https://rooms.sytue.eu", sala, "k-1")
	u, _ := url.Parse(enlace)
	if id, ok := LeerReunion("sytue.eu", u); !ok || id != sala || ClaveDeReunion(u) != "k-1" {
		t.Fatalf("enlace %q: id %q, clave %q", enlace, id, ClaveDeReunion(u))
	}
}
