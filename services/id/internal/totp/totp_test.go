// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package totp

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGenerateAndEncodeSecret(t *testing.T) {
	a, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if len(a) != SecretLen {
		t.Fatalf("longitud del secreto = %d, quiero %d", len(a), SecretLen)
	}
	b, _ := GenerateSecret()
	if bytes.Equal(a, b) {
		t.Fatal("dos secretos generados son iguales (¿no usa CSPRNG?)")
	}
	// EncodeSecret ⇄ DecodeSecret (base32 sin padding).
	dec, err := DecodeSecret(EncodeSecret(a))
	if err != nil || !bytes.Equal(dec, a) {
		t.Fatalf("round-trip encode/decode: %v (%x vs %x)", err, dec, a)
	}
	// El código generado con el secreto recién creado valida.
	if !Validate(a, Code(a, time.Now()), time.Now()) {
		t.Fatal("un código del secreto generado debe validar")
	}
}

func TestProvisioningURI(t *testing.T) {
	secret := []byte("12345678901234567890") // RFC 6238 App. B
	raw := ProvisioningURI("Sytue", "ada@sytue.eu", secret)
	if !strings.HasPrefix(raw, "otpauth://totp/") {
		t.Fatalf("prefijo inesperado: %q", raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("URI no parseable: %v", err)
	}
	if u.Scheme != "otpauth" || u.Host != "totp" {
		t.Fatalf("scheme/host = %q/%q", u.Scheme, u.Host)
	}
	q := u.Query()
	if q.Get("secret") != EncodeSecret(secret) {
		t.Fatalf("secret del URI = %q, quiero %q", q.Get("secret"), EncodeSecret(secret))
	}
	if q.Get("issuer") != "Sytue" || q.Get("algorithm") != "SHA1" ||
		q.Get("digits") != "6" || q.Get("period") != "30" {
		t.Fatalf("parámetros otpauth inesperados: %v", q)
	}
}

// RFC 6238 Appendix B usa un secreto ASCII "12345678901234567890".
var rfcSecret = []byte("12345678901234567890")

func TestCode_RoundTripValidate(t *testing.T) {
	now := time.Now()
	code := Code(rfcSecret, now)
	if len(code) != digits {
		t.Fatalf("longitud de código = %d, quería %d", len(code), digits)
	}
	if !Validate(rfcSecret, code, now) {
		t.Fatal("un código recién generado debe validar")
	}
}

func TestValidate_SkewWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	// Código del paso anterior debe validar dentro de la ventana ±1.
	prev := Code(rfcSecret, now.Add(-period*time.Second))
	if !Validate(rfcSecret, prev, now) {
		t.Fatal("el código del paso anterior debe validar (skew ±1)")
	}
	next := Code(rfcSecret, now.Add(period*time.Second))
	if !Validate(rfcSecret, next, now) {
		t.Fatal("el código del paso siguiente debe validar (skew ±1)")
	}
	// Dos pasos fuera NO debe validar.
	far := Code(rfcSecret, now.Add(3*period*time.Second))
	if Validate(rfcSecret, far, now) {
		t.Fatal("un código a 3 pasos no debe validar")
	}
}

func TestValidate_RejectsWrongAndMalformed(t *testing.T) {
	now := time.Now()
	if Validate(rfcSecret, "", now) {
		t.Fatal("código vacío no debe validar")
	}
	if Validate(rfcSecret, "12345", now) {
		t.Fatal("código de longitud incorrecta no debe validar")
	}
	if Validate(nil, Code(rfcSecret, now), now) {
		t.Fatal("secreto vacío no debe validar")
	}
	if Validate(rfcSecret, "abcdef", now) {
		t.Fatal("código no numérico no debe validar")
	}
}

func TestDecodeSecret(t *testing.T) {
	// base32 (sin padding) de "hello" = NBSWY3DP.
	b, err := DecodeSecret("NBSWY3DP")
	if err != nil {
		t.Fatalf("DecodeSecret: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("decodificado = %q, quería hello", string(b))
	}
	// Acepta minúsculas y espacios.
	if _, err := DecodeSecret("nbsw y3dp"); err != nil {
		t.Fatalf("DecodeSecret con minúsculas/espacios: %v", err)
	}
	if _, err := DecodeSecret("!!!"); err == nil {
		t.Fatal("base32 inválido debe fallar")
	}
}
