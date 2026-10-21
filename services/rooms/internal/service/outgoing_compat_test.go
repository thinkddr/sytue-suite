// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/services/rooms/internal/compat"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// TestOutgoingBodyNativeSinCambios: el dialecto por defecto sigue enviando el MISMO
// JSON de siempre (los campos de compat no viajan en él).
func TestOutgoingBodyNativeSinCambios(t *testing.T) {
	req := outgoingRequest{Token: "t", ChannelID: "c", Text: "hola", Tenant: "acme", TriggerWord: "deploy"}
	for _, dialect := range []string{"", compat.Native} {
		body, err := outgoingBody(dialect, req)
		if err != nil {
			t.Fatalf("outgoingBody(%q): %v", dialect, err)
		}
		want := `{"token":"t","channel_id":"c","message_id":"","user_id":"","user_name":"","text":"hola","timestamp":0}`
		if string(body) != want {
			t.Fatalf("cuerpo nativo =\n%s\nquiero\n%s", body, want)
		}
	}
}

// TestOutgoingBodySlack: dialecto slack ⇒ form-urlencoded con los campos que espera
// un bot escrito para los outgoing webhooks de Slack.
func TestOutgoingBodySlack(t *testing.T) {
	body, err := outgoingBody(compat.Slack, outgoingRequest{
		Token: "verif", Tenant: "acme", ChannelID: "c-1", UserID: "u-1",
		UserName: "Ana", Text: "deploy ya", TriggerWord: "deploy", Timestamp: 1700000000,
	})
	if err != nil {
		t.Fatalf("outgoingBody: %v", err)
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("no es form-urlencoded: %v", err)
	}
	for k, want := range map[string]string{
		"token": "verif", "team_domain": "acme", "channel_id": "c-1", "channel_name": "c-1",
		"user_id": "u-1", "user_name": "Ana", "text": "deploy ya", "trigger_word": "deploy",
	} {
		if got := vals.Get(k); got != want {
			t.Fatalf("%s = %q, quiero %q", k, got, want)
		}
	}
}

// TestDoOutgoingSlackDialect: el POST va form-urlencoded y la respuesta estilo Slack
// se traduce a texto de Rooms para publicarla.
func TestDoOutgoingSlackDialect(t *testing.T) {
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_, _ = w.Write([]byte(`{"response_type":"in_channel","text":"hecho *ya*"}`))
	}))
	defer srv.Close()

	// Cliente SIN la barrera del dialer (el destino es loopback): aquí se aísla el
	// shape del request/response; la barrera SSRF la cubre outgoing_test.go.
	svc := &Service{outgoing: srv.Client()}
	w := domain.OutgoingWebhook{CallbackURL: srv.URL, Token: "verif", Dialect: compat.Slack}
	text, err := svc.doOutgoing(context.Background(), w, []byte("text=hola&token=verif"))
	if err != nil {
		t.Fatalf("doOutgoing: %v", err)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	if gotBody != "text=hola&token=verif" {
		t.Fatalf("cuerpo = %q", gotBody)
	}
	if text != "" {
		t.Fatalf("Slack response must not enter the conversation: %q", text)
	}
}

// TestMatchedTriggerAlimentaSlack: `trigger_word` sale de la misma regla que decide
// el disparo (sin lógica duplicada que pueda divergir).
func TestMatchedTriggerAlimentaSlack(t *testing.T) {
	words := []string{"deploy", "estado"}
	if got := domain.MatchedTrigger("deploy prod ya", words); got != "deploy" {
		t.Fatalf("MatchedTrigger = %q", got)
	}
	if got := domain.MatchedTrigger("nada que ver", words); got != "" {
		t.Fatalf("sin coincidencia debe ser vacío, got %q", got)
	}
}
