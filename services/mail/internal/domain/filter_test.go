// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestFilterValidate(t *testing.T) {
	box := uuid.New()
	cases := []struct {
		name string
		f    Filter
		ok   bool
	}{
		{"sin condición ni acción", Filter{}, false},
		{"condición sin acción", Filter{MatchFrom: "jefe@x"}, false},
		{"acción sin condición", Filter{MoveTo: &box}, false},
		{"condición from + mover", Filter{MatchFrom: "jefe@x", MoveTo: &box}, true},
		{"condición asunto + marcar leído", Filter{MatchSubject: "boletín", MarkRead: true}, true},
		{"solo espacios en la condición", Filter{MatchFrom: "  ", MarkRead: true}, false},
	}
	for _, c := range cases {
		if got := c.f.Validate() == nil; got != c.ok {
			t.Errorf("%s: Validate ok=%v, quiero %v", c.name, got, c.ok)
		}
	}
}

func TestFilterMatches(t *testing.T) {
	m := Message{FromAddr: "boletin@Marca.com", FromName: "Marca Diario", Subject: "Resumen semanal"}
	cases := []struct {
		name string
		f    Filter
		want bool
	}{
		{"from por dirección, case-insensitive", Filter{MatchFrom: "marca.com"}, true},
		{"from por nombre", Filter{MatchFrom: "diario"}, true},
		{"asunto subcadena", Filter{MatchSubject: "semanal"}, true},
		{"AND: ambas casan", Filter{MatchFrom: "marca", MatchSubject: "resumen"}, true},
		{"AND: una falla", Filter{MatchFrom: "marca", MatchSubject: "factura"}, false},
		{"from que no casa", Filter{MatchFrom: "amazon"}, false},
		{"sin condiciones no casa", Filter{}, false},
	}
	for _, c := range cases {
		if got := c.f.Matches(m); got != c.want {
			t.Errorf("%s: Matches=%v, quiero %v", c.name, got, c.want)
		}
	}
}

func TestApplyInboundFirstMatchWins(t *testing.T) {
	boxA, boxB := uuid.New(), uuid.New()
	filters := []Filter{
		{Enabled: true, MatchFrom: "nope", MoveTo: &boxA},                      // no casa
		{Enabled: true, MatchSubject: "oferta", MoveTo: &boxB, MarkRead: true}, // casa
		{Enabled: true, MatchSubject: "oferta", MarkFlagged: true},             // no debería aplicarse (ya ganó la anterior)
	}
	m := Message{FromAddr: "ventas@tienda.eu", Subject: "Gran oferta de verano"}
	dest := ApplyInbound(filters, &m)
	if dest == nil || *dest != boxB {
		t.Fatalf("destino = %v, quiero %v", dest, boxB)
	}
	if !m.HasFlag(FlagSeen) {
		t.Error("mark_read debe añadir \\Seen")
	}
	if m.HasFlag(FlagFlagged) {
		t.Error("la tercera regla no debe aplicarse (solo gana la primera que casa)")
	}
}

func TestApplyInboundSkipsDisabledAndNoMatch(t *testing.T) {
	box := uuid.New()
	filters := []Filter{
		{Enabled: false, MatchSubject: "oferta", MoveTo: &box}, // desactivada
		{Enabled: true, MatchSubject: "no-casa", MoveTo: &box},
	}
	m := Message{Subject: "Gran oferta"}
	if dest := ApplyInbound(filters, &m); dest != nil {
		t.Fatalf("ninguna regla activa casa: destino = %v, quiero nil", dest)
	}
	if len(m.Flags) != 0 {
		t.Errorf("flags = %v, quiero ninguno", m.Flags)
	}
}

func TestApplyInboundMarkOnlyKeepsMailbox(t *testing.T) {
	// Una regla que solo marca (sin mover) devuelve nil: el mensaje se queda donde
	// iba (Inbox), pero con el flag aplicado.
	filters := []Filter{{Enabled: true, MatchFrom: "jefe@empresa.eu", MarkFlagged: true}}
	m := Message{FromAddr: "jefe@empresa.eu", Subject: "urgente"}
	if dest := ApplyInbound(filters, &m); dest != nil {
		t.Fatalf("destino = %v, quiero nil (no mover)", dest)
	}
	if !m.HasFlag(FlagFlagged) {
		t.Error("mark_flagged debe añadir \\Flagged")
	}
}
