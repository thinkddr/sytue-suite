// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"
)

// evAt construye un evento puntual OPAQUE/CONFIRMED con UID dado en [start, +dur).
func evAt(uid string, start time.Time, dur time.Duration) Event {
	return Event{
		UID: uid, DTStart: start, DTEnd: start.Add(dur),
		TZID: "UTC", Status: StatusConfirmed, Transparency: TranspOpaque,
	}
}

func TestResourceConflict_FreeSlot(t *testing.T) {
	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	existing := []Event{evAt("a", base, time.Hour)} // 09:00–10:00
	// Candidata 10:00–11:00: contigua, no solapa (semiabierto).
	cand := evAt("cand", base.Add(time.Hour), time.Hour)
	if p, conflict := ResourceConflict(existing, cand, cand.DTStart, cand.DTEnd); conflict {
		t.Fatalf("franja libre marcada como ocupada: %+v", p)
	}
}

func TestResourceConflict_Overlap(t *testing.T) {
	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	existing := []Event{evAt("a", base, time.Hour)}           // 09:00–10:00
	cand := evAt("cand", base.Add(30*time.Minute), time.Hour) // 09:30–10:30 solapa
	if _, conflict := ResourceConflict(existing, cand, cand.DTStart, cand.DTEnd); !conflict {
		t.Fatal("solape no detectado (doble-reserva permitida)")
	}
}

// Una reserva recurrente existente ocupa el recurso en CADA ocurrencia: una
// candidata que cae en una de ellas (aunque sea semanas después) debe rechazarse.
func TestResourceConflict_RecurringExisting(t *testing.T) {
	base := time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC) // lunes
	series := evAt("weekly", base, time.Hour)
	series.RRule = "FREQ=WEEKLY;COUNT=8"
	existing := []Event{series}
	// Candidata en el 3er lunes (2 semanas después), misma hora → choca.
	cand := evAt("cand", base.AddDate(0, 0, 14), time.Hour)
	if _, conflict := ResourceConflict(existing, cand, cand.DTStart, cand.DTEnd); !conflict {
		t.Fatal("ocurrencia recurrente no detectada como ocupada")
	}
	// Candidata un martes (día sin ocurrencia) → libre.
	free := evAt("cand2", base.AddDate(0, 0, 1), time.Hour)
	if _, conflict := ResourceConflict(existing, free, free.DTStart, free.DTEnd); conflict {
		t.Fatal("día sin ocurrencia marcado como ocupado")
	}
}

// El solape se calcula en el INSTANTE real (UTC), respetando el huso de cada lado:
// 10:00 en Madrid (verano, +02:00) = 08:00 UTC, que choca con una reserva 08:00 UTC.
func TestResourceConflict_AcrossTimeZones(t *testing.T) {
	existing := []Event{evAt("utc", time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC), time.Hour)}
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skipf("sin tzdata Europe/Madrid: %v", err)
	}
	cand := Event{
		UID: "cand", TZID: "Europe/Madrid", Status: StatusConfirmed, Transparency: TranspOpaque,
		DTStart: time.Date(2026, 7, 1, 10, 0, 0, 0, madrid).UTC(),
		DTEnd:   time.Date(2026, 7, 1, 11, 0, 0, 0, madrid).UTC(),
	}
	if _, conflict := ResourceConflict(existing, cand, cand.DTStart, cand.DTEnd); !conflict {
		t.Fatal("solape entre husos no detectado (10:00 Madrid = 08:00 UTC)")
	}
}

// Reprogramar la MISMA reserva (mismo UID) no debe chocar con su versión anterior.
func TestResourceConflict_SelfReschedule(t *testing.T) {
	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	existing := []Event{evAt("evt-42", base, time.Hour)}
	cand := evAt("evt-42", base.Add(15*time.Minute), time.Hour) // se solapa consigo misma
	if _, conflict := ResourceConflict(existing, cand, cand.DTStart, cand.DTEnd); conflict {
		t.Fatal("reprogramar la propia reserva se rechazó como doble-reserva")
	}
}

// Una reserva CANCELADA o TRANSPARENT no ocupa: no cuenta para el conflicto.
func TestResourceConflict_CancelledNotBusy(t *testing.T) {
	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	cancelled := evAt("a", base, time.Hour)
	cancelled.Status = StatusCancelled
	cand := evAt("cand", base, time.Hour)
	if _, conflict := ResourceConflict([]Event{cancelled}, cand, cand.DTStart, cand.DTEnd); conflict {
		t.Fatal("una reserva cancelada no debe ocupar el recurso")
	}
}

func TestValidResource(t *testing.T) {
	cases := []struct {
		name     string
		kind     ResourceKind
		resName  string
		capacity int
		email    string
		want     bool
	}{
		{"sala ok", ResourceRoom, "Sala A", 8, "sala-a@sytue.eu", true},
		{"equipo ok sin email", ResourceEquipment, "Proyector", 0, "", true},
		{"tipo inválido", "vehicle", "Coche", 4, "", false},
		{"nombre vacío", ResourceRoom, "  ", 4, "", false},
		{"aforo negativo", ResourceRoom, "Sala", -1, "", false},
		{"email inválido", ResourceRoom, "Sala", 4, "no-arroba", false},
	}
	for _, c := range cases {
		if got := ValidResource(c.kind, c.resName, c.capacity, c.email); got != c.want {
			t.Errorf("%s: ValidResource=%v, want %v", c.name, got, c.want)
		}
	}
}

func TestShareCanWrite(t *testing.T) {
	if CanWrite(PermReader) {
		t.Error("reader no debe poder escribir")
	}
	if !CanWrite(PermWriter) || !CanWrite(PermManage) {
		t.Error("writer y manage (delegación) deben poder escribir")
	}
	if !ValidSharePermission(PermManage) {
		t.Error("manage debe ser un permiso válido")
	}
}
