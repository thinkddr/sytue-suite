// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) RRule {
	t.Helper()
	r, err := ParseRRule(s)
	if err != nil {
		t.Fatalf("ParseRRule(%q): %v", s, err)
	}
	return r
}

func TestParseRRuleErrors(t *testing.T) {
	cases := []string{
		"",
		"INTERVAL=2",  // sin FREQ
		"FREQ=HOURLY", // FREQ no soportada
		"FREQ=DAILY;COUNT=3;UNTIL=20240101T000000Z", // COUNT + UNTIL
		"FREQ=WEEKLY;BYDAY=XX",                      // dia invalido
		"FREQ=MONTHLY;BYMONTHDAY=40",                // fuera de rango
	}
	for _, c := range cases {
		if _, err := ParseRRule(c); err == nil {
			t.Errorf("ParseRRule(%q): se esperaba error", c)
		}
	}
}

func TestExpandDailyCount(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	r := mustParse(t, "FREQ=DAILY;COUNT=5")
	insts := r.Expand(start, nil, nil, start, start.AddDate(0, 1, 0))
	if len(insts) != 5 {
		t.Fatalf("DAILY COUNT=5: got %d instancias, want 5", len(insts))
	}
	for i, inst := range insts {
		want := start.AddDate(0, 0, i)
		if !inst.Equal(want) {
			t.Errorf("instancia %d = %s, want %s", i, inst, want)
		}
	}
}

func TestExpandDailyInterval(t *testing.T) {
	start := time.Date(2026, 1, 1, 8, 30, 0, 0, time.UTC)
	r := mustParse(t, "FREQ=DAILY;INTERVAL=2;COUNT=3")
	insts := r.Expand(start, nil, nil, start, start.AddDate(0, 1, 0))
	want := []time.Time{
		start,
		start.AddDate(0, 0, 2),
		start.AddDate(0, 0, 4),
	}
	if len(insts) != len(want) {
		t.Fatalf("got %d, want %d", len(insts), len(want))
	}
	for i := range want {
		if !insts[i].Equal(want[i]) {
			t.Errorf("instancia %d = %s, want %s", i, insts[i], want[i])
		}
	}
}

func TestExpandWeeklyByDay(t *testing.T) {
	// 2026-06-01 es lunes.
	start := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	r := mustParse(t, "FREQ=WEEKLY;BYDAY=MO,WE,FR;COUNT=6")
	insts := r.Expand(start, nil, nil, start, start.AddDate(0, 2, 0))
	if len(insts) != 6 {
		t.Fatalf("got %d, want 6", len(insts))
	}
	wantWeekdays := []time.Weekday{
		time.Monday, time.Wednesday, time.Friday,
		time.Monday, time.Wednesday, time.Friday,
	}
	for i, inst := range insts {
		if inst.Weekday() != wantWeekdays[i] {
			t.Errorf("instancia %d weekday = %s, want %s", i, inst.Weekday(), wantWeekdays[i])
		}
		if inst.Hour() != 10 {
			t.Errorf("instancia %d hour = %d, want 10", i, inst.Hour())
		}
	}
}

func TestExpandMonthlyByMonthDay(t *testing.T) {
	start := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	r := mustParse(t, "FREQ=MONTHLY;BYMONTHDAY=15;COUNT=4")
	insts := r.Expand(start, nil, nil, start, start.AddDate(1, 0, 0))
	want := []time.Time{
		time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	}
	if len(insts) != 4 {
		t.Fatalf("got %d, want 4", len(insts))
	}
	for i := range want {
		if !insts[i].Equal(want[i]) {
			t.Errorf("instancia %d = %s, want %s", i, insts[i], want[i])
		}
	}
}

func TestExpandMonthlyLastFriday(t *testing.T) {
	start := time.Date(2026, 1, 30, 9, 0, 0, 0, time.UTC) // ultimo viernes enero 2026
	r := mustParse(t, "FREQ=MONTHLY;BYDAY=-1FR;COUNT=3")
	insts := r.Expand(start, nil, nil, start, start.AddDate(1, 0, 0))
	if len(insts) != 3 {
		t.Fatalf("got %d, want 3", len(insts))
	}
	for _, inst := range insts {
		if inst.Weekday() != time.Friday {
			t.Errorf("instancia %s no es viernes", inst)
		}
		// Debe ser el ultimo viernes: no hay otro viernes despues en el mes.
		if inst.AddDate(0, 0, 7).Month() == inst.Month() {
			t.Errorf("instancia %s no es el ultimo viernes del mes", inst)
		}
	}
}

