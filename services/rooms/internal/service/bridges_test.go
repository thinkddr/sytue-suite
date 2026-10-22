// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Cada plataforma recibe el mensaje en SU formato: Slack con mrkdwn escapado,
// Google Chat con el hilo de Rooms como clave de hilo, Teams como tarjeta.
func TestBridgeRequestPorPlataforma(t *testing.T) {
	notice := externalNotice(hubnotify.AvisoMensajeRooms("https://rooms.sytue.eu", "acme", uuid.NewString(), uuid.NewString()))

	_, cuerpo, err := bridgeRequest(domain.BridgeSlack, "https://hooks.slack.com/services/T/B/X", notice)
	if err != nil {
		t.Fatal(err)
	}
	var slack map[string]any
	_ = json.Unmarshal(cuerpo, &slack)
	if slack["text"] != string(notice) || slack["unfurl_links"] != false || slack["unfurl_media"] != false {
		t.Errorf("slack = %v", slack)
	}

	destino, cuerpo, err := bridgeRequest(domain.BridgeGoogleChat, "https://chat.googleapis.com/v1/spaces/AAAA/messages?key=k&token=t", notice)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(destino)
	if u.Query().Get("key") != "k" || u.Query().Get("token") != "t" {
		t.Errorf("la URL de Google Chat pierde su clave o no pide hilo: %s", destino)
	}
	var gchat map[string]any
	_ = json.Unmarshal(cuerpo, &gchat)
	if gchat["text"] != string(notice) || gchat["thread"] != nil {
		t.Errorf("google chat = %s", cuerpo)
	}

	_, cuerpo, err = bridgeRequest(domain.BridgeTeams, "https://acme.webhook.office.com/webhookb2/x", notice)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cuerpo), `"contentType":"application/vnd.microsoft.card.adaptive"`) || !strings.Contains(string(cuerpo), hubnotify.TextoAvisoRooms) {
		t.Errorf("teams = %s", cuerpo)
	}
}

// Un 2xx es entregado; cualquier otra cosa vuelve con el código y lo que dijo el
// otro lado, que es lo que se enseña al admin en la lista.
func TestPostBridge(t *testing.T) {
	var recibido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		recibido = string(b)
		if strings.Contains(recibido, "roto") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("no_service"))
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	svc := &Service{outgoing: srv.Client()}
	if err := svc.postBridge(context.Background(), srv.URL, []byte(`{"text":"hola"}`)); err != nil || recibido != `{"text":"hola"}` {
		t.Fatalf("entrega: %v (%q)", err, recibido)
	}
	if err := svc.postBridge(context.Background(), srv.URL, []byte(`{"text":"roto"}`)); err == nil || err.Error() != "HTTP 404" {
		t.Fatalf("rechazo: %v", err)
	}
	// Una URL fuera de la lista blanca no llega a salir, aunque resuelva.
	if err := svc.sendBridge(context.Background(), domain.Bridge{Platform: domain.BridgeSlack, WebhookURL: srv.URL}, bridgeMessage{Body: "x"}); err == nil {
		t.Fatal("un puente a un host cualquiera debe rechazarse")
	}
}

func TestOAuthBridgeCannotForwardConversationContent(t *testing.T) {
	var request map[string]string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer gateway.Close()
	svc := &Service{cfg: config.Config{PublicURL: "https://rooms.sytue.eu"}, roomsGateway: newRoomsGateway(gateway.URL, "internal-test")}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	m := bridgeMessage{ID: uuid.New(), ChannelID: uuid.New(), AuthorSub: "secret-user", AuthorName: "Private name", Body: "PRIVATE body and key"}
	if err := svc.sendBridge(ctx, domain.Bridge{InstanceID: uuid.NewString(), RemoteID: "spaces/AAAA"}, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(request)
	if request["channel_id"] != m.ChannelID.String() || request["message_id"] != m.ID.String() || len(request) != 5 || strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "Private name") || strings.Contains(string(raw), "secret-user") {
		t.Fatalf("gateway received conversation content: %s", raw)
	}
	svc.cfg.PublicURL = "http://rooms.sytue.eu"
	if err := svc.sendBridge(ctx, domain.Bridge{InstanceID: uuid.NewString()}, m); err == nil {
		t.Fatal("unsafe configuration must not fall back to forwarding content")
	}
}
