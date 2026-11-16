// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNoteInputChocaSoloSiPideVersionYEsOtra(t *testing.T) {
	guardada := time.Date(2026, 11, 9, 10, 0, 0, 123456000, time.UTC)
	var sin NoteInput
	if sin.ChocaCon(guardada) {
		t.Fatal("sin versión esperada la escritura no debe chocar: los clientes viejos escriben como antes")
	}
	misma := guardada.In(time.FixedZone("CET", 3600))
	if (&NoteInput{ExpectedUpdatedAt: &misma}).ChocaCon(guardada) {
		t.Fatal("la misma instante en otra zona no es un conflicto")
	}
	otra := guardada.Add(-time.Microsecond)
	if !(&NoteInput{ExpectedUpdatedAt: &otra}).ChocaCon(guardada) {
		t.Fatal("una versión distinta de la guardada debe chocar")
	}
}

func TestNoteInputLeeLaVersionEsperadaDelJSON(t *testing.T) {
	var in NoteInput
	if err := json.Unmarshal([]byte(`{"title":"a","expected_updated_at":"2026-11-09T10:00:00.123456Z"}`), &in); err != nil {
		t.Fatal(err)
	}
	if in.ExpectedUpdatedAt == nil || !in.ExpectedUpdatedAt.Equal(time.Date(2026, 11, 9, 10, 0, 0, 123456000, time.UTC)) {
		t.Fatalf("expected_updated_at no se ha leído: %v", in.ExpectedUpdatedAt)
	}
}
