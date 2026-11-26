// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package autorizacion

import "testing"

// Una intención solo avanza desde la revisión en que se comprobó: la de antes de un
// cambio de cuenta no se revive.
func TestRenovadaSoloAvanzaDesdeLaRevisionComprobada(t *testing.T) {
	i := &AuthorizationIntent{revision: 3}
	if i.Renovada(2, 7) || i.revision != 3 {
		t.Fatal("avanzó desde una revisión que no era la suya")
	}
	if !i.Renovada(3, 4) || i.revision != 4 {
		t.Fatal("no avanzó desde la suya")
	}
	var nada *AuthorizationIntent
	if nada.Renovada(0, 1) {
		t.Fatal("una intención nula no avanza")
	}
}
