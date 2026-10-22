// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestValidateOutgoingURLRejectsPrivate(t *testing.T) {
	ctx := context.Background()
	// Barrera 1: literales privados/loopback y esquemas no http rechazados sin conectar.
	bad := []string{
		"http://127.0.0.1/hook",
		"http://10.1.2.3:9000/x",
		"http://169.254.169.254/latest/meta-data/", // metadata SSRF clásico
		"http://[::1]:8080/x",
		"http://localhost/x", // resuelve a loopback
		"ftp://example.com/x",
		"file:///etc/passwd",
	}
	for _, u := range bad {
		if err := validateOutgoingURL(ctx, u); err == nil {
			t.Fatalf("validateOutgoingURL(%q) debe RECHAZAR (SSRF)", u)
		}
	}
}

// TestPostOutgoingRejectsLoopback comprueba que el POST a un servidor real de
// loopback se rechaza SIN llegar a entregarse (la barrera SSRF corta antes).
func TestPostOutgoingRejectsLoopback(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := &Service{outgoing: ssrfSafeClient()}                       // postOutgoing/doOutgoing no leen cfg
	w := domain.OutgoingWebhook{CallbackURL: srv.URL, Token: "verif"} // srv.URL es 127.0.0.1
	text, err := svc.postOutgoing(context.Background(), w, outgoingRequest{Text: "hola"})
	if err == nil {
		t.Fatal("postOutgoing a loopback debe fallar por SSRF")
	}
	if !strings.Contains(err.Error(), "ssrf") {
		t.Fatalf("el error debe ser de SSRF, got: %v", err)
	}
	if text != "" {
		t.Fatalf("no debe devolver texto de un destino bloqueado: %q", text)
	}
	if hits != 0 {
		t.Fatalf("el servidor de loopback NO debió recibir la petición (hits=%d)", hits)
	}
}

// TestPostOutgoingDeliversToPublicShapedResponse ejercita el camino feliz del cliente
// (parseo de {text}) forzando el dialer a un servidor de test local: valida que, si el
// destino FUERA público, se entrega el POST con el token en cabecera y se lee {text}.
// Reemplaza el transporte por uno que evita la barrera del dialer (el destino real es
// loopback), aislando la lógica de request/parse de la barrera de red.
func TestPostOutgoingRequestShapeAndResponse(t *testing.T) {
	var gotToken, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Sytue-Webhook-Token")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_, _ = w.Write([]byte(`{"text":"pong"}`))
	}))
	defer srv.Close()

	// Cliente SIN la barrera del dialer (transport por defecto): aislamos el shape del
	// request/response; la barrera SSRF la cubre TestPostOutgoingRejectsLoopback.
	svc := &Service{outgoing: srv.Client()}
	w := domain.OutgoingWebhook{CallbackURL: srv.URL, Token: "verif-tok"}
	text, err := svc.doOutgoing(context.Background(), w, []byte(`{"text":"hola"}`))
	if err != nil {
		t.Fatalf("doOutgoing: %v", err)
	}
	if text != "" {
		t.Fatalf("provider response must not be imported: %q", text)
	}
	if gotToken != "verif-tok" {
		t.Fatalf("el token de verificación debe viajar en la cabecera, got %q", gotToken)
	}
	if !strings.Contains(gotBody, "hola") {
		t.Fatalf("el cuerpo del POST debe llevar el payload, got %q", gotBody)
	}
}

func TestOutgoingNoticeNeverContainsPrivateMessage(t *testing.T) {
	svc := &Service{cfg: config.Config{PublicURL: "https://rooms.sytue.eu"}}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	msg := outgoingPayload{ID: uuid.New(), ChannelID: uuid.New(), AuthorSub: "private-user", AuthorName: "Private Author", Body: "PRIVATE body key=secret"}
	w := domain.OutgoingWebhook{Tenant: "acme", Token: "verification", TriggerWords: []string{"PRIVATE"}}
	request, err := svc.outgoingNoticeRequest(ctx, w, msg)
	if err != nil {
		t.Fatal(err)
	}
	for _, dialect := range []string{"", "slack", "teams"} {
		body, err := outgoingBody(dialect, request)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{msg.AuthorSub, msg.AuthorName, "PRIVATE", "key=secret"} {
			if strings.Contains(string(body), private) {
				t.Fatalf("%s webhook leaked conversation content: %s", dialect, body)
			}
		}
	}
	if !strings.HasPrefix(request.Text, hubnotify.TextoAvisoRooms+"\nhttps://") || request.UserID != "" || request.TriggerWord != "" || len(request.TriggerWords) != 0 {
		t.Fatalf("not a private notice: %+v", request)
	}
}
