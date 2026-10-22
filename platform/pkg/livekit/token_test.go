// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package livekit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const (
	testKey    = "APIabc123"
	testSecret = "super-secret-livekit-key-material"
)

// decodeJWTClaims decodifica (sin verificar) el payload de un JWT para inspección.
func decodeJWTClaims(t *testing.T, tok string) claims {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT malformado: %d partes", len(parts))
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	var c claims
	if err := json.Unmarshal(pb, &c); err != nil {
		t.Fatalf("claims: %v", err)
	}
	return c
}

func TestAccessTokenClaims(t *testing.T) {
	s := NewSigner(testKey, testSecret)
	pub, sub := true, true
	tok, err := s.AccessToken(AccessTokenOptions{
		Identity: "user-1",
		Name:     "Alice",
		Room:     "room-x",
		Grant:    VideoGrant{RoomAdmin: true, CanPublish: &pub, CanSubscribe: &sub},
		TTL:      time.Hour,
	})
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	c := decodeJWTClaims(t, tok)
	if c.Issuer != testKey {
		t.Errorf("iss = %q, want %q", c.Issuer, testKey)
	}
	if c.Subject != "user-1" || c.Name != "Alice" {
		t.Errorf("sub/name inesperados: %+v", c)
	}
	if c.Video.Room != "room-x" || !c.Video.RoomJoin || !c.Video.RoomAdmin {
		t.Errorf("grants de vídeo inesperados: %+v", c.Video)
	}
	if c.ExpiresAt <= time.Now().Unix() {
		t.Errorf("exp no está en el futuro: %d", c.ExpiresAt)
	}
	// El JWT debe verificar con el secreto correcto.
	if !verifyJWTSig(testSecret, tok) {
		t.Error("la firma no verifica con el secreto correcto")
	}
	if verifyJWTSig("otro-secreto", tok) {
		t.Error("la firma verifica con un secreto incorrecto")
	}
}

func TestAccessTokenRequiresRoomAndIdentity(t *testing.T) {
	s := NewSigner(testKey, testSecret)
	if _, err := s.AccessToken(AccessTokenOptions{Room: "r"}); err == nil {
		t.Error("falta identity y no dio error")
	}
	if _, err := s.AccessToken(AccessTokenOptions{Identity: "u"}); err == nil {
		t.Error("falta room y no dio error")
	}
}

// verifyJWTSig recomputa la firma HS256 y la compara.
func verifyJWTSig(secret, tok string) bool {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(parts[2]))
}

