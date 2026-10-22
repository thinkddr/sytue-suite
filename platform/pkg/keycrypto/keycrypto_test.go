// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package keycrypto

import (
	"crypto/rand"
	"strings"
	"testing"
)

func newKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, KeyLen)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("aleatoriedad: %v", err)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := New(newKey(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const plain = "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n"
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, prefix) {
		t.Fatalf("el cifrado no lleva el prefijo de versión: %q", enc)
	}
	if strings.Contains(enc, "PRIVATE KEY") {
		t.Fatal("el texto cifrado filtra el plano en claro")
	}
	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("roundtrip mismatch: %q != %q", got, plain)
	}
}

func TestEncryptUsesRandomNonce(t *testing.T) {
	c, _ := New(newKey(t))
	a, _ := c.Encrypt("mismo")
	b, _ := c.Encrypt("mismo")
	if a == b {
		t.Fatal("dos cifrados del mismo texto son idénticos (¿nonce fijo?)")
	}
}

func TestDecryptTamperedFails(t *testing.T) {
	c, _ := New(newKey(t))
	enc, _ := c.Encrypt("datos")
	// Alterar un carácter del cuerpo base64 debe romper la autenticación GCM.
	tampered := enc[:len(enc)-2] + string([]byte{enc[len(enc)-2] ^ 0x01}) + enc[len(enc)-1:]
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("Decrypt aceptó un texto cifrado manipulado")
	}
}

func TestNilCipherIsIdentity(t *testing.T) {
	c, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil): %v", err)
	}
	if c.Enabled() {
		t.Fatal("un Cipher sin clave no debe estar habilitado")
	}
	enc, err := c.Encrypt("claro")
	if err != nil || enc != "claro" {
		t.Fatalf("sin clave Encrypt debe ser identidad, got %q, %v", enc, err)
	}
	// Un valor en claro heredado se lee tal cual incluso sin clave.
	got, err := c.Decrypt("claro")
	if err != nil || got != "claro" {
		t.Fatalf("Decrypt de valor en claro debe ser identidad, got %q, %v", got, err)
	}
}

func TestNilCipherRejectsCiphertext(t *testing.T) {
	// Sin clave, un valor cifrado (con prefijo) no puede descifrarse.
	var c *Cipher
	if _, err := c.Decrypt(prefix + "AAAA"); err == nil {
		t.Fatal("Decrypt sin clave aceptó un valor cifrado")
	}
}

func TestNewRejectsWrongKeyLength(t *testing.T) {
	if _, err := New([]byte("corta")); err == nil {
		t.Fatal("New aceptó una clave de longitud incorrecta")
	}
}

func TestLegacyPlaintextDecrypt(t *testing.T) {
	// Con clave configurada, un valor SIN prefijo se trata como valor en claro
	// heredado (migración sin downtime) y se devuelve tal cual.
	c, _ := New(newKey(t))
	const legacy = "-----BEGIN RSA PRIVATE KEY-----legacy-----END RSA PRIVATE KEY-----"
	got, err := c.Decrypt(legacy)
	if err != nil || got != legacy {
		t.Fatalf("valor en claro heredado no leído tal cual: %q, %v", got, err)
	}
}
