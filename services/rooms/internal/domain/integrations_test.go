// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestHashTokenDeterministicAndOpaque(t *testing.T) {
	tok := "s3cr3t-token-value"
	h1 := HashToken(tok)
	h2 := HashToken(tok)
	if h1 != h2 {
		t.Fatalf("HashToken no determinista: %q != %q", h1, h2)
	}
	if h1 == tok {
		t.Fatal("el hash NO puede igualar el token en claro")
	}
	if len(h1) != 64 { // SHA-256 hex
		t.Fatalf("hash con longitud inesperada %d, quiero 64", len(h1))
	}
	if HashToken("otro") == h1 {
		t.Fatal("tokens distintos deben dar hashes distintos")
	}
}

func TestNewTokensUniqueAndHashed(t *testing.T) {
	plain1, hash1, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken: %v", err)
	}
	plain2, _, _ := NewOpaqueToken()
	if plain1 == plain2 {
		t.Fatal("dos tokens opacos no deben coincidir")
	}
	if hash1 != HashToken(plain1) {
		t.Fatal("el hash devuelto debe ser HashToken(plain)")
	}
	if strings.Contains(hash1, plain1) {
		t.Fatal("el hash no debe contener el token en claro")
	}

	botPlain, botHash, err := NewBotToken()
	if err != nil {
		t.Fatalf("NewBotToken: %v", err)
	}
	if !strings.HasPrefix(botPlain, BotTokenPrefix) {
		t.Fatalf("el token de bot debe llevar el prefijo %q: %q", BotTokenPrefix, botPlain)
	}
	if botHash != HashToken(botPlain) {
		t.Fatal("hash de bot debe ser HashToken(plain)")
	}
}

func TestIsBotAuthor(t *testing.T) {
	id := uuid.New()
	if !IsBotAuthor(HookAuthorSub(id)) {
		t.Fatal("hook:<id> debe marcarse como integración")
	}
	if !IsBotAuthor(BotAuthorSub(id)) {
		t.Fatal("bot:<id> debe marcarse como integración")
	}
	// Un sub de usuario real (UUID/opaque) NO debe marcarse.
	if IsBotAuthor(id.String()) || IsBotAuthor("auth0|abc123") {
		t.Fatal("un sub de usuario no debe marcarse como integración")
	}
}

func TestValidCallbackURL(t *testing.T) {
	ok := []string{"https://example.com/hook", "http://ci.example.org:8080/x?y=1"}
	for _, u := range ok {
		if !ValidCallbackURL(u) {
			t.Fatalf("ValidCallbackURL(%q) debería ser true", u)
		}
	}
	bad := []string{"", "ftp://example.com", "file:///etc/passwd", "not-a-url", "https://", "javascript:alert(1)"}
	for _, u := range bad {
		if ValidCallbackURL(u) {
			t.Fatalf("ValidCallbackURL(%q) debería ser false", u)
		}
	}
}

func TestNormalizeTriggerWords(t *testing.T) {
	got := NormalizeTriggerWords([]string{" Deploy ", "deploy", "", "STATUS", "status"})
	want := []string{"deploy", "status"}
	if len(got) != len(want) {
		t.Fatalf("NormalizeTriggerWords dedupe/lower falló: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestMatchesOutgoing(t *testing.T) {
	chA := uuid.New()
	chB := uuid.New()

	// Ligado a un canal, sin palabras: dispara con cualquier mensaje del canal, no de otro.
	boundAny := OutgoingWebhook{ChannelID: &chA}
	if !MatchesOutgoing(chA, "hola mundo", boundAny) {
		t.Fatal("saliente ligado sin palabras debe disparar en su canal")
	}
	if MatchesOutgoing(chB, "hola", boundAny) {
		t.Fatal("no debe disparar en otro canal")
	}

	// Con palabra disparadora: dispara si el cuerpo EMPIEZA por ella (case-insensitive).
	words := OutgoingWebhook{ChannelID: &chA, TriggerWords: []string{"deploy"}}
	if !MatchesOutgoing(chA, "Deploy staging now", words) {
		t.Fatal("debe disparar cuando el cuerpo empieza por la palabra")
	}
	if MatchesOutgoing(chA, "please deploy", words) {
		t.Fatal("no debe disparar si la palabra no está al inicio")
	}

	// Sin canal ni palabras: nunca dispara (la validación al crear lo impide, pero el
	// matcher es defensivo).
	if MatchesOutgoing(chA, "x", OutgoingWebhook{}) {
		t.Fatal("un saliente sin canal ni palabras no debe disparar")
	}
}