// signWebhook produce un JWT de webhook válido (como haría el servidor LiveKit):
// claim sha256 = hash base64-std del cuerpo, firmado HS256 con el api_secret.
func signWebhook(t *testing.T, secret string, body []byte, exp int64) string {
	t.Helper()
	sum := sha256.Sum256(body)
	c := claims{
		ExpiresAt: exp,
		SHA256:    base64.StdEncoding.EncodeToString(sum[:]),
	}
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	pb, _ := json.Marshal(c)
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(pb)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhookValid(t *testing.T) {
	s := NewSigner(testKey, testSecret)
	body := []byte(`{"event":"room_started"}`)
	tok := signWebhook(t, testSecret, body, time.Now().Add(time.Minute).Unix())
	if err := s.verifyWebhook(tok, body); err != nil {
		t.Fatalf("webhook válido rechazado: %v", err)
	}
}

func TestVerifyWebhookRejections(t *testing.T) {
	s := NewSigner(testKey, testSecret)
	body := []byte(`{"event":"room_started"}`)
	future := time.Now().Add(time.Minute).Unix()

	// Firmado con secreto incorrecto.
	if err := s.verifyWebhook(signWebhook(t, "mal", body, future), body); err == nil {
		t.Error("firma incorrecta aceptada")
	}
	// Cuerpo distinto al firmado (hash no coincide).
	tok := signWebhook(t, testSecret, body, future)
	if err := s.verifyWebhook(tok, []byte(`{"event":"tampered"}`)); err == nil {
		t.Error("cuerpo manipulado aceptado")
	}
	// Token expirado.
	expired := signWebhook(t, testSecret, body, time.Now().Add(-time.Minute).Unix())
	if err := s.verifyWebhook(expired, body); err == nil {
		t.Error("token expirado aceptado")
	}
	// Token malformado.
	if err := s.verifyWebhook("no.es.un.jwt.valido", body); err == nil {
		t.Error("token malformado aceptado")
	}
}

func TestParseWebhookEndToEnd(t *testing.T) {
	s := NewSigner(testKey, testSecret)
	body := []byte(`{"event":"participant_joined","room":{"name":"r1","sid":"RM_x"},"participant":{"identity":"u1","sid":"PA_x"}}`)
	tok := signWebhook(t, testSecret, body, time.Now().Add(time.Minute).Unix())
	ev, err := s.ParseWebhook(tok, body)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if ev.Event != EventParticipantJoined || ev.Room == nil || ev.Room.Name != "r1" ||
		ev.Participant == nil || ev.Participant.Identity != "u1" {
		t.Fatalf("evento decodificado inesperado: %+v", ev)
	}

	// Sin cabecera Authorization.
	if _, err := s.ParseWebhook("", body); err == nil {
		t.Error("webhook sin auth aceptado")
	}
}

// TestParseWebhookProtojsonNumbers: LiveKit serializa con protojson, y el estándar de
// protobuf JSON manda los int64 ENTRE COMILLAS para no perder precisión. El cuerpo de
// este test es el que envía LiveKit v1.13 de verdad, capturado en dev.
//
// Con los campos declarados como `int64` a secas, TODO webhook se rechazaba con
// «cannot unmarshal string into int64» y el servicio respondía 401. No se detectó antes
// porque el SFU no tenía configurado a quién avisar, así que nunca llegó ninguno: dos
// fallos que se tapaban el uno al otro.
func TestParseWebhookProtojsonNumbers(t *testing.T) {
	s := NewSigner(testKey, testSecret)
	body := []byte(`{"event":"room_finished","id":"EV_x","createdAt":"1785238899",` +
		`"room":{"sid":"RM_x","name":"r1","creationTime":"1785238899","numParticipants":0},` +
		`"participant":{"identity":"u1","sid":"PA_x","joinedAt":"1785238800"}}`)
	tok := signWebhook(t, testSecret, body, time.Now().Add(time.Minute).Unix())

	ev, err := s.ParseWebhook(tok, body)
	if err != nil {
		t.Fatalf("ParseWebhook con enteros como cadena: %v", err)
	}
	if ev.CreatedAt != 1785238899 {
		t.Errorf("createdAt = %d, quiero 1785238899", ev.CreatedAt)
	}
	if ev.Room == nil || ev.Room.CreationTime != 1785238899 {
		t.Errorf("room.creationTime no se decodificó: %+v", ev.Room)
	}
	if ev.Participant == nil || ev.Participant.JoinedAt != 1785238800 {
		t.Errorf("participant.joinedAt no se decodificó: %+v", ev.Participant)
	}

	// La forma numérica (la que usan otros emisores y los tests antiguos) sigue valiendo.
	numeric := []byte(`{"event":"room_finished","createdAt":1785238899,"room":{"name":"r1","creationTime":1785238899}}`)
	ev2, err := s.ParseWebhook(signWebhook(t, testSecret, numeric, time.Now().Add(time.Minute).Unix()), numeric)
	if err != nil {
		t.Fatalf("ParseWebhook con enteros numéricos: %v", err)
	}
	if ev2.CreatedAt != 1785238899 || ev2.Room.CreationTime != 1785238899 {
		t.Errorf("la forma numérica dejó de funcionar: %+v", ev2)
	}
}
