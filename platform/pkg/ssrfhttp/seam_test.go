// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ssrfhttp

import (
	"context"
	"net"
	"testing"
)

// TestSetIPPolicyForTest comprueba que el seam relaja la política y la RESTAURA:
// por defecto loopback está bloqueado; con la política relajada pasa; tras restaurar
// vuelve a bloquearse. Sin esto, ningún test hermético (httptest en loopback) podría
// ejercitar el código guardado por SSRF.
func TestSetIPPolicyForTest(t *testing.T) {
	ctx := context.Background()

	if err := ValidateHost(ctx, "127.0.0.1"); err == nil {
		t.Fatal("por defecto, 127.0.0.1 debería estar bloqueado")
	}

	restore := SetIPPolicyForTest(func(net.IP) bool { return false })
	if err := ValidateHost(ctx, "127.0.0.1"); err != nil {
		t.Fatalf("con la política relajada, 127.0.0.1 debería pasar: %v", err)
	}

	restore()
	if err := ValidateHost(ctx, "127.0.0.1"); err == nil {
		t.Fatal("tras restaurar, 127.0.0.1 debería volver a bloquearse")
	}
}
