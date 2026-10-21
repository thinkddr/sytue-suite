// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package meethttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/livekit"
)

// fakeSink implementa webhookSink registrando el evento recibido.
type fakeSink struct {
	ev  livekit.WebhookEvent
	err error
	hit bool
}

func (f *fakeSink) HandleWebhookEvent(_ context.Context, ev livekit.WebhookEvent) error {
	f.hit = true
	f.ev = ev
	return f.err
}

// signWebhookToken mintea un JWT HS256 con el claim sha256 = hash(body), tal como
// hace LiveKit al firmar sus webhooks. Reproduce el formato que verifica el
// Signer sin exponer helpers internos de producción.
func signWebhookToken(t *testing.T, secret string, body []byte) string {
	t.Helper()
	b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	sum := sha256.Sum256(body)
	payload, _ := json.Marshal(map[string]any{
		"sha256": base64.StdEncoding.EncodeToString(sum[:]),
	})
	signingInput := b64(header) + "." + b64(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	return signingInput + "." + b64(mac.Sum(nil))
}

const testSecret = "test-secret"

func newWebhookHandler(sink *fakeSink) *WebhookHandler {
	return &WebhookHandler{svc: sink, signer: livekit.NewSigner("api-key", testSecret)}
}

func TestWebhookValidSignature(t *testing.T) {
	sink := &fakeSink{}
	wh := newWebhookHandler(sink)

	body := []byte(`{"event":"room_started","room":{"name":"r1","sid":"s1"}}`)
	token := signWebhookToken(t, testSecret, body)

	r := httptest.NewRequest(http.MethodPost, "/webhooks/livekit", strings.NewReader(string(body)))
	r.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	wh.Receive(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("webhook válido: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !sink.hit || sink.ev.Event != "room_started" {
		t.Errorf("evento no procesado: hit=%v ev=%q", sink.hit, sink.ev.Event)
	}
}

func TestWebhookMissingAuth(t *testing.T) {
	wh := newWebhookHandler(&fakeSink{})
	r := httptest.NewRequest(http.MethodPost, "/webhooks/livekit", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	wh.Receive(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth: code=%d", rec.Code)
	}
}

func TestWebhookBadSignature(t *testing.T) {
	wh := newWebhookHandler(&fakeSink{})
	body := []byte(`{"event":"room_started"}`)
	// Firmar con un secreto distinto -> firma inválida.
	token := signWebhookToken(t, "otro-secreto", body)

	r := httptest.NewRequest(http.MethodPost, "/webhooks/livekit", strings.NewReader(string(body)))
	r.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	wh.Receive(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("firma mala: code=%d", rec.Code)
	}
}

func TestWebhookBodyHashMismatch(t *testing.T) {
	wh := newWebhookHandler(&fakeSink{})
	// Token firmado para OTRO cuerpo -> el hash no coincide -> rechazado.
	token := signWebhookToken(t, testSecret, []byte(`{"event":"x"}`))

	r := httptest.NewRequest(http.MethodPost, "/webhooks/livekit", strings.NewReader(`{"event":"room_started"}`))
	r.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	wh.Receive(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("hash no coincide: code=%d", rec.Code)
	}
}

func TestWebhookSinkErrorReturns500(t *testing.T) {
	sink := &fakeSink{err: context.DeadlineExceeded}
	wh := newWebhookHandler(sink)
	body := []byte(`{"event":"room_finished","room":{"name":"r1"}}`)
	token := signWebhookToken(t, testSecret, body)

	r := httptest.NewRequest(http.MethodPost, "/webhooks/livekit", strings.NewReader(string(body)))
	r.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	wh.Receive(rec, r)
	// 500 para que LiveKit reintente la entrega.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("error de sink: code=%d", rec.Code)
	}
}
