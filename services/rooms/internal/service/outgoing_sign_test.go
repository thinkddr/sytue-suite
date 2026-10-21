// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/rooms/internal/compat"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// El secreto de las pruebas: 32 bytes en base64 estándar, como los que genera
// domain.NewSigningSecret.
const testSecret = "c2VjcmV0by1kZS1wcnVlYmEtZGUtMzItYnl0ZXMh"

// verificaComoBolt reproduce, byte a byte, lo que hace un bot de Slack (Bolt o
// slack_sdk) al recibir una petición: recalcula el HMAC sobre «v0:<ts>:<cuerpo>» y lo
// compara con la cabecera. Si esto no cuadra, el bot responde 401 y el mensaje se pierde.
func verificaComoBolt(t *testing.T, r *http.Request, body, secret string) {
	t.Helper()
	ts := r.Header.Get(compat.SlackTimestampHeader)
	if ts == "" {
		t.Fatal("sin X-Slack-Request-Timestamp: un bot de Slack lo exige antes de mirar la firma")
	}
	// El receptor rechaza timestamps viejos para que una petición capturada no se pueda
	// reenviar indefinidamente; comprobamos que el que enviamos es utilizable.
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		t.Fatalf("timestamp no numérico: %q", ts)
	}
	if d := time.Since(time.Unix(n, 0)); d > 5*time.Minute || d < -time.Minute {
		t.Fatalf("timestamp fuera de la ventana de 5 min que aplica el receptor: %v", d)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + ts + ":" + body))
	want := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if got := r.Header.Get(compat.SlackSignatureHeader); got != want {
		t.Fatalf("X-Slack-Signature = %q, el receptor calcula %q", got, want)
	}
}

// TestSalienteFirmadoComoSlack: la entrega tiene que pasar la verificación EXACTA de
// Bolt/slack_sdk. Sin esto responden 401 y el bot no llega a ver el mensaje.
func TestSalienteFirmadoComoSlack(t *testing.T) {
	cuerpo := "text=hola&token=verif"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		verificaComoBolt(t, r, string(raw), testSecret)
		_, _ = w.Write([]byte(`{"text":"ok"}`))
	}))
	defer srv.Close()

	svc := &Service{outgoing: srv.Client()}
	w := domain.OutgoingWebhook{CallbackURL: srv.URL, Token: "verif", Dialect: compat.Slack, SigningSecret: testSecret}
	if _, err := svc.doOutgoing(context.Background(), w, []byte(cuerpo)); err != nil {
		t.Fatalf("doOutgoing: %v", err)
	}
}

// TestFirmaCubreElCuerpo: si la firma no cambiara con el cuerpo, no probaría nada — un
// intermediario podría reescribir el mensaje conservando la cabecera.
func TestFirmaCubreElCuerpo(t *testing.T) {
	a := compat.SignSlack(testSecret, 1700000000, []byte("text=hola"))
	b := compat.SignSlack(testSecret, 1700000000, []byte("text=adios"))
	if a == b {
		t.Fatal("dos cuerpos distintos dan la misma firma")
	}
	if c := compat.SignSlack(testSecret, 1700000001, []byte("text=hola")); a == c {
		t.Fatal("la firma no depende del timestamp: una petición capturada se podría reenviar")
	}
	if d := compat.SignSlack("otro-secreto", 1700000000, []byte("text=hola")); a == d {
		t.Fatal("la firma no depende del secreto")
	}
}

// TestSinSecretoNoSeFirma: un saliente sin secreto sale SIN cabecera de firma. Firmar
// con clave vacía sería peor que no firmar: cualquiera podría calcularla.
func TestSinSecretoNoSeFirma(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://ejemplo.test/hook", http.NoBody)
	signOutgoing(req, domain.OutgoingWebhook{Dialect: compat.Slack}, []byte("x"))
	if req.Header.Get(compat.SlackSignatureHeader) != "" {
		t.Fatal("firmó sin secreto")
	}
}

// TestSalienteTeamsFirmadoYTraducido: el dialecto Teams envía la actividad de Bot
// Framework con Authorization: HMAC <b64> —lo que un bot de Teams valida— y su
// respuesta vuelve traducida a texto de Rooms.
func TestSalienteTeamsFirmadoYTraducido(t *testing.T) {
	var actividad map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)

		// Lo que hace un receptor de Teams: base64-decodifica su clave de seguridad,
		// calcula el HMAC del cuerpo y compara con la cabecera.
		key, err := base64.StdEncoding.DecodeString(testSecret)
		if err != nil {
			t.Fatalf("el secreto debe ser base64 para que Teams pueda decodificarlo: %v", err)
		}
		mac := hmac.New(sha256.New, key)
		mac.Write(raw)
		want := "HMAC " + base64.StdEncoding.EncodeToString(mac.Sum(nil))
		if got := r.Header.Get("Authorization"); got != want {
			t.Fatalf("Authorization = %q, el receptor calcula %q", got, want)
		}
		if err := json.Unmarshal(raw, &actividad); err != nil {
			t.Fatalf("el cuerpo debe ser la actividad JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"@type":"MessageCard","text":"desplegado a **prod**"}`))
	}))
	defer srv.Close()

	body, err := outgoingBody(compat.Teams, outgoingRequest{
		MessageID: "m-1", ChannelID: "c-1", UserID: "u-1", UserName: "Ana",
		Text: "deploy ya", Tenant: "acme", Timestamp: 1700000000,
	})
	if err != nil {
		t.Fatalf("outgoingBody: %v", err)
	}
	svc := &Service{outgoing: srv.Client()}
	w := domain.OutgoingWebhook{CallbackURL: srv.URL, Dialect: compat.Teams, SigningSecret: testSecret}
	text, err := svc.doOutgoing(context.Background(), w, body)
	if err != nil {
		t.Fatalf("doOutgoing: %v", err)
	}
	if text != "" {
		t.Fatalf("Teams response must not enter the conversation: %q", text)
	}
	// Los campos por los que se orienta un bot de Teams: sin ellos descarta el mensaje.
	if actividad["type"] != "message" || actividad["channelId"] != "msteams" {
		t.Fatalf("la actividad no tiene la forma que espera Teams: %v", actividad)
	}
	if actividad["text"] != "deploy ya" {
		t.Fatalf("texto = %v", actividad["text"])
	}
	if from, _ := actividad["from"].(map[string]any); from == nil || from["name"] != "Ana" {
		t.Fatalf("sin el autor, el bot no sabe a quién responde: %v", actividad["from"])
	}
}

// TestSecretoDeFirmaNoEsElToken: si se reusara el token, la firma no probaría nada —
// el token viaja EN CLARO en cada entrega, así que quien vea una puede forjar la
// siguiente. Son dos secretos distintos a propósito.
func TestSecretoDeFirmaNoEsElToken(t *testing.T) {
	tok, _, err := domain.NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken: %v", err)
	}
	sec, err := domain.NewSigningSecret()
	if err != nil {
		t.Fatalf("NewSigningSecret: %v", err)
	}
	if sec == tok {
		t.Fatal("el secreto de firma coincide con el token de verificación")
	}
	raw, err := base64.StdEncoding.DecodeString(sec)
	if err != nil {
		t.Fatalf("el secreto debe ser base64 estándar (Teams lo decodifica): %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("clave de %d bytes, quiero 32", len(raw))
	}
	if strings.ContainsAny(sec, "-_") {
		t.Fatal("base64url: Teams lo decodificaría mal")
	}
}
