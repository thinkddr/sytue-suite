// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package keycrypto_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
)

func TestVerifyHMACSHA256ConocidoYManipulado(t *testing.T) {
	// Vector HMAC-SHA256 conocido: clave key y cuerpo inglés de 43 bytes.
	const sig = "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"
	body := []byte("The quick brown fox jumps over the lazy dog")
	if !keycrypto.VerifyHMACSHA256("key", body, sig) {
		t.Fatal("rechazó el vector conocido")
	}
	for _, tc := range []struct{ secret, body, sig string }{{"otra", string(body), sig}, {"key", string(body) + "!", sig}, {"key", string(body), sig[:62]}, {"", string(body), sig}, {"key", string(body), "zz" + sig[2:]}} {
		if keycrypto.VerifyHMACSHA256(tc.secret, []byte(tc.body), tc.sig) {
			t.Fatal("aceptó una firma manipulada")
		}
	}
}
