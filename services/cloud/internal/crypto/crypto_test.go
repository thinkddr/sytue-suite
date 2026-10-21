// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

// testKey genera una clave AES-256 codificada en base64 estándar.
func testKey(t *testing.T) string {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return base64.StdEncoding.EncodeToString(k)
}

func TestSealOpenRoundTrip(t *testing.T) {
	s, err := NewSealer(testKey(t))
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}
	cases := []string{
		"", "hola", "postgres://app:secret@db:5432/x?sslmode=require",
		"clave con espacios y símbolos: ñ€@#", string(bytes.Repeat([]byte("A"), 4096)),
	}
	for _, plain := range cases {
		blob, err := s.SealString(plain)
		if err != nil {
			t.Fatalf("Seal(%q): %v", plain, err)
		}
		got, err := s.OpenString(blob)
		if err != nil {
			t.Fatalf("Open(%q): %v", plain, err)
		}
		if got != plain {
			t.Fatalf("round-trip: got %q want %q", got, plain)
		}
	}
}

// TestSealNonDeterministic verifica que dos cifrados del mismo texto difieren
// (nonce aleatorio distinto): no debe filtrar igualdad de plaintext.
func TestSealNonDeterministic(t *testing.T) {
	s, _ := NewSealer(testKey(t))
	a, _ := s.SealString("mismo-secreto")
	b, _ := s.SealString("mismo-secreto")
	if bytes.Equal(a, b) {
		t.Fatal("dos cifrados idénticos: nonce no aleatorio")
	}
}

// TestOpenWrongKeyFails verifica que otra clave no descifra (autenticación GCM).
func TestOpenWrongKeyFails(t *testing.T) {
	s1, _ := NewSealer(testKey(t))
	s2, _ := NewSealer(testKey(t))
	blob, _ := s1.SealString("top-secret")
	if _, err := s2.Open(blob); !errors.Is(err, ErrCiphertextInvalid) {
		t.Fatalf("Open con clave distinta: got %v want ErrCiphertextInvalid", err)
	}
}

// TestOpenTamperedFails verifica que alterar el texto cifrado lo invalida.
func TestOpenTamperedFails(t *testing.T) {
	s, _ := NewSealer(testKey(t))
	blob, _ := s.SealString("integridad")
	blob[len(blob)-1] ^= 0xFF // corrompe el tag
	if _, err := s.Open(blob); !errors.Is(err, ErrCiphertextInvalid) {
		t.Fatalf("Open corrupto: got %v want ErrCiphertextInvalid", err)
	}
}

// TestOpenTooShortFails verifica el rechazo de blobs menores que el nonce.
func TestOpenTooShortFails(t *testing.T) {
	s, _ := NewSealer(testKey(t))
	if _, err := s.Open([]byte{1, 2, 3}); !errors.Is(err, ErrCiphertextInvalid) {
		t.Fatalf("Open corto: got %v want ErrCiphertextInvalid", err)
	}
}

func TestNewSealerKeyValidation(t *testing.T) {
	// 16 bytes (AES-128) debe rechazarse: exigimos AES-256.
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	if _, err := NewSealer(short); err == nil {
		t.Fatal("clave de 16 bytes aceptada, se esperaba error")
	}
	if _, err := NewSealer("no-es-base64-@@@!"); err == nil {
		t.Fatal("clave no base64 aceptada, se esperaba error")
	}
}

// TestKeyringRotation recorre la rotación completa de KEK en el cifrador: una
// clave suelta es la versión 1; al añadir la 2 pasa a ser la primaria y lo viejo
// SE SIGUE LEYENDO (ahí es donde una rotación mal hecha tira el servicio); lo
// re-sellado queda legible con la nueva a solas, que es lo que permite retirar la
// vieja.
func TestKeyringRotation(t *testing.T) {
	k1, k2 := testKey(t), testKey(t)

	v1, err := NewSealer(k1) // formato histórico: clave suelta ⇒ versión 1
	if err != nil {
		t.Fatalf("NewSealer(suelta): %v", err)
	}
	if v1.Version() != 1 {
		t.Fatalf("clave suelta: versión %d, se esperaba 1", v1.Version())
	}
	viejo, err := v1.SealString("secreto-de-antes")
	if err != nil {
		t.Fatalf("Seal con v1: %v", err)
	}

	anillo, err := NewSealer("1:" + k1 + ", 2:" + k2)
	if err != nil {
		t.Fatalf("NewSealer(anillo): %v", err)
	}
	if anillo.Version() != 2 {
		t.Fatalf("anillo: primaria %d, se esperaba la más alta (2)", anillo.Version())
	}
	if got, err := anillo.OpenString(viejo); err != nil || got != "secreto-de-antes" {
		t.Fatalf("el anillo no lee lo cifrado con la clave vieja: got %q err %v", got, err)
	}

	resellado, err := anillo.Reseal(viejo)
	if err != nil {
		t.Fatalf("Reseal: %v", err)
	}
	nuevo, err := NewSealer("2:" + k2) // clave vieja RETIRADA
	if err != nil {
		t.Fatalf("NewSealer(solo nueva): %v", err)
	}
	if got, err := nuevo.OpenString(resellado); err != nil || got != "secreto-de-antes" {
		t.Fatalf("tras re-sellar, la clave nueva no abre el secreto: got %q err %v", got, err)
	}
	// Y lo NO re-sellado deja de leerse: por eso el censo tiene que estar a cero
	// antes de retirar una clave.
	if _, err := nuevo.Open(viejo); !errors.Is(err, ErrCiphertextInvalid) {
		t.Fatalf("clave retirada abriendo un blob sin re-sellar: %v", err)
	}
}

func TestNewSealerRingValidation(t *testing.T) {
	k := testKey(t)
	for name, spec := range map[string]string{
		"versión repetida": "1:" + k + ",1:" + testKey(t),
		"versión cero":     "0:" + k,
		"versión negativa": "-1:" + k,
		"versión enorme":   "40000:" + k, // no cabe en smallint (key_version)
		"versión no num":   "v2:" + k,
		"vacía":            "",
	} {
		if _, err := NewSealer(spec); err == nil {
			t.Errorf("%s: aceptada, se esperaba error", name)
		}
	}
}

// TestNewSealerAcceptsBase64Variants verifica que se aceptan std y url-safe.
func TestNewSealerAcceptsBase64Variants(t *testing.T) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	for name, enc := range map[string]*base64.Encoding{
		"std":    base64.StdEncoding,
		"rawstd": base64.RawStdEncoding,
		"url":    base64.URLEncoding,
		"rawurl": base64.RawURLEncoding,
	} {
		if _, err := NewSealer(enc.EncodeToString(raw)); err != nil {
			t.Fatalf("variante %s rechazada: %v", name, err)
		}
	}
}
