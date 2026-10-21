// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidLabelName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"Trabajo", true},
		{"Trabajo/Proyectos", true}, // '/' admitido para jerarquía por nombre
		{"  con espacios  ", true},
		{"", false},
		{"   ", false},
		{"con\x00nul", false},
		{"con\ttab", false},
		{"con\nsalto", false},
		{strings.Repeat("x", 128), true},
		{strings.Repeat("x", 129), false},
	}
	for _, c := range cases {
		if got := ValidLabelName(c.name); got != c.want {
			t.Errorf("ValidLabelName(%q) = %v, quiero %v", c.name, got, c.want)
		}
	}
}

func TestValidLabelColor(t *testing.T) {
	if !ValidLabelColor("") {
		t.Error("'' (neutro) debe ser válido")
	}
	for _, c := range LabelColors {
		if !ValidLabelColor(c) {
			t.Errorf("color de paleta %q debe ser válido", c)
		}
	}
	for _, bad := range []string{"neon", "#ff0000", "red; color:evil", "SLATE"} {
		if ValidLabelColor(bad) {
			t.Errorf("color fuera de paleta %q no debe ser válido", bad)
		}
	}
}

// El keyword de una etiqueta es su id: estable y válido como atom IMAP / keyword
// JMAP (solo [0-9a-f-]), sin caracteres que rompan FLAGS/keywords.
func TestLabelKeyword(t *testing.T) {
	id := uuid.New()
	l := Label{ID: id}
	kw := l.Keyword()
	if kw != id.String() {
		t.Fatalf("Keyword() = %q, quiero %q", kw, id.String())
	}
	if strings.ContainsAny(kw, " ()[]{}%*\"\\") {
		t.Fatalf("keyword %q contiene caracteres no válidos para IMAP/JMAP", kw)
	}
}
