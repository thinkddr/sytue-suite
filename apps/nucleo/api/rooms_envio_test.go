// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Lo que viaja con un envío es lo que el servicio lee, con sus nombres, y lo que va a
// cero no viaja: un `"priority":""` lo rechazaría la validación del servicio.
//
// VISTO EN ROJO: con `json:"requires_ack"` sin `omitempty` en RoomsSend, el cuerpo del
// envío sencillo lleva `"requires_ack":false` y el primer caso falla.
func TestLoQueViajaConUnEnvio(t *testing.T) {
	cuerpos := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := io.ReadAll(r.Body)
		cuerpos[r.Method+" "+r.URL.Path] = string(c)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/messages/m1/ack":
			_, _ = io.WriteString(w, `{"count":4}`)
		case "GET /api/v1/messages/m1/readers":
			_, _ = io.WriteString(w, `{"readers":[{"subject":"u2","name":"Ada","read_at":"2026-09-23T10:00:00Z"}],"total":12}`)
		case "PUT /api/v1/messages/m1/card/0", "DELETE /api/v1/messages/m1/card/done":
			_, _ = io.WriteString(w, `{"votes":[{"value":"0","count":3,"subs":["u1"]}]}`)
		case "PATCH /api/v1/scheduled/p1":
			_, _ = io.WriteString(w, `{"id":"p1","channel_id":"c1","body":"luego","send_at":"2026-09-25T07:00:00Z"}`)
		default:
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"x","body":"hola","requires_ack":true,"ack_count":2,"acked":true,"mentions_all":true,`+
				`"attachments":[{"file_id":"f1","name":"a.pdf","size_bytes":10}],"card":{"kind":"poll","options":["Sí","No"],`+
				`"votes":[{"value":"0","count":1,"subs":["u1"]}]}}`)
		}
	}))
	defer srv.Close()
	r, ctx := NewRooms(srv.URL, nil, nil), context.Background()

	if _, err := r.PostWith(ctx, "c1", "hola", RoomsSend{}); err != nil {
		t.Fatal(err)
	}
	if got := cuerpos["POST /api/v1/channels/c1/messages"]; got != `{"body":"hola"}` {
		t.Errorf("envío sencillo: %s", got)
	}
	m, err := r.PostWith(ctx, "c1", "hola @Ada", RoomsSend{Mentions: []string{"u2"}, Priority: "urgent", RequiresAck: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := cuerpos["POST /api/v1/channels/c1/messages"]; got != `{"body":"hola @Ada","mentions":["u2"],"priority":"urgent","requires_ack":true}` {
		t.Errorf("envío con opciones: %s", got)
	}
	if !m.RequiresAck || m.AckCount != 2 || !m.Acked || !m.MentionsAll || len(m.Attachments) != 1 ||
		m.Card == nil || m.Card.Kind != "poll" || len(m.Card.Votes) != 1 {
		t.Errorf("el mensaje devuelto no trae sus campos: %+v", m)
	}
	if _, err := r.ReplyWith(ctx, "m1", "sí", RoomsSend{Mentions: []string{"u3"}}); err != nil {
		t.Fatal(err)
	}
	if got := cuerpos["POST /api/v1/messages/m1/replies"]; got != `{"body":"sí","mentions":["u3"]}` {
		t.Errorf("respuesta: %s", got)
	}

	cuando := time.Date(2026, 9, 24, 9, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	if _, err := r.Schedule(ctx, "c1", "luego", nil, cuando); err != nil {
		t.Fatal(err)
	}
	if got := cuerpos["POST /api/v1/channels/c1/messages"]; got != `{"body":"luego","send_at":"2026-09-24T07:00:00Z"}` {
		t.Errorf("programar: %s", got)
	}
	p, err := r.Reschedule(ctx, "p1", cuando.Add(24*time.Hour))
	if err != nil || p.SendAt.IsZero() {
		t.Fatalf("reprogramar: %+v, %v", p, err)
	}
	if got := cuerpos["PATCH /api/v1/scheduled/p1"]; got != `{"send_at":"2026-09-25T07:00:00Z"}` {
		t.Errorf("reprogramar: %s", got)
	}
	if n, err := r.Ack(ctx, "m1"); err != nil || n != 4 {
		t.Errorf("acuse: %d, %v", n, err)
	}
	if vs, err := r.VoteCard(ctx, "m1", "0", true); err != nil || len(vs) != 1 || vs[0].Count != 3 {
		t.Errorf("votar: %+v, %v", vs, err)
	}
	if _, err := r.VoteCard(ctx, "m1", "done", false); err != nil {
		t.Errorf("reabrir la tarea: %v", err)
	}
	ls, total, err := r.Readers(ctx, "m1")
	if err != nil || total != 12 || len(ls) != 1 || ls[0].Name != "Ada" || ls[0].ReadAt.IsZero() {
		t.Errorf("lectores: %+v de %d, %v", ls, total, err)
	}
}

// Los nombres de arriba son los del servicio: si cambian allí, esto se entera aquí.
func TestLosCamposDelEnvioSonLosDelServicio(t *testing.T) {
	manejadores := webref.Read(t, "services/rooms/internal/http/handlers.go")
	for _, literal := range []string{
		"`json:\"mentions,omitempty\"`", "`json:\"file_ids,omitempty\"`", "`json:\"priority,omitempty\"`",
		"`json:\"requires_ack,omitempty\"`", "`json:\"send_at,omitempty\"`", "`json:\"card,omitempty\"`",
		`map[string]any{"count": count}`, `map[string]any{"readers": lectores, "total": total}`,
		`map[string]any{"votes": votes}`,
	} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/rooms/internal/http/handlers.go ya no dice %s", literal)
		}
	}
	dominio := webref.Read(t, "services/rooms/internal/domain/domain.go") + webref.Read(t, "services/rooms/internal/domain/cards.go")
	for _, campo := range []string{
		`json:"requires_ack,omitempty"`, `json:"ack_count,omitempty"`, `json:"acked,omitempty"`, `json:"mentions_all,omitempty"`,
		`json:"attachments,omitempty"`, `json:"file_id"`, `json:"content_type,omitempty"`, `json:"size_bytes,omitempty"`,
		`json:"options,omitempty"`, `json:"multi,omitempty"`, `json:"assignee_sub,omitempty"`, `json:"votes,omitempty"`,
		`json:"value"`,
	} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("services/rooms ya no declara %s", campo)
		}
	}
	if u := webref.Read(t, "services/rooms/internal/domain/unfurl.go"); !strings.Contains(u, "`json:\"image_url,omitempty\"`") ||
		!strings.Contains(u, "`json:\"site_name,omitempty\"`") {
		t.Error("services/rooms ya no dice image_url y site_name en el unfurl")
	}
	if !strings.Contains(webref.Read(t, "services/rooms/internal/store/readers.go"), "`json:\"read_at\"`") {
		t.Error("services/rooms ya no dice read_at en los lectores")
	}
	rutas := webref.Read(t, "services/rooms/internal/http/server.go")
	for _, ruta := range []string{
		`porMensaje.Post("/messages/{messageID}/ack", h.AckMessage)`, `porMensaje.Get("/messages/{messageID}/readers", h.MessageReaders)`,
		`api.Patch("/scheduled/{scheduledID}", h.UpdateScheduled)`,
		`porMensaje.Put("/messages/{messageID}/card/{value}", h.SetCardVote)`,
		`porMensaje.Delete("/messages/{messageID}/card/{value}", h.ClearCardVote)`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/rooms ya no monta %s", ruta)
		}
	}
}