func TestExpandUntilAndExdate(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	r := mustParse(t, "FREQ=DAILY;UNTIL=20260605T090000Z")
	ex := []time.Time{time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)}
	insts := r.Expand(start, nil, ex, start, start.AddDate(0, 1, 0))
	// 1,2,(3 excluido),4,5 => 4 instancias
	if len(insts) != 4 {
		t.Fatalf("got %d, want 4 (UNTIL + EXDATE)", len(insts))
	}
	for _, inst := range insts {
		if inst.Day() == 3 {
			t.Errorf("la fecha excluida (dia 3) aparece en la expansion")
		}
	}
}

func TestExpandRDate(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	r := mustParse(t, "FREQ=WEEKLY;COUNT=2")
	rd := []time.Time{time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)}
	insts := r.Expand(start, rd, nil, start, start.AddDate(0, 1, 0))
	// 06-01, 06-08 (weekly count 2) + RDATE 06-03 => 3
	if len(insts) != 3 {
		t.Fatalf("got %d, want 3 (weekly + RDATE)", len(insts))
	}
}

func TestRecurrenceEnd(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	dur := time.Hour

	if e := mustParse(t, "FREQ=DAILY").RecurrenceEnd(start, dur); e != nil {
		t.Errorf("regla infinita: RecurrenceEnd debe ser nil, got %s", e)
	}

	rUntil := mustParse(t, "FREQ=DAILY;UNTIL=20260610T090000Z")
	if e := rUntil.RecurrenceEnd(start, dur); e == nil || !e.Equal(time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("UNTIL: RecurrenceEnd = %v, want 2026-06-10T10:00Z", e)
	}

	rCount := mustParse(t, "FREQ=DAILY;COUNT=3")
	if e := rCount.RecurrenceEnd(start, dur); e == nil || !e.Equal(time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("COUNT: RecurrenceEnd = %v, want 2026-06-03T10:00Z", e)
	}
}

func TestFreeBusyMergesOverlaps(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	mk := func(h1, h2 int, status EventStatus, transp Transparency) Instance {
		ev := Event{Status: status, Transparency: transp}
		return Instance{
			Event: ev,
			Start: base.Add(time.Duration(h1) * time.Hour),
			End:   base.Add(time.Duration(h2) * time.Hour),
		}
	}
	insts := []Instance{
		mk(9, 10, StatusConfirmed, TranspOpaque),
		mk(9, 11, StatusConfirmed, TranspOpaque),       // solapa con el anterior
		mk(14, 15, StatusConfirmed, TranspOpaque),      // separado
		mk(16, 17, StatusConfirmed, TranspTransparent), // TRANSPARENT: no cuenta
		mk(18, 19, StatusCancelled, TranspOpaque),      // CANCELLED: no cuenta
	}
	periods := FreeBusy(insts, base, base.AddDate(0, 0, 1))
	if len(periods) != 2 {
		t.Fatalf("got %d periodos, want 2", len(periods))
	}
	if !periods[0].Start.Equal(base.Add(9*time.Hour)) || !periods[0].End.Equal(base.Add(11*time.Hour)) {
		t.Errorf("periodo 0 = %v..%v, want 09..11", periods[0].Start, periods[0].End)
	}
	if !periods[1].Start.Equal(base.Add(14*time.Hour)) || !periods[1].End.Equal(base.Add(15*time.Hour)) {
		t.Errorf("periodo 1 = %v..%v, want 14..15", periods[1].Start, periods[1].End)
	}
}

func TestExpandEventsAppliesOverride(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	master := Event{
		UID:          "evt-1",
		DTStart:      start,
		DTEnd:        start.Add(time.Hour),
		Status:       StatusConfirmed,
		Transparency: TranspOpaque,
		RRule:        "FREQ=DAILY;COUNT=3",
	}
	// Override la segunda instancia (06-02) movida a las 15:00.
	recID := start.AddDate(0, 0, 1)
	override := Event{
		UID:          "evt-1",
		DTStart:      time.Date(2026, 6, 2, 15, 0, 0, 0, time.UTC),
		DTEnd:        time.Date(2026, 6, 2, 16, 0, 0, 0, time.UTC),
		Status:       StatusConfirmed,
		Transparency: TranspOpaque,
		RecurrenceID: &recID,
	}
	insts := ExpandEvents([]Event{master, override}, start, start.AddDate(0, 0, 7))
	if len(insts) != 3 {
		t.Fatalf("got %d instancias, want 3", len(insts))
	}
	var found bool
	for _, in := range insts {
		if in.Start.Equal(time.Date(2026, 6, 2, 15, 0, 0, 0, time.UTC)) {
			found = true
		}
		if in.Start.Equal(time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)) {
			t.Errorf("la instancia original 06-02T09:00 no fue reemplazada por el override")
		}
	}
	if !found {
		t.Errorf("no se encontro la instancia override 06-02T15:00")
	}
}
