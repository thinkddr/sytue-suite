// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"testing"
	"time"
)

// --- cursor keyset del listado admin de usuarios (format/parse) ---

func TestUserCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 7, 8, 12, 34, 56, 789012000, time.UTC)
	const id = "0d4b0f6e-8b1a-4c1e-9f0a-2d3c4b5a6978"

	tok := formatUserCursor(ts, id)
	if tok == "" {
		t.Fatal("formatUserCursor devolvió vacío")
	}
	got, err := parseUserCursor(tok)
	if err != nil {
		t.Fatalf("parseUserCursor(round-trip): %v", err)
	}
	if got == nil || !got.CreatedAt.Equal(ts) || got.ID != id {
		t.Fatalf("round-trip: %+v, quiero (%s, %s)", got, ts, id)
	}

	// La zona horaria se normaliza a UTC al formatear: misma posición keyset.
	mad, _ := time.LoadLocation("Europe/Madrid")
	got2, err := parseUserCursor(formatUserCursor(ts.In(mad), id))
	if err != nil || !got2.CreatedAt.Equal(ts) {
		t.Fatalf("round-trip con zona: %+v (%v)", got2, err)
	}
}

func TestParseUserCursorEmptyIsFirstPage(t *testing.T) {
	got, err := parseUserCursor("")
	if err != nil || got != nil {
		t.Fatalf("cursor vacío = (%+v, %v), quiero (nil, nil)", got, err)
	}
}

func TestParseUserCursorRejectsGarbage(t *testing.T) {
	ts := time.Now().UTC()
	const id = "0d4b0f6e-8b1a-4c1e-9f0a-2d3c4b5a6978"
	bad := []string{
		"@@no-base64url@@",
		formatUserCursor(ts, id) + "x",        // base64 corrupto
		"c2luLXNlcGFyYWRvcg",                  // "sin-separador"
		"bm8tZmVjaGF8MTIzNA",                  // "no-fecha|1234": ts inválido
		formatUserCursor(ts, "no-es-un-uuid"), // ts válido, id no-UUID
	}
	for _, tok := range bad {
		if got, err := parseUserCursor(tok); err == nil {
			t.Fatalf("parseUserCursor(%q) = %+v, quiero error", tok, got)
		}
	}
}
