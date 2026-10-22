// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// TestUptimeTransitionEvent fija la lógica de la que depende que RecordCheckResult
// emita (o no) un evento al bus: solo la TRANSICIÓN up↔down entre estados conocidos
// notifica; el baseline (primer sondeo, estado previo desconocido) y la repetición
// del mismo estado NO emiten (evita ruido).
func TestUptimeTransitionEvent(t *testing.T) {
	cases := []struct {
		old, next string
		wantType  string
		wantOK    bool
	}{
		{domain.UptimeStatusUnknown, domain.UptimeStatusUp, "", false},   // baseline up
		{domain.UptimeStatusUnknown, domain.UptimeStatusDown, "", false}, // baseline down
		{domain.UptimeStatusUp, domain.UptimeStatusUp, "", false},        // sin cambio
		{domain.UptimeStatusDown, domain.UptimeStatusDown, "", false},    // sin cambio
		{domain.UptimeStatusUp, domain.UptimeStatusDown, EventUptimeDown, true},
		{domain.UptimeStatusDown, domain.UptimeStatusUp, EventUptimeUp, true},
	}
	for _, c := range cases {
		gotType, gotOK := uptimeTransitionEvent(c.old, c.next)
		if gotType != c.wantType || gotOK != c.wantOK {
			t.Errorf("uptimeTransitionEvent(%q,%q) = (%q,%v), quiero (%q,%v)",
				c.old, c.next, gotType, gotOK, c.wantType, c.wantOK)
		}
	}
}
