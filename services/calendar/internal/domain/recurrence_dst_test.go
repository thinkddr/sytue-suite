// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests DETERMINISTAS de la expansión de recurrencias con zonas horarias reales
// (RFC 5545 DTSTART;TZID) y DST. Instantes fijados por constantes (nunca
// time.Now): una serie a hora local fija conserva su hora-de-pared a ambos lados
// de un cambio de hora, con lo que el instante UTC se desplaza ±1 h. tzdata la
// aporta la stdlib (time/tzdata embebido); estos tests fallan si falla la carga.
package domain

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v (¿tzdata embebida?)", name, err)
	}
	return loc
}

// wide es una ventana amplia que envuelve cualquier transición DST del año.
func wide(year int) (time.Time, time.Time) {
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
}

// TestExpandWeeklyMadridSpringForward: «09:00 cada lunes» en Europe/Madrid cruza
// el spring-forward (2025-03-30 02:00->03:00, +1->+2). La hora local sigue siendo
// 09:00; el instante UTC pasa de 08:00Z a 07:00Z.
func TestExpandWeeklyMadridSpringForward(t *testing.T) {
	mad := mustLoc(t, "Europe/Madrid")
	start := time.Date(2025, 3, 24, 9, 0, 0, 0, mad) // lunes 09:00 CET (+1)
	r := mustParse(t, "FREQ=WEEKLY;COUNT=3")
	from, to := wide(2025)
	insts := r.Expand(start, nil, nil, from, to)

	want := []time.Time{
		time.Date(2025, 3, 24, 8, 0, 0, 0, time.UTC), // CET  +1
		time.Date(2025, 3, 31, 7, 0, 0, 0, time.UTC), // CEST +2 (tras spring-forward)
		time.Date(2025, 4, 7, 7, 0, 0, 0, time.UTC),  // CEST +2
	}
	assertInstants(t, insts, want, mad, 9)
	if insts[0].UTC().Hour() == insts[1].UTC().Hour() {
		t.Errorf("el instante UTC no se desplazó con el DST: %s vs %s", insts[0].UTC(), insts[1].UTC())
	}
}

// TestExpandWeeklyMadridFallBack: la misma serie cruza el fall-back (2025-10-26
// 03:00->02:00, +2->+1). Hora local 09:00 constante; UTC pasa de 07:00Z a 08:00Z.
func TestExpandWeeklyMadridFallBack(t *testing.T) {
	mad := mustLoc(t, "Europe/Madrid")
	start := time.Date(2025, 10, 20, 9, 0, 0, 0, mad) // lunes 09:00 CEST (+2)
	r := mustParse(t, "FREQ=WEEKLY;COUNT=2")
	from, to := wide(2025)
	insts := r.Expand(start, nil, nil, from, to)

	want := []time.Time{
		time.Date(2025, 10, 20, 7, 0, 0, 0, time.UTC), // CEST +2
		time.Date(2025, 10, 27, 8, 0, 0, 0, time.UTC), // CET  +1 (tras fall-back)
	}
	assertInstants(t, insts, want, mad, 9)
}

// TestExpandWeeklyNewYorkSpringForward: «09:00 cada lunes» en America/New_York
// cruza su spring-forward (2025-03-09, -5->-4). UTC pasa de 14:00Z a 13:00Z.
func TestExpandWeeklyNewYorkSpringForward(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	start := time.Date(2025, 3, 3, 9, 0, 0, 0, ny) // lunes 09:00 EST (-5)
	r := mustParse(t, "FREQ=WEEKLY;COUNT=2")
	from, to := wide(2025)
	insts := r.Expand(start, nil, nil, from, to)

	want := []time.Time{
		time.Date(2025, 3, 3, 14, 0, 0, 0, time.UTC),  // EST -5
		time.Date(2025, 3, 10, 13, 0, 0, 0, time.UTC), // EDT -4 (tras spring-forward)
	}
	assertInstants(t, insts, want, ny, 9)
}

// TestExpandNonexistentLocalTime: hora local INEXISTENTE (spring-forward gap). En
// Europe/Madrid el 2025-03-30 las 02:30 no ocurren (el reloj salta 02:00->03:00).
// Política (delegada a time.Date de la stdlib, determinista): la hora-de-pared
// «rueda hacia delante» al nuevo offset -> 03:30 CEST (+2) = 01:30Z.
func TestExpandNonexistentLocalTime(t *testing.T) {
	mad := mustLoc(t, "Europe/Madrid")
	start := time.Date(2025, 3, 30, 2, 30, 0, 0, mad) // no existe: rueda a 03:30
	r := mustParse(t, "FREQ=DAILY;COUNT=1")
	from, to := wide(2025)
	insts := r.Expand(start, nil, nil, from, to)

	if len(insts) != 1 {
		t.Fatalf("got %d instancias, want 1", len(insts))
	}
	if want := time.Date(2025, 3, 30, 1, 30, 0, 0, time.UTC); !insts[0].Equal(want) {
		t.Errorf("gap: instante = %s, want %s (01:30Z)", insts[0].UTC(), want)
	}
	if wall := insts[0].In(mad); wall.Hour() != 3 || wall.Minute() != 30 {
		t.Errorf("gap: hora local = %02d:%02d, want 03:30 (rodada hacia delante)", wall.Hour(), wall.Minute())
	}
}

