// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

func TestNewSignerEmptyKeyIsNoop(t *testing.T) {
	s, err := NewSigner("sytue.eu", "sel", "")
	if err != nil {
		t.Fatalf("NewSigner vacío: %v", err)
	}
	if s != nil {
		t.Fatal("clave vacía debe dar signer nil (firma desactivada en dev)")
	}
	// Sign sobre un signer nil devuelve el mensaje sin tocar.
	raw := []byte("From: a@sytue.eu\r\n\r\nhola\r\n")
	out, err := s.Sign(raw)
	if err != nil {
		t.Fatalf("Sign(nil): %v", err)
	}
	if !bytes.Equal(out, raw) {
		t.Fatal("Sign(nil) debe devolver el mensaje intacto")
	}
}

func TestNewSignerInvalidPEM(t *testing.T) {
	if _, err := NewSigner("sytue.eu", "sel", "no-es-pem"); err == nil {
		t.Fatal("esperaba error con PEM inválido")
	}
}

func TestSignAddsDKIMSignature(t *testing.T) {
	// Clave Ed25519 en PKCS#8 PEM.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generar clave: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	s, err := NewSigner("sytue.eu", "sytue", keyPEM)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	if s == nil {
		t.Fatal("signer nil con clave válida")
	}

	raw := []byte("From: ada@sytue.eu\r\nTo: bob@example.com\r\nSubject: hi\r\n" +
		"Date: Mon, 30 Jun 2026 10:00:00 +0000\r\nMessage-Id: <x@sytue.eu>\r\n\r\ncuerpo\r\n")
	out, err := s.Sign(raw)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !strings.Contains(string(out), "DKIM-Signature:") {
		t.Fatal("la salida debe incluir la cabecera DKIM-Signature")
	}
	if !strings.Contains(string(out), "d=sytue.eu") || !strings.Contains(string(out), "s=sytue") {
		t.Fatal("la firma debe declarar d= y s= correctos")
	}
}
