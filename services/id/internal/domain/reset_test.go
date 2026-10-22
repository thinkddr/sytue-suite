// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

var resetKey = []byte("clave-hmac-de-reseteo-de-32-bytes!!")

func TestResetToken_ValidSignature(t *testing.T) {
	token, hash, err := NewResetToken(resetKey)
	if err != nil {
		t.Fatalf("NewResetToken: %v", err)
	}
	if !VerifyResetTokenSig(resetKey, token) {
		t.Fatal("token recién emitido: la firma debería verificar")
	}
	// El hash persistido es el SHA-256 del token en claro, y el token NO aparece en él.
	if hash == "" || hash == token || HashToken(token) != hash {
		t.Fatalf("hash inconsistente: hash=%q token=%q", hash, token)
	}
	if strings.Contains(hash, token) {
		t.Fatal("el token en claro no debe derivarse del hash almacenado")
	}
}

func TestResetToken_TamperedRejected(t *testing.T) {
	token, _, err := NewResetToken(resetKey)
	if err != nil {
		t.Fatal(err)
	}
	// Alterar el PENÚLTIMO carácter de la firma (base64url) invalida el HMAC de
	// forma determinista: el último carácter arrastra bits de relleno (no
	// significativos), así que alterarlo solo cambia el HMAC decodificado ~94%
	// de las veces (flake). El penúltimo tiene sus 6 bits significativos.
	tampered := token[:len(token)-2] + flip(token[len(token)-2]) + token[len(token)-1:]
	if VerifyResetTokenSig(resetKey, tampered) {
		t.Fatal("token manipulado: la firma NO debería verificar")
	}
	// Alterar la parte aleatoria también rompe la firma.
	tampered2 := flip(token[0]) + token[1:]
	if VerifyResetTokenSig(resetKey, tampered2) {
		t.Fatal("parte aleatoria manipulada: la firma NO debería verificar")
	}
}

func TestResetToken_WrongKeyRejected(t *testing.T) {
	token, _, err := NewResetToken(resetKey)
	if err != nil {
		t.Fatal(err)
	}
	if VerifyResetTokenSig([]byte("otra-clave-hmac-distinta-de-32-b!!!"), token) {
		t.Fatal("token forjado con otra clave: NO debería verificar")
	}
}

func TestResetToken_MalformedRejected(t *testing.T) {
	for _, bad := range []string{"", ".", "abc", "abc.", ".abc", "sinpunto", "no-b64!.no-b64!"} {
		if VerifyResetTokenSig(resetKey, bad) {
			t.Errorf("token malformado %q: NO debería verificar", bad)
		}
	}
}

// flip devuelve un carácter distinto del dado (para manipular el token).
func flip(c byte) string {
	if c == 'A' {
		return "B"
	}
	return "A"
}
