// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"strings"
	"testing"
)

func TestCabeceraAuthenticationResults(t *testing.T) {
	got := CabeceraAuthenticationResults("mx.sytue.eu", ResultPass, ResultFail, "", "Ejemplo.org", "ejemplo.org")
	want := "Authentication-Results: mx.sytue.eu;\r\n\tspf=pass smtp.mailfrom=ejemplo.org;\r\n\tdkim=fail;\r\n\tdmarc=none header.from=ejemplo.org\r\n"
	if got != want {
		t.Fatalf("cabecera:\n%q\nquería\n%q", got, want)
	}
}

func TestCabeceraAuthenticationResultsNoDejaInyectar(t *testing.T) {
	got := CabeceraAuthenticationResults("", ResultPass, ResultPass, ResultPass, "malo.org\r\nX-Falsa: 1; dkim=pass", "")
	if strings.Count(got, "\r\n") != 4 || strings.Contains(got, "X-Falsa:") {
		t.Fatalf("el dominio del remitente inyectó cabeceras: %q", got)
	}
	if !strings.HasPrefix(got, "Authentication-Results: sytue;") {
		t.Fatalf("sin authserv-id debe usar «sytue»: %q", got)
	}
}
