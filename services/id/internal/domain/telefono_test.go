// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

func TestNormalizePhone(t *testing.T) {
	casos := []struct {
		in, want string
		ok       bool
	}{
		{"+34 600 123 456", "+34600123456", true},
		{"0034 600-12-34-56", "+34600123456", true},
		{" +49 (0151) 2345.6789 ", "+49015123456789", true},
		{"+353 851234567", "+353851234567", true},
		// Sin prefijo de país no se adivina.
		{"600 123 456", "600123456", false},
		{"+0 600 123 456", "+0600123456", false},
		{"+34 600", "+34600", false},
		{"+34 600 123 456 789 012", "+34600123456789012", false},
		{"ana@sytue.eu", "ana@sytue.eu", false},
		{"@ana", "@ana", false},
		{"", "", false},
	}
	for _, c := range casos {
		got, ok := NormalizePhone(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("NormalizePhone(%q) = %q, %v; quiero %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestNormalizeLogin(t *testing.T) {
	casos := map[string]string{
		"+34 600 123 456":           "+34600123456",
		"Ana@Sytue.eu":              "ana@sytue.eu",
		"ana@sytue.eu ana@sytue.eu": "ana@sytue.eu",
		"@Ana":                      "@ana",
	}
	for in, want := range casos {
		if got := NormalizeLogin(in); got != want {
			t.Errorf("NormalizeLogin(%q) = %q; quiero %q", in, got, want)
		}
	}
}

func TestNewSignupCode(t *testing.T) {
	vistos := map[string]bool{}
	for range 50 {
		c, err := NewSignupCode()
		if err != nil || len(c) != 6 || c[0] < '0' || c[0] > '9' {
			t.Fatalf("código %q (%v): quiero seis dígitos", c, err)
		}
		vistos[c] = true
	}
	if len(vistos) < 45 {
		t.Fatalf("solo %d códigos distintos de 50: no parece aleatorio", len(vistos))
	}
	if HashSignupCode("a", "123456") == HashSignupCode("b", "123456") {
		t.Fatal("el mismo código en dos altas no puede dar el mismo hash")
	}
}
