// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ical

import (
	"strings"
	"testing"
	"time"

	domain "git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

// TestLaSEQUENCESaleComoEntero: EncodeEvent escribe SEQUENCE como INTEGER (RFC 5545
// §3.8.7.4), sin «;VALUE=TEXT»; es lo que llevan las invitaciones iTIP que salen.
func TestLaSEQUENCESaleComoEntero(t *testing.T) {
	inicio := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	raw, err := EncodeEvent(domain.Event{UID: "seq-entero", Summary: "Reunión", DTStart: inicio, DTEnd: inicio.Add(time.Hour), SequenceNo: 3}, domain.MethodRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "\r\nSEQUENCE:3\r\n") || strings.Contains(raw, "SEQUENCE;") {
		t.Fatalf("SEQUENCE no sale como entero:\n%s", raw)
	}
}
