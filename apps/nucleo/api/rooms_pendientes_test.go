// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import "testing"

// TestLoQueSumaUnCanalDeRooms es la regla que comparten la lista de Rooms y su widget:
// silenciado o «nada» no suma, «menciones» solo las menciones y el resto lo no leído.
func TestLoQueSumaUnCanalDeRooms(t *testing.T) {
	casos := []struct {
		silenciado bool
		nivel      string
		quiero     int
	}{
		{false, "", 5},
		{false, "all", 5},
		{false, "mentions", 2},
		{false, "none", 0},
		{true, "all", 0},
		{true, "mentions", 0},
	}
	for _, c := range casos {
		if n := PendientesRooms(c.silenciado, c.nivel, 5, 2); n != c.quiero {
			t.Errorf("silenciado=%v nivel=%q suma %d; quiero %d", c.silenciado, c.nivel, n, c.quiero)
		}
	}
	if n := PendientesRooms(false, "all", -3, 0); n != 0 {
		t.Errorf("un contador negativo resta: %d", n)
	}
}
