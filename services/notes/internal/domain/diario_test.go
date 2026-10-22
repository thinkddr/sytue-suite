// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package domain

import "testing"

func TestRangoDiario(t *testing.T) {
	casos := []struct {
		from, to string
		max      int
		ok       bool
	}{
		{"2026-10-01", "2026-10-07", 7, true},
		{"2026-10-01", "2026-10-01", 1, true},
		{"2026-10-01", "2026-10-08", 7, false}, // ocho días
		{"2026-10-07", "2026-10-01", 7, false}, // al revés
		{"2026-02-30", "2026-03-01", 7, false}, // día inexistente
		{"2026-10-1", "2026-10-02", 7, false},  // sin cero
		{"", "2026-10-02", 7, false},
	}
	for _, c := range casos {
		if got := RangoDiario(c.from, c.to, c.max); got != c.ok {
			t.Errorf("RangoDiario(%q, %q, %d) = %v, quería %v", c.from, c.to, c.max, got, c.ok)
		}
	}
}