// TestExpandAmbiguousLocalTime: hora local AMBIGUA (fall-back overlap). En
// Europe/Madrid el 2025-10-26 las 02:30 ocurren DOS veces (03:00->02:00). Política
// (time.Date, determinista): elige la SEGUNDA ocurrencia, offset estándar +1 =
// 01:30Z.
func TestExpandAmbiguousLocalTime(t *testing.T) {
	mad := mustLoc(t, "Europe/Madrid")
	start := time.Date(2025, 10, 26, 2, 30, 0, 0, mad) // ambigua: se resuelve a +1
	r := mustParse(t, "FREQ=DAILY;COUNT=1")
	from, to := wide(2025)
	insts := r.Expand(start, nil, nil, from, to)

	if len(insts) != 1 {
		t.Fatalf("got %d instancias, want 1", len(insts))
	}
	if want := time.Date(2025, 10, 26, 1, 30, 0, 0, time.UTC); !insts[0].Equal(want) {
		t.Errorf("overlap: instante = %s, want %s (01:30Z)", insts[0].UTC(), want)
	}
	wall := insts[0].In(mad)
	if _, off := wall.Zone(); off != 3600 {
		t.Errorf("overlap: offset = %ds, want 3600 (+1, hora estándar)", off)
	}
}

// TestExpandAllDayFloating: un evento de día completo (all-day) es FLOTANTE: cae
// el mismo día natural en cualquier zona y NO se desplaza por DST. EventLocation
// devuelve UTC para all-day; expandido en UTC, cada instancia es medianoche del
// mismo día aunque la serie cruce un cambio de hora.
func TestExpandAllDayFloating(t *testing.T) {
	// DTStart de un all-day se persiste a medianoche UTC.
	start := time.Date(2025, 3, 28, 0, 0, 0, 0, time.UTC)
	loc := EventLocation("Europe/Madrid", true) // all-day => UTC (flotante)
	if loc != time.UTC {
		t.Fatalf("EventLocation(all-day) = %v, want UTC", loc)
	}
	r := mustParse(t, "FREQ=DAILY;COUNT=4") // cruza el spring-forward del 03-30
	from, to := wide(2025)
	insts := r.Expand(start.In(loc), nil, nil, from, to)

	want := []time.Time{
		time.Date(2025, 3, 28, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 3, 29, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 3, 30, 0, 0, 0, 0, time.UTC), // día del cambio: sigue a medianoche
		time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
	}
	if len(insts) != len(want) {
		t.Fatalf("got %d instancias, want %d", len(insts), len(want))
	}
	for i := range want {
		if !insts[i].Equal(want[i]) {
			t.Errorf("all-day instancia %d = %s, want %s", i, insts[i].UTC(), want[i])
		}
	}
}

// TestRecurrenceEndAcrossDST comprueba que el fin de una serie con COUNT se
// calcula desde el instante correcto según la zona (el último inicio cae en CEST).
func TestRecurrenceEndAcrossDST(t *testing.T) {
	mad := mustLoc(t, "Europe/Madrid")
	start := time.Date(2025, 3, 24, 9, 0, 0, 0, mad) // 08:00Z, CET
	r := mustParse(t, "FREQ=WEEKLY;COUNT=3")         // último inicio 2025-04-07 (CEST)
	end := r.RecurrenceEnd(start, time.Hour)
	if end == nil {
		t.Fatal("RecurrenceEnd nil para serie con COUNT")
	}
	// Último inicio 2025-04-07 09:00 CEST = 07:00Z; +1 h de duración = 08:00Z.
	if want := time.Date(2025, 4, 7, 8, 0, 0, 0, time.UTC); !end.Equal(want) {
		t.Errorf("RecurrenceEnd = %s, want %s", end.UTC(), want)
	}
}

// assertInstants verifica los instantes UTC esperados y que la hora-de-pared en
// `loc` sea `wantHour` en todas (invariante DST: la hora local no cambia).
func assertInstants(t *testing.T, got, want []time.Time, loc *time.Location, wantHour int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d instancias, want %d", len(got), len(want))
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("instancia %d = %s, want %s", i, got[i].UTC(), want[i])
		}
		if h := got[i].In(loc).Hour(); h != wantHour {
			t.Errorf("instancia %d hora local = %d, want %d (hora-de-pared constante)", i, h, wantHour)
		}
	}
}
