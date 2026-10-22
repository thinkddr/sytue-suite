// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

// Un @usuario válido tiene que ser también una parte local de buzón válida: es
// la dirección de una cuenta personal. Lo que se prueba es esa frontera.
func TestValidUsername(t *testing.T) {
	casos := map[string]bool{
		"ana":                   true,
		"ana.perez":             true,
		"ana-perez":             true,
		"ana2026":               true,
		"an":                    false, // demasiado corto
		"ana perez":             false, // espacio
		"ana..perez":            false, // separador repetido
		".ana":                  false, // separador en el extremo
		"ana-":                  false,
		"ana_perez":             false, // el guion bajo no es de las direcciones de la casa
		"añá":                   false, // lo no ASCII no se teclea igual dos veces
		"Ana":                   false, // sin normalizar
		"@ana":                  false,
		strings.Repeat("a", 65): false, // más que una parte local
	}
	for u, quiere := range casos {
		if got := ValidUsername(u); got != quiere {
			t.Errorf("ValidUsername(%q) = %v, quiere %v", u, got, quiere)
		}
	}
}

func TestNormalizeUsername(t *testing.T) {
	for entrada, quiere := range map[string]string{
		" @Ana.Perez ": "ana.perez",
		"ANA":          "ana",
		"@@ana":        "@ana", // solo se quita UNA arroba: lo demás lo rechaza ValidUsername
		"ana perez":    "ana perez",
	} {
		if got := NormalizeUsername(entrada); got != quiere {
			t.Errorf("NormalizeUsername(%q) = %q, quiere %q", entrada, got, quiere)
		}
	}
}

// @soporte o @sytue-id serían soporte@sytue.eu: la misma dirección que ya está
// prohibida regalar.
func TestUsernameReservado(t *testing.T) {
	for _, u := range []string{"soporte", "admin", "no-reply", "sytue.id", "info2"} {
		if !UsernameReservado(u) {
			t.Errorf("%q debería estar reservado", u)
		}
	}
	if UsernameReservado("ana.perez") {
		t.Error("ana.perez no está reservado")
	}
}

func TestUsernameDeLogin(t *testing.T) {
	casos := []struct {
		entrada, quiere string
		ok              bool
	}{
		{"@ana", "ana", true},
		{"Ana", "ana", true},
		{"ana@sytue.eu", "", false},
		{"ana@acme.com", "", false},
		{"", "", false},
	}
	for _, c := range casos {
		got, ok := UsernameDeLogin(c.entrada)
		if got != c.quiere || ok != c.ok {
			t.Errorf("UsernameDeLogin(%q) = (%q, %v), quiere (%q, %v)", c.entrada, got, ok, c.quiere, c.ok)
		}
	}
}

func TestUsernameCandidate(t *testing.T) {
	if got := UsernameCandidate("ana.perez", 0); got != "ana.perez" {
		t.Errorf("n=0: %q", got)
	}
	if got := UsernameCandidate("ana.perez", 1); got != "ana.perez2" {
		t.Errorf("n=1: %q", got)
	}
	largo := strings.Repeat("a", 64)
	if got := UsernameCandidate(largo, 9); len(got) > 64 || !ValidUsername(got) {
		t.Errorf("el sufijo no puede pasarse de 64: %q (%d)", got, len(got))
	}
}
