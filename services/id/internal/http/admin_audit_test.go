// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- cursor keyset del log de auditoría (format/parse, id bigint) ---

func TestAuditCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 7, 8, 12, 34, 56, 789012000, time.UTC)
	tok := formatAuditCursor(ts, 424242)
	got, err := parseAuditCursor(tok)
	if err != nil {
		t.Fatalf("parseAuditCursor: %v", err)
	}
	if got == nil || !got.OccurredAt.Equal(ts) || got.ID != 424242 {
		t.Fatalf("round-trip = %+v, quiero (%s, 424242)", got, ts)
	}
}

func TestParseAuditCursorEmptyIsFirstPage(t *testing.T) {
	got, err := parseAuditCursor("")
	if err != nil || got != nil {
		t.Fatalf("cursor vacío = (%+v, %v), quiero (nil, nil)", got, err)
	}
}

func TestParseAuditCursorRejectsGarbage(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	bad := []string{
		"@@no-base64url@@",                      // fuera del alfabeto base64url
		formatAuditCursor(time.Now(), 1) + "!!", // base64 corrupto
		enc("sin-separador"),                    // sin '|'
		enc("no-fecha|123"),                     // ts inválido
		enc("2026-07-08T00:00:00Z|not-int"),     // ts válido, id no numérico
	}
	for _, tok := range bad {
		if got, err := parseAuditCursor(tok); err == nil {
			t.Errorf("parseAuditCursor(%q) = %+v, quiero error", tok, got)
		}
	}
}

// --- parseAuditBound: RFC3339, fecha suelta y el ajuste inclusivo de until ---

func TestParseAuditBound(t *testing.T) {
	if got, err := parseAuditBound("", false); err != nil || got != nil {
		t.Fatalf("vacío = (%v,%v), quiero (nil,nil)", got, err)
	}
	rfc, err := parseAuditBound("2026-07-10T12:00:00Z", false)
	if err != nil || rfc == nil || !rfc.Equal(time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("RFC3339 = (%v,%v)", rfc, err)
	}
	// Fecha suelta como límite inferior: medianoche del día.
	lo, err := parseAuditBound("2026-07-10", false)
	if err != nil || !lo.Equal(time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("since fecha = (%v,%v)", lo, err)
	}
	// Fecha suelta como límite superior: día siguiente (until inclusivo del día).
	hi, err := parseAuditBound("2026-07-10", true)
	if err != nil || !hi.Equal(time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("until fecha = (%v,%v)", hi, err)
	}
	if _, err := parseAuditBound("no-es-fecha", false); err == nil {
		t.Fatal("basura debería fallar")
	}
}

// --- parseAuditFilter sobre la query real ---

func TestParseAuditFilterValid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet,
		"/?action=auth.&actor=00000000-0000-0000-0000-000000000001&since=2026-07-01&until=2026-07-07", http.NoBody)
	f, err := parseAuditFilter(r)
	if err != nil {
		t.Fatalf("parseAuditFilter: %v", err)
	}
	if f.Action != "auth." || f.ActorID != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("action/actor = %q/%q", f.Action, f.ActorID)
	}
	if f.Since == nil || f.Until == nil || !f.Since.Before(*f.Until) {
		t.Errorf("rango inválido: %v..%v", f.Since, f.Until)
	}
}

func TestParseAuditFilterRejects(t *testing.T) {
	bad := []string{
		"/?actor=not-a-uuid",
		"/?since=nope",
		"/?until=2026-13-40",
		"/?since=2026-07-10&until=2026-07-01", // rango invertido/vacío
	}
	for _, q := range bad {
		if _, err := parseAuditFilter(httptest.NewRequest(http.MethodGet, q, http.NoBody)); err == nil {
			t.Errorf("parseAuditFilter(%q) debería fallar", q)
		}
	}
}
