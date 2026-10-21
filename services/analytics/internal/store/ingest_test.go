// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/analytics/internal/domain"
)

// La fila que de verdad se persiste son los argumentos de este INSERT. Aquí se
// comprueba lo que el ADR promete: ni la IP ni el user-agent crudos aparecen en
// ella — no porque se filtren, sino porque no hay columna donde ponerlos.
func TestBuildInsertNoLlevaIPNiUserAgent(t *testing.T) {
	t.Parallel()
	const (
		ip = "203.0.113.77"
		ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36"
	)
	ev := domain.Event{
		Tenant:  "acme",
		SiteID:  "11111111-1111-1111-1111-111111111111",
		TS:      time.Now().UTC(),
		Visitor: domain.Visitor([]byte(strings.Repeat("k", 32)), "site", ip, ua),
		Name:    domain.EventPageview,
		Path:    "/precios",
		Country: "ES",
		Browser: "Chrome", OS: "macOS", Device: "desktop",
	}
	sql, args := buildInsert([]domain.Event{ev, ev})

	if strings.Contains(sql, ip) || strings.Contains(sql, ua) {
		t.Fatal("la sentencia SQL contiene la IP o el user-agent")
	}
	if got := len(args); got != 2*eventFields {
		t.Fatalf("se esperaban %d argumentos, hubo %d", 2*eventFields, got)
	}
	for i, a := range args {
		s := fmt.Sprint(a)
		if strings.Contains(s, ip) {
			t.Fatalf("el argumento %d contiene la IP: %q", i, s)
		}
		if strings.Contains(s, "Mozilla") || strings.Contains(s, "AppleWebKit") {
			t.Fatalf("el argumento %d contiene el user-agent crudo: %q", i, s)
		}
	}
	// Placeholders correlativos: si el orden se rompiera, PostgreSQL escribiría
	// cada valor en la columna de al lado.
	if !strings.Contains(sql, "$1,") || !strings.Contains(sql, fmt.Sprintf("$%d)", 2*eventFields)) {
		t.Fatalf("placeholders mal numerados: %s", sql)
	}
	// Sin COPY: bajo FORCE RLS PostgreSQL lo rechaza (ADR 0026 §3).
	if strings.Contains(strings.ToUpper(sql), "COPY ") {
		t.Fatal("el firehose no puede usar COPY … FROM bajo FORCE RLS")
	}
}

func TestNullableConvierteVacioEnNull(t *testing.T) {
	t.Parallel()
	if nullable("") != nil {
		t.Fatal("la cadena vacía debe viajar como NULL")
	}
	if nullable("x") != any("x") {
		t.Fatal("un valor no vacío debe viajar tal cual")
	}
}

// La cola no bloquea nunca: el colector es público y retener peticiones sería
// peor que perder muestras.
func TestWriterDescartaCuandoLaColaEstaLlena(t *testing.T) {
	t.Parallel()
	w := NewWriter(nil, 2, 2, time.Second)
	if !w.Enqueue(domain.Event{}) {
		t.Fatal("el primer evento debe caber")
	}
	if !w.Enqueue(domain.Event{}) {
		t.Fatal("el segundo evento debe caber")
	}
	if w.Enqueue(domain.Event{}) {
		t.Fatal("con la cola llena el evento debe descartarse, no bloquear")
	}
	if _, dropped := w.Stats(); dropped != 1 {
		t.Fatalf("se esperaba 1 descartado, hubo %d", dropped)
	}
}
