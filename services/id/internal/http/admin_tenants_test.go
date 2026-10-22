// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"testing"
	"time"
)

// El cursor keyset de la lista de organizaciones (format/parse) debe ir y volver
// sin pérdida, normalizando la zona a UTC (misma posición keyset). Espeja el test
// del cursor de usuarios.
func TestOrgCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 7, 18, 9, 8, 7, 654321000, time.UTC)
	const id = "1a2b3c4d-5e6f-4a1b-8c2d-3e4f5a6b7c8d"

	tok := formatOrgCursor(ts, id)
	if tok == "" {
		t.Fatal("formatOrgCursor devolvió vacío")
	}
	got, err := parseOrgCursor(tok)
	if err != nil {
		t.Fatalf("parseOrgCursor(round-trip): %v", err)
	}
	if got == nil || !got.CreatedAt.Equal(ts) || got.ID != id {
		t.Fatalf("round-trip: %+v, quiero (%s, %s)", got, ts, id)
	}

	mad, _ := time.LoadLocation("Europe/Madrid")
	got2, err := parseOrgCursor(formatOrgCursor(ts.In(mad), id))
	if err != nil || got2 == nil || !got2.CreatedAt.Equal(ts) {
		t.Fatalf("round-trip con zona: %+v (%v)", got2, err)
	}
}

// Un cursor vacío es la primera página (nil, nil); uno corrupto o con id no-uuid
// es entrada inválida del cliente (error), nunca un id que rompa el cast ::uuid.
func TestParseOrgCursorEmptyAndInvalid(t *testing.T) {
	got, err := parseOrgCursor("")
	if err != nil || got != nil {
		t.Fatalf("cursor vacío = (%+v, %v), quiero (nil, nil)", got, err)
	}
	if _, err := parseOrgCursor("!!!not-base64!!!"); err == nil {
		t.Fatal("cursor no-base64 debería fallar")
	}
	if _, err := parseOrgCursor(formatOrgCursor(time.Now(), "no-es-uuid")); err == nil {
		t.Fatal("cursor con id no-uuid debería fallar")
	}
}
