// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests unitarios de la lógica pura de dominio de Sytue Mail (sin BD/red).
package domain

import "testing"

func TestParseAddress(t *testing.T) {
	cases := []struct {
		in         string
		wantLocal  string
		wantDomain string
		wantErr    bool
	}{
		{"ada@sytue.eu", "ada", "sytue.eu", false},
		{"<Ada@Sytue.EU>", "Ada", "sytue.eu", false}, // local es case-sensitive; domain se normaliza
		{"  bob@example.com  ", "bob", "example.com", false},
		{"no-at-sign", "", "", true},
		{"@example.com", "", "", true},
		{"user@", "", "", true},
		{"user@localhost", "", "", true}, // sin punto en el dominio
		{"a b@example.com", "", "", true},
		{"user@exa mple.com", "", "", true},
	}
	for _, c := range cases {
		addr, err := ParseAddress(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseAddress(%q): esperaba error, got %v", c.in, addr)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseAddress(%q): error inesperado: %v", c.in, err)
			continue
		}
		if addr.Local != c.wantLocal || addr.Domain != c.wantDomain {
			t.Errorf("ParseAddress(%q) = %q@%q, quiero %q@%q", c.in, addr.Local, addr.Domain, c.wantLocal, c.wantDomain)
		}
	}
}

func TestAddressString(t *testing.T) {
	a := Address{Local: "ada", Domain: "sytue.eu"}
	if got := a.String(); got != "ada@sytue.eu" {
		t.Fatalf("String() = %q", got)
	}
}

func TestValidMailboxName(t *testing.T) {
	valid := []string{"Inbox", "Proyectos", "  Trabajo  ", "Año 2026"}
	for _, n := range valid {
		if !ValidMailboxName(n) {
			t.Errorf("ValidMailboxName(%q) = false, quiero true", n)
		}
	}
	invalid := []string{"", "a/b", "con\x00nul", "línea\nrota"}
	for _, n := range invalid {
		if ValidMailboxName(n) {
			t.Errorf("ValidMailboxName(%q) = true, quiero false", n)
		}
	}
}

func TestMessageHasFlag(t *testing.T) {
	m := Message{Flags: []string{FlagSeen, FlagFlagged}}
	if !m.HasFlag(FlagSeen) || !m.HasFlag("\\seen") { // case-insensitive
		t.Fatal("HasFlag debería casar \\Seen (case-insensitive)")
	}
	if m.HasFlag(FlagDeleted) {
		t.Fatal("HasFlag no debería casar \\Deleted")
	}
}

func TestDefaultMailboxesHasInboxFirst(t *testing.T) {
	if len(DefaultMailboxes) == 0 || DefaultMailboxes[0].Role != RoleInbox {
		t.Fatal("Inbox debe ser el primer buzón de sistema")
	}
	seen := map[SpecialUse]bool{}
	for _, mb := range DefaultMailboxes {
		if seen[mb.Role] {
			t.Fatalf("rol duplicado en DefaultMailboxes: %q", mb.Role)
		}
		seen[mb.Role] = true
	}
}
