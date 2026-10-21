// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

func TestValidSharePermission(t *testing.T) {
	for _, p := range []SharePermission{PermReader, PermWriter} {
		if !ValidSharePermission(p) {
			t.Errorf("ValidSharePermission(%q) = false, quiero true", p)
		}
	}
	for _, p := range []SharePermission{"", "admin", "owner", "READER", "read"} {
		if ValidSharePermission(p) {
			t.Errorf("ValidSharePermission(%q) = true, quiero false", p)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	//nolint:gocritic // los espacios de la clave son el caso de prueba (NormalizeEmail debe recortarlos).
	cases := map[string]string{
		"  Alice@Example.COM ": "alice@example.com",
		"BOB@sytue.eu":         "bob@sytue.eu",
		"":                     "",
	}
	for in, want := range cases {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, quiero %q", in, got, want)
		}
	}
}

func TestValidEmail(t *testing.T) {
	ok := []string{"a@b.co", "alice.smith@sytue.eu", "x+tag@sub.example.com"}
	for _, e := range ok {
		if !ValidEmail(e) {
			t.Errorf("ValidEmail(%q) = false, quiero true", e)
		}
	}
	bad := []string{"", "noat", "@nolocal.com", "a@nodot", "a@b", "two@@at.com", "has space@x.com", "trailing@", "a@b\n.com"}
	for _, e := range bad {
		if ValidEmail(e) {
			t.Errorf("ValidEmail(%q) = true, quiero false", e)
		}
	}
}
