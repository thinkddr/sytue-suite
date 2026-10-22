// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
)

// testCipher devuelve un cifrador AES-256 con una KEK fija de pruebas. Fija y no
// aleatoria a propósito: si un test falla, el valor sellado que quede en la base
// del contenedor se puede abrir a mano para ver qué se guardó.
//
// No es un secreto: solo existe dentro del test, contra una base efímera.
func testCipher(t *testing.T) *keycrypto.Cipher {
	t.Helper()
	c, err := keycrypto.New([]byte("clave-de-pruebas-de-32-bytes...."))
	if err != nil {
		t.Fatalf("keycrypto de pruebas: %v", err)
	}
	return c
}
