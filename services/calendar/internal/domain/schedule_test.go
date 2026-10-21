// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests DETERMINISTAS del asistente de programacion («buscar hueco»): los huecos
// libres comunes = complemento de la union de ocupados, restringido a la franja
// laboral, con duracion suficiente y DST correcto. Instantes fijados por
// constantes (nunca time.Now).
package domain

import (
	"testing"
	"time"
)

// busy construye un periodo ocupado [s, e) UTC.
func busy(s, e time.Time) FreeBusyPeriod {
	return FreeBusyPeriod{Start: s.UTC(), End: e.UTC(), Type: FBBusy}
}

// utc es un instante UTC compacto.
func utc(y int, mo time.Month, d, h, m int) time.Time {
	return time.Date(y, mo, d, h, m, 0, 0, time.UTC)
}

func assertSlots(t *testing.T, got, want []FreeSlot) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d huecos, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !got[i].Start.Equal(want[i].Start) || !got[i].End.Equal(want[i].End) {
			t.Errorf("hueco %d = [%s, %s], want [%s, %s]", i,
				got[i].Start.UTC(), got[i].End.UTC(), want[i].Start.UTC(), want[i].End.UTC())
		}
	}
}

var work9to17 = WorkingHours{StartMin: 9 * 60, EndMin: 17 * 60}

// TestSuggestBasicSplitsAroundBusy: un ocupado en mitad de la jornada parte la
// franja laboral en dos huecos (antes y despues), ninguno fuera de 09:00-17:00.
func TestSuggestBasicSplitsAroundBusy(t *testing.T) {
	from, to := utc(2025, 6, 2, 0, 0), utc(2025, 6, 3, 0, 0) // lunes completo
	b := []FreeBusyPeriod{busy(utc(2025, 6, 2, 10, 0), utc(2025, 6, 2, 11, 0))}
	got := SuggestFreeSlots(b, from, to, time.UTC, work9to17, time.Hour, 50)
	assertSlots(t, got, []FreeSlot{
		{Start: utc(2025, 6, 2, 9, 0), End: utc(2025, 6, 2, 10, 0)},
		{Start: utc(2025, 6, 2, 11, 0), End: utc(2025, 6, 2, 17, 0)},
	})
}

// TestSuggestEmptyBusyIsFullWorkday: sin ocupados, el unico hueco del dia es la
// franja laboral entera (respeta la ventana; nada de noche).
func TestSuggestEmptyBusyIsFullWorkday(t *testing.T) {
	from, to := utc(2025, 6, 2, 0, 0), utc(2025, 6, 3, 0, 0)
	got := SuggestFreeSlots(nil, from, to, time.UTC, work9to17, 30*time.Minute, 50)
	assertSlots(t, got, []FreeSlot{
		{Start: utc(2025, 6, 2, 9, 0), End: utc(2025, 6, 2, 17, 0)},
	})
}

// TestSuggestIntersectionTwoParticipants: la UNION de los ocupados de A y B deja
// un unico hueco comun [12:00, 13:00). Solo con dur <= 60m es sugerible.
func TestSuggestIntersectionTwoParticipants(t *testing.T) {
	from, to := utc(2025, 6, 2, 0, 0), utc(2025, 6, 3, 0, 0)
	b := []FreeBusyPeriod{
		busy(utc(2025, 6, 2, 9, 0), utc(2025, 6, 2, 12, 0)),  // A: manana
		busy(utc(2025, 6, 2, 13, 0), utc(2025, 6, 2, 17, 0)), // B: tarde
	}
	got := SuggestFreeSlots(b, from, to, time.UTC, work9to17, time.Hour, 50)
	assertSlots(t, got, []FreeSlot{
		{Start: utc(2025, 6, 2, 12, 0), End: utc(2025, 6, 2, 13, 0)},
	})
	// Con 61 minutos el hueco de 60 no cabe: sin sugerencias.
	if none := SuggestFreeSlots(b, from, to, time.UTC, work9to17, 61*time.Minute, 50); len(none) != 0 {
		t.Errorf("dur=61m: got %d huecos, want 0", len(none))
	}
}

// TestSuggestAllBusyNoSlots: si todo el dia (mas alla de la franja) esta ocupado,
// no hay ningun hueco.
func TestSuggestAllBusyNoSlots(t *testing.T) {
	from, to := utc(2025, 6, 2, 0, 0), utc(2025, 6, 3, 0, 0)
	b := []FreeBusyPeriod{busy(utc(2025, 6, 2, 8, 0), utc(2025, 6, 2, 18, 0))}
	if got := SuggestFreeSlots(b, from, to, time.UTC, work9to17, time.Hour, 50); len(got) != 0 {
		t.Errorf("todo ocupado: got %d huecos, want 0: %v", len(got), got)
	}
}

