// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package coedicion

import (
	"testing"
	"unicode/utf16"
)

// TestAnclarOffsetComoLaWeb: los casos de `cursors.test.ts` («anclarOffset»), uno a uno.
func TestAnclarOffsetComoLaWeb(t *testing.T) {
	u := func(s string) []uint16 { return utf16.Encode([]rune(s)) }
	for _, c := range []struct {
		off            int
		antes, despues string
		quiero         int
	}{
		{3, "hola mundo", "hola mundo", 3},
		{4, "hola mundo", "¡hola mundo", 5},
		{4, "hola mundo", "la mundo", 2},
		{4, "hola mundo", "hola mundo cruel", 4},
		{0, "hola", "xhola", 0},
		{4, "hola mundo", "hola, qué tal mundo", 4},
		{7, "hola mundo cruel", "hola cruel", 5},
		{2, "aaaa", "aaaaa", 2},
		{4, "aaaa", "aaaaa", 5},
		{1, "a", "", 0},
		{4, "hola", "hola!", 5},
		{4, "hola", "hola mundo", 10},
		{2, "hola", "hola!", 2},
		// Un emoji delante son dos unidades, como en `textContent`.
		{4, "hola", "👍hola", 6},
	} {
		if got := AnclarOffset(c.off, u(c.antes), u(c.despues)); got != c.quiero {
			t.Errorf("AnclarOffset(%d, %q, %q) = %d; la web da %d", c.off, c.antes, c.despues, got, c.quiero)
		}
	}
}
