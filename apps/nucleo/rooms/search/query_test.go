// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package search

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	d := func(s string) *time.Time {
		tt, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("fecha de test inválida %q: %v", s, err)
		}
		return &tt
	}

	cases := []struct {
		name string
		raw  string
		want Query
	}{
		{"cada palabra es un término (AND)", "hola mundo", Query{Terms: []string{"hola", "mundo"}}},
		{"from + texto", "deploy from:ana", Query{Terms: []string{"deploy"}, From: "ana"}},
		{
			"todos los operadores + texto entremezclado",
			"before:2024-02-01 error in:general from:bob after:2024-01-01 timeout",
			Query{
				Terms: []string{"error", "timeout"}, From: "bob", In: "general",
				After: d("2024-01-01"), Before: d("2024-02-01"),
			},
		},
		{"clave insensible a mayúsculas", "FROM:ana IN:general", Query{From: "ana", In: "general"}},
		{"operador sin valor se ignora", "from: hola in:", Query{Terms: []string{"hola"}}},
		{"fecha inválida se ignora en silencio", "after:ayer before:2024-13-40 x", Query{Terms: []string{"x"}}},
		{"solo operadores, texto vacío", "from:ana in:random", Query{From: "ana", In: "random"}},
		{"url con ':' cae a texto libre", "mira http://sytue.eu/x", Query{Terms: []string{"mira", "http://sytue.eu/x"}}},
		{"vacío", "   ", Query{}},
		// Frase exacta: las comillas mantienen los espacios en UN solo término.
		{"frase exacta", `"informe de cierre"`, Query{Terms: []string{"informe de cierre"}}},
		{
			"frase exacta + término suelto",
			`urgente "informe de cierre"`,
			Query{Terms: []string{"urgente", "informe de cierre"}},
		},
		{"lo entrecomillado no es operador", `"in:general"`, Query{Terms: []string{"in:general"}}},
		// La almohadilla es como se nombra un canal en Slack/MM/Teams, y el valor se
		// compara contra `slug`/`name`, que no la llevan: sin recortarla, `in:#general`
		// no casa con nada y la búsqueda sale vacía sin decir por qué.
		{"in: con almohadilla", "in:#general", Query{In: "general"}},
		{"in: sin almohadilla sigue igual", "in:general", Query{In: "general"}},
		{"in: solo la almohadilla se ignora", "in:#", Query{}},
		{"comilla sin cerrar llega al final", `"sin cerrar`, Query{Terms: []string{"sin cerrar"}}},
		// Exclusión.
		{"exclusión de palabra", "plan -borrador", Query{Terms: []string{"plan"}, Not: []string{"borrador"}}},
		{"exclusión de frase", `plan -"copia de seguridad"`, Query{
			Terms: []string{"plan"}, Not: []string{"copia de seguridad"},
		}},
		{"guion suelto es texto", "a - b", Query{Terms: []string{"a", "-", "b"}}},
		// on: es exactamente el día (reusa la ventana after/before).
		{"on: acota un día", "on:2024-03-05", Query{After: d("2024-03-05"), Before: d("2024-03-06")}},
		{"on: inválido se ignora en silencio, como before:/after:", "on:mañana", Query{}},
		// has: / is:
		{"has:link", "has:link", Query{HasLink: true}},
		{"has:file y mayúsculas", "HAS:FILE", Query{HasFile: true}},
		{"has: desconocido cae a texto", "has:banana", Query{Terms: []string{"has:banana"}}},
		{"is:saved", "is:saved", Query{Saved: true}},
		{"is:pinned", "is:pinned", Query{Pinned: true}},
		{"is: desconocido cae a texto", "is:raro", Query{Terms: []string{"is:raro"}}},
		{
			"combinación completa",
			`presupuesto -viejo has:file is:pinned on:2024-03-05 "cierre anual"`,
			Query{
				Terms: []string{"presupuesto", "cierre anual"}, Not: []string{"viejo"},
				HasFile: true, Pinned: true, After: d("2024-03-05"), Before: d("2024-03-06"),
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.raw)
			if !slices.Equal(got.Terms, c.want.Terms) || !slices.Equal(got.Not, c.want.Not) {
				t.Fatalf("términos: got %q/%q want %q/%q", got.Terms, got.Not, c.want.Terms, c.want.Not)
			}
			if got.From != c.want.From || got.In != c.want.In {
				t.Fatalf("from/in: got %+v want %+v", got, c.want)
			}
			if !eqTime(got.After, c.want.After) || !eqTime(got.Before, c.want.Before) {
				t.Fatalf("fechas: got after=%v before=%v; want after=%v before=%v",
					got.After, got.Before, c.want.After, c.want.Before)
			}
			if got.HasLink != c.want.HasLink || got.HasFile != c.want.HasFile ||
				got.Saved != c.want.Saved || got.Pinned != c.want.Pinned {
				t.Fatalf("has:/is:: got %+v want %+v", got, c.want)
			}
		})
	}
}

// Tope de tokens: una consulta gigante no se traduce en cientos de ILIKE.
func TestParseQueryTopeDeTokens(t *testing.T) {
	got := Parse(strings.Repeat("palabra ", 500))
	if len(got.Terms) != maxSearchTokens {
		t.Fatalf("términos esperados %d, got %d", maxSearchTokens, len(got.Terms))
	}
}

func eqTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
