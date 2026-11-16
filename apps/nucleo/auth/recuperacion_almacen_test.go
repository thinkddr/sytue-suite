// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"strings"
	"testing"
)

func TestOnlyScopedLoginBoundSSSSRecoveryUsesSharedAppleGroup(t *testing.T) {
	exact := PrefijoRecuperacionAlmacen + strings.Repeat("a", 64) + "-" + strings.Repeat("b", 32)
	if !cuentaRecuperacionAlmacen(exact) || !cuentaValida(exact) {
		t.Fatal("exact scoped recovery purpose rejected")
	}
	for _, name := range []string{PrefijoRecuperacionAlmacen, exact + "x", strings.ToUpper(exact), strings.Replace(exact, "-bbbb", "_bbbb", 1), "rooms-matrix-session-" + strings.Repeat("a", 64), "rooms-matrix-master-" + strings.Repeat("a", 64), "offline-cache-" + strings.Repeat("a", 32)} {
		if cuentaRecuperacionAlmacen(name) {
			t.Fatal("foreign secret purpose would enter suite keychain group", name)
		}
	}
}
