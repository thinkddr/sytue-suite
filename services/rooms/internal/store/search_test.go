// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Sin texto ni operadores no se añaden condiciones ni args (la consulta base manda
// sola: ni siquiera un `body ILIKE '%%'` inútil).
func TestSearchFiltersEmpty(t *testing.T) {
	var qb sqlArgs
	if got := searchFilters(&qb, domain.SearchQuery{}, "$1"); got != "" {
		t.Fatalf("sin operadores no debe añadir SQL: %q", got)
	}
	if len(qb.vals) != 0 {
		t.Fatalf("sin operadores no debe añadir args: %v", qb.vals)
	}
}

// Con todos los operadores: condiciones correctas, numeración de $ continuada tras
// los args base, y cada valor registrado una sola vez.
func TestSearchFiltersAssembly(t *testing.T) {
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	var qb sqlArgs
	qb.add("acme") // $1 tenant
	qb.add("sub")  // $2 subject (args base, como en SearchAll)
	conds := searchFilters(&qb, domain.SearchQuery{
		From: "ana", In: "general", After: &after, Before: &before,
	}, "$2")

	// Cada operador aporta su condición sobre la columna esperada.
	for _, want := range []string{
		"plegar(msg.author_name) LIKE", "msg.author_sub =",
		"plegar(c.slug) LIKE", "plegar(c.name) LIKE",
		"msg.created_at >=", "msg.created_at <",
	} {
		if !strings.Contains(conds, want) {
			t.Errorf("falta %q en %q", want, conds)
		}
	}

	// Numeración: from=$3 (patrón) + $4 (sub exacto), in=$5, after=$6, before=$7.
	for _, ph := range []string{"$3", "$4", "$5", "$6", "$7"} {
		if !strings.Contains(conds, ph) {
			t.Errorf("falta placeholder %s en %q", ph, conds)
		}
	}
	// 2 base + from (patrón + crudo) + in + after + before = 7 args.
	if len(qb.vals) != 7 {
		t.Fatalf("args esperados 7, got %d (%v)", len(qb.vals), qb.vals)
	}
	if qb.vals[2] != "%ana%" || qb.vals[3] != "ana" || qb.vals[4] != "%general%" ||
		qb.vals[5] != after || qb.vals[6] != before {
		t.Fatalf("args mal registrados: %v", qb.vals)
	}
}

// Solo una parte de los operadores: no cuela placeholders de los ausentes.
func TestSearchFiltersPartial(t *testing.T) {
	var qb sqlArgs
	conds := searchFilters(&qb, domain.SearchQuery{From: "bob"}, "$1")
	if !strings.Contains(conds, "author_sub") {
		t.Fatalf("from debe generar condición de autor: %q", conds)
	}
	if strings.Contains(conds, "created_at") || strings.Contains(conds, "c.slug") {
		t.Fatalf("no debe generar filtros de canal/fecha ausentes: %q", conds)
	}
	if len(qb.vals) != 2 {
		t.Fatalf("solo from → 2 args (patrón + crudo), got %v", qb.vals)
	}
}

// Texto: un LIKE plegado por término positivo (AND) y un NOT LIKE por excluido, cada
// uno con su propio arg ya escapado.
func TestSearchFiltersTextTerms(t *testing.T) {
	var qb sqlArgs
	conds := searchFilters(&qb, domain.SearchQuery{
		Terms: []string{"reunión", "viernes", "informe de cierre"},
		Not:   []string{"borrador"},
	}, "$1")
	if got := strings.Count(conds, "AND msg.body_plegado LIKE"); got != 3 {
		t.Fatalf("un LIKE por término positivo, got %d en %q", got, conds)
	}
	if got := strings.Count(conds, "AND msg.body_plegado NOT LIKE"); got != 1 {
		t.Fatalf("un NOT LIKE por término excluido, got %d en %q", got, conds)
	}
	want := []any{"%reunión%", "%viernes%", "%informe de cierre%", "%borrador%"}
	if len(qb.vals) != len(want) {
		t.Fatalf("args esperados %v, got %v", want, qb.vals)
	}
	for i, w := range want {
		if qb.vals[i] != w {
			t.Fatalf("arg %d: got %v want %v", i, qb.vals[i], w)
		}
	}
}

// has:/is: no aportan args (salvo el subject ya registrado) y sus condiciones son
// semi-joins por PK, no JOINs en el FROM (nada que multiplique filas).
func TestSearchFiltersHasIs(t *testing.T) {
	var qb sqlArgs
	conds := searchFilters(&qb, domain.SearchQuery{
		HasLink: true, HasFile: true, Saved: true, Pinned: true,
	}, "$2")
	for _, want := range []string{
		`msg.body ~* 'https?://'`, `msg.attachments <> '[]'::jsonb`,
		"EXISTS (SELECT 1 FROM message_saves", "ms.subject=$2",
		"EXISTS (SELECT 1 FROM message_pins",
	} {
		if !strings.Contains(conds, want) {
			t.Errorf("falta %q en %q", want, conds)
		}
	}
	if strings.Contains(conds, " JOIN ") {
		t.Errorf("has:/is: deben ser EXISTS, no JOIN: %q", conds)
	}
	if len(qb.vals) != 0 {
		t.Fatalf("has:/is: no deben añadir args nuevos: %v", qb.vals)
	}
}

// El comodín del usuario se escapa: «%» y «_» son literales y la propia barra de
// escape también. Es la mitad del arreglo (sin esto, buscar «%» devuelve TODO).
func TestLikePatternEscapa(t *testing.T) {
	cases := map[string]string{
		"50%":      `%50\%%`,
		"%":        `%\%%`,
		"a_b":      `%a\_b%`,
		`c:\ruta`:  `%c:\\ruta%`,
		"normal":   "%normal%",
		`100\%%__`: `%100\\\%\%\_\_%`,
	}
	for in, want := range cases {
		if got := likePattern(in); got != want {
			t.Errorf("likePattern(%q) = %q, quiero %q", in, got, want)
		}
	}
}
