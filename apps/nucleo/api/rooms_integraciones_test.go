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

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// «Apps e integraciones» habla con las rutas del servicio y con sus nombres; el
// dialecto por defecto no viaja.
//
// VISTO EN ROJO (2026-09-24, en el frontal): sin quitar "sytue" en
// CreateOutgoingWebhook, el cuerpo lleva `"dialect":"sytue"` y el caso falla.
func TestLasIntegraciones(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := io.ReadAll(r.Body)
		pedidas = append(pedidas, r.Method+" "+r.URL.Path+" "+string(c))
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/channels/c1/webhooks/incoming":
			_, _ = io.WriteString(w, `{"webhooks":[{"id":"i1","channel_id":"c1","name":"CI"}]}`)
		case "POST /api/v1/channels/c1/webhooks/incoming":
			_, _ = io.WriteString(w, `{"webhook":{"id":"i1","channel_id":"c1","name":"CI"},"token":"tk","url":"https://rooms.eu/hooks/tk","channel_mail_domain":"rooms.eu"}`)
		case "POST /api/v1/webhooks/incoming/i1/rotate":
			_, _ = io.WriteString(w, `{"webhook":{"id":"i1","channel_id":"c1","name":"CI"},"token":"tk2","url":"https://rooms.eu/hooks/tk2"}`)
		case "POST /api/v1/webhooks/outgoing":
			_, _ = io.WriteString(w, `{"webhook":{"id":"o1","callback_url":"https://bot.eu","signing_secret":"ss"}}`)
		case "GET /api/v1/bots":
			_, _ = io.WriteString(w, `{"bots":[{"id":"b1","name":"Deploy"}]}`)
		case "POST /api/v1/bots":
			_, _ = io.WriteString(w, `{"bot":{"id":"b2","name":"Nuevo"},"token":"bot_x"}`)
		case "GET /api/v1/whatsapp":
			_, _ = io.WriteString(w, `{"accounts":[{"id":"w1","team_id":"t1","phone_number_id":"99","display_phone":"+34 600"}]}`)
		case "POST /api/v1/commands":
			_, _ = io.WriteString(w, `{"command":{"id":"k1","name":"deploy","request_url":"https://x.eu","token":"t","signing_secret":"s"}}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	r, ctx := NewRooms(srv.URL, nil, nil), context.Background()
	if ws, err := r.IncomingWebhooks(ctx, "c1"); err != nil || len(ws) != 1 {
		t.Errorf("entrantes: %+v, %v", ws, err)
	}
	if s, err := r.CreateIncomingWebhook(ctx, "c1", "CI", ""); err != nil || s.Token != "tk" || s.URL == "" || s.ChannelMailDomain != "rooms.eu" {
		t.Errorf("crear entrante: %+v, %v", s, err)
	}
	// Sin `channel_mail_domain`, el correo a canal está apagado: nada que componer.
	if s, err := r.RotateIncomingWebhook(ctx, "i1"); err != nil || s.Token != "tk2" || s.ChannelMailDomain != "" {
		t.Errorf("rotar: %+v, %v", s, err)
	}
	o, err := r.CreateOutgoingWebhook(ctx, RoomsOutgoingWebhook{CallbackURL: "https://bot.eu", TriggerWords: []string{"deploy"}, Dialect: "sytue"})
	if err != nil || o.SigningSecret != "ss" {
		t.Errorf("saliente: %+v, %v", o, err)
	}
	if bs, err := r.Bots(ctx); err != nil || len(bs) != 1 {
		t.Errorf("bots: %+v, %v", bs, err)
	}
	if b, tok, err := r.CreateBot(ctx, "Nuevo"); err != nil || b.ID != "b2" || tok != "bot_x" {
		t.Errorf("crear bot: %+v %q, %v", b, tok, err)
	}
	if err := r.SetBotInChannel(ctx, "b1", "c1", true); err != nil {
		t.Error(err)
	}
	if err := r.SetBotInChannel(ctx, "b1", "c1", false); err != nil {
		t.Error(err)
	}
	if as, err := r.WhatsApp(ctx); err != nil || len(as) != 1 || as[0].DisplayPhone != "+34 600" {
		t.Errorf("whatsapp: %+v, %v", as, err)
	}
	if c, err := r.CreateCommand(ctx, RoomsCommand{Name: "deploy", RequestURL: "https://x.eu"}); err != nil || c.SigningSecret != "s" {
		t.Errorf("comando: %+v, %v", c, err)
	}
	for _, f := range []func() error{
		func() error { return r.DeleteIncomingWebhook(ctx, "i1") }, func() error { return r.DeleteOutgoingWebhook(ctx, "o1") },
		func() error { return r.DeleteBot(ctx, "b1") }, func() error { return r.DeleteBridge(ctx, "p1") },
		func() error { return r.DisconnectWhatsApp(ctx, "w1") }, func() error { return r.DeleteCommand(ctx, "k1") },
	} {
		if err := f(); err != nil {
			t.Error(err)
		}
	}
	quiero := []string{
		"GET /api/v1/channels/c1/webhooks/incoming ",
		`POST /api/v1/channels/c1/webhooks/incoming {"name":"CI"}`,
		"POST /api/v1/webhooks/incoming/i1/rotate {}",
		`POST /api/v1/webhooks/outgoing {"callback_url":"https://bot.eu","trigger_words":["deploy"]}`,
		"GET /api/v1/bots ",
		`POST /api/v1/bots {"name":"Nuevo"}`,
		"POST /api/v1/bots/b1/channels/c1 {}",
		"DELETE /api/v1/bots/b1/channels/c1 ",
		"GET /api/v1/whatsapp ",
		`POST /api/v1/commands {"name":"deploy","request_url":"https://x.eu"}`,
		"DELETE /api/v1/webhooks/incoming/i1 ", "DELETE /api/v1/webhooks/outgoing/o1 ", "DELETE /api/v1/bots/b1 ",
		"DELETE /api/v1/bridges/p1 ", "DELETE /api/v1/whatsapp/w1 ", "DELETE /api/v1/commands/k1 ",
	}
	if strings.Join(pedidas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("pedidas:\n%s", strings.Join(pedidas, "\n"))
	}

	rutas := webref.Read(t, "services/rooms/internal/http/server.go")
	for _, ruta := range []string{
		`api.Get("/channels/{channelID}/webhooks/incoming", ih.ListIncoming)`, `api.Post("/webhooks/incoming/{id}/rotate", ih.RotateIncoming)`,
		`api.Post("/webhooks/outgoing", ih.CreateOutgoing)`, `api.Post("/bots/{botID}/channels/{channelID}", ih.AddBotChannel)`,
		`api.Post("/channels/{channelID}/bridges", ih.CreateBridge)`, `api.Post("/whatsapp/consent", ih.ConnectWhatsAppConsent)`,
		`api.Post("/commands", ih.CreateCommand)`, `api.Delete("/commands/{id}", ih.DeleteCommand)`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("el servicio ya no sirve %s", ruta)
		}
	}
	dominio := webref.Read(t, "services/rooms/internal/domain/whatsapp.go") + webref.Read(t, "services/rooms/internal/domain/bridges.go") +
		webref.Read(t, "services/rooms/internal/domain/slashcommands.go") + webref.Read(t, "services/rooms/internal/http/integrations.go")
	for _, campo := range []string{"`json:\"display_phone\"`", "`json:\"last_error,omitempty\"`", "`json:\"request_url\"`", "`json:\"signing_secret,omitempty\"`", "`json:\"callback_url\"`", `out["channel_mail_domain"]`} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("el servicio ya no lleva %s", campo)
		}
	}
}