// TestSuggestOverlappingBusyMerges: ocupados solapados de participantes distintos
// se fusionan antes de restar (no dejan micro-huecos espurios).
func TestSuggestOverlappingBusyMerges(t *testing.T) {
	from, to := utc(2025, 6, 2, 0, 0), utc(2025, 6, 3, 0, 0)
	b := []FreeBusyPeriod{
		busy(utc(2025, 6, 2, 10, 0), utc(2025, 6, 2, 12, 0)),
		busy(utc(2025, 6, 2, 11, 0), utc(2025, 6, 2, 13, 0)), // solapa al anterior
	}
	got := SuggestFreeSlots(b, from, to, time.UTC, work9to17, 30*time.Minute, 50)
	assertSlots(t, got, []FreeSlot{
		{Start: utc(2025, 6, 2, 9, 0), End: utc(2025, 6, 2, 10, 0)},
		{Start: utc(2025, 6, 2, 13, 0), End: utc(2025, 6, 2, 17, 0)},
	})
}

// TestSuggestDurationFilter: los tramos mas cortos que la duracion se descartan;
// con una duracion menor reaparecen.
func TestSuggestDurationFilter(t *testing.T) {
	from, to := utc(2025, 6, 2, 0, 0), utc(2025, 6, 3, 0, 0)
	b := []FreeBusyPeriod{busy(utc(2025, 6, 2, 9, 30), utc(2025, 6, 2, 16, 30))}
	if got := SuggestFreeSlots(b, from, to, time.UTC, work9to17, time.Hour, 50); len(got) != 0 {
		t.Errorf("tramos de 30m con dur=60m: got %d, want 0", len(got))
	}
	got := SuggestFreeSlots(b, from, to, time.UTC, work9to17, 30*time.Minute, 50)
	assertSlots(t, got, []FreeSlot{
		{Start: utc(2025, 6, 2, 9, 0), End: utc(2025, 6, 2, 9, 30)},
		{Start: utc(2025, 6, 2, 16, 30), End: utc(2025, 6, 2, 17, 0)},
	})
}

// TestSuggestWeekdaysOnly: un sabado no ofrece huecos cuando WeekdaysOnly.
func TestSuggestWeekdaysOnly(t *testing.T) {
	from, to := utc(2025, 6, 7, 0, 0), utc(2025, 6, 8, 0, 0) // 2025-06-07 es sabado
	wh := WorkingHours{StartMin: 9 * 60, EndMin: 17 * 60, WeekdaysOnly: true}
	if got := SuggestFreeSlots(nil, from, to, time.UTC, wh, time.Hour, 50); len(got) != 0 {
		t.Errorf("sabado con WeekdaysOnly: got %d huecos, want 0", len(got))
	}
	// Sin la restriccion, el sabado si tiene su franja libre.
	wh.WeekdaysOnly = false
	if got := SuggestFreeSlots(nil, from, to, time.UTC, wh, time.Hour, 50); len(got) != 1 {
		t.Errorf("sabado sin restriccion: got %d huecos, want 1", len(got))
	}
}

// TestSuggestMadridSpringForwardDST: la franja «09:00-17:00» en Europe/Madrid el
// dia del spring-forward (2025-03-30) es hora-de-pared LOCAL: 09:00 CEST (+2) =
// 07:00Z y 17:00 CEST = 15:00Z. El hueco libre respeta el offset del dia.
func TestSuggestMadridSpringForwardDST(t *testing.T) {
	mad, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatalf("LoadLocation: %v (¿tzdata embebida?)", err)
	}
	from := time.Date(2025, 3, 30, 0, 0, 0, 0, mad) // 00:00 CET = 2025-03-29 23:00Z
	to := time.Date(2025, 3, 31, 0, 0, 0, 0, mad)   // 00:00 CEST 03-31 = 2025-03-30 22:00Z
	got := SuggestFreeSlots(nil, from, to, mad, work9to17, time.Hour, 50)
	assertSlots(t, got, []FreeSlot{
		{Start: utc(2025, 3, 30, 7, 0), End: utc(2025, 3, 30, 15, 0)},
	})
	// La hora-de-pared local debe ser 09:00-17:00 pese al cambio de hora.
	if h := got[0].Start.In(mad).Hour(); h != 9 {
		t.Errorf("inicio local = %d:00, want 09:00", h)
	}
	if h := got[0].End.In(mad).Hour(); h != 17 {
		t.Errorf("fin local = %d:00, want 17:00", h)
	}
}

// TestSuggestMultiDayLimit: la sugerencia recorre varios dias y respeta el limite.
func TestSuggestMultiDayLimit(t *testing.T) {
	from, to := utc(2025, 6, 2, 0, 0), utc(2025, 6, 6, 0, 0) // lun-jue (4 dias)
	got := SuggestFreeSlots(nil, from, to, time.UTC, work9to17, time.Hour, 2)
	if len(got) != 2 {
		t.Fatalf("limit=2: got %d huecos, want 2", len(got))
	}
	// Van en orden cronologico: dia 2 y dia 3.
	if got[0].Start.Day() != 2 || got[1].Start.Day() != 3 {
		t.Errorf("orden = dias %d,%d, want 2,3", got[0].Start.Day(), got[1].Start.Day())
	}
}