// Los comandos «/» hablan con las rutas del servicio y con sus nombres.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `"line"` en vez de `"input"` en
// ExecuteCommand, el cuerpo no casa.
func TestLosComandosHablanConElServicio(t *testing.T) {
	cuerpos := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := io.ReadAll(r.Body)
		cuerpos[r.Method+" "+r.URL.Path] = string(c)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/commands":
			_, _ = io.WriteString(w, `{"commands":[{"name":"desplegar","description":"Despliega","usage_hint":"[entorno]","request_url":"x"}]}`)
		case "POST /api/v1/commands/execute":
			_, _ = io.WriteString(w, `{"text":"hecho","response_type":"ephemeral"}`)
		case "GET /api/v1/unfurl":
			if r.URL.Query().Get("url") != "https://sytue.eu/precios" {
				t.Errorf("unfurl de %q", r.URL.Query().Get("url"))
			}
			_, _ = io.WriteString(w, `{"unfurl":{"url":"https://sytue.eu/precios","title":"Precios","image_url":"/api/v1/unfurl/imagen/ab","site_name":"Sytue"}}`)
		case "GET /api/v1/unfurl/imagen/ab":
			_, _ = io.WriteString(w, "png")
		case "PATCH /api/v1/channels/c1":
			_, _ = io.WriteString(w, `{"id":"c1","topic":"nuevo"}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	r, ctx := NewRooms(srv.URL, nil, nil), context.Background()
	cs, err := r.Commands(ctx)
	if err != nil || len(cs) != 1 || cs[0].UsageHint != "[entorno]" {
		t.Fatalf("comandos: %+v, %v", cs, err)
	}
	res, err := r.ExecuteCommand(ctx, "c1", "/desplegar prod")
	if err != nil || res.Text != "hecho" || res.ResponseType != "ephemeral" {
		t.Errorf("ejecutar: %+v, %v", res, err)
	}
	if got := cuerpos["POST /api/v1/commands/execute"]; got != `{"channel_id":"c1","input":"/desplegar prod"}` {
		t.Errorf("ejecutar manda %s", got)
	}
	u, err := r.Unfurl(ctx, "https://sytue.eu/precios")
	if err != nil || u.Title != "Precios" || u.SiteName != "Sytue" {
		t.Fatalf("unfurl: %+v, %v", u, err)
	}
	if rc, err := r.UnfurlImage(ctx, u.ImageURL); err != nil {
		t.Errorf("imagen del unfurl: %v", err)
	} else {
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		if string(b) != "png" {
			t.Errorf("imagen del unfurl: %q", b)
		}
	}
	tema := "nuevo"
	if c, err := r.UpdateChannel(ctx, "c1", RoomsChannelPatch{Topic: &tema}); err != nil || c.Topic != "nuevo" {
		t.Errorf("tema: %+v, %v", c, err)
	}
	if got := cuerpos["PATCH /api/v1/channels/c1"]; got != `{"topic":"nuevo"}` {
		t.Errorf("tema manda %s", got)
	}
	if err := r.LeaveChannel(ctx, "c1"); err != nil || cuerpos["POST /api/v1/channels/c1/leave"] != "" {
		t.Errorf("salir: %v", err)
	}
	if err := r.DeleteChannel(ctx, "c1"); err != nil {
		t.Errorf("borrar: %v", err)
	}
	if _, ok := cuerpos["DELETE /api/v1/channels/c1"]; !ok {
		t.Error("borrar no pide DELETE /channels/c1")
	}
	if err := r.HideDirect(ctx, "c1"); err != nil || cuerpos["POST /api/v1/channels/c1/hide"] != "" {
		t.Errorf("ocultar el directo: %v", err)
	}
	if err := r.AddTeamMember(ctx, "t1", "u2", "Ada"); err != nil || cuerpos["POST /api/v1/teams/t1/members"] != `{"subject":"u2","name":"Ada"}` {
		t.Errorf("invitar: %v, %s", err, cuerpos["POST /api/v1/teams/t1/members"])
	}
	if err := r.RemoveTeamMember(ctx, "t1", "u2"); err != nil {
		t.Errorf("sacar: %v", err)
	}
	if _, ok := cuerpos["DELETE /api/v1/teams/t1/members/u2"]; !ok {
		t.Error("sacar no pide DELETE /teams/t1/members/u2")
	}
	rutas := webref.Read(t, "services/rooms/internal/http/server.go")
	for _, ruta := range []string{
		`api.Get("/commands", ih.ListCommands)`, `api.Post("/commands/execute", ih.ExecuteCommand)`,
		`porCanal.Post("/channels/{channelID}/leave", h.LeaveChannel)`, `porCanal.Delete("/channels/{channelID}", h.DeleteChannel)`,
		`porCanal.Post("/channels/{channelID}/hide", h.HideDirect)`, `api.Get("/unfurl", h.Unfurl)`, `api.Get("/unfurl/imagen/{hash}", h.UnfurlImagen)`, `api.Post("/teams/{teamID}/members", h.AddTeamMember)`,
		`api.Delete("/teams/{teamID}/members/{sub}", h.RemoveTeamMember)`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/rooms ya no monta %s", ruta)
		}
	}
	comandos := webref.Read(t, "services/rooms/internal/http/slashcommands.go") + webref.Read(t, "services/rooms/internal/service/slashcommands.go") +
		webref.Read(t, "services/rooms/internal/domain/slashcommands.go")
	for _, campo := range []string{"`json:\"input\"`", "`json:\"channel_id\"`", "`json:\"response_type\"`", "`json:\"usage_hint,omitempty\"`"} {
		if !strings.Contains(comandos, campo) {
			t.Errorf("los comandos del servicio ya no dicen %s", campo)
		}
	}
}
