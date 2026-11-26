// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"testing"
)

// Buscar en las notas sigue la regla de apps/nucleo/buscar: «cafe» encuentra «Café».
func TestLasNotasSeBuscanSinAcentos(t *testing.T) {
	ctx := context.Background()
	n, err := AbrirNotas(t.TempDir(), "Notas", "notas", "")
	if err != nil {
		t.Fatal(err)
	}
	cs, _, _ := n.Canales(ctx)
	if _, err := n.Enviar(ctx, cs[0].ID, "", "Café con Begoña a las diez"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"cafe", "BEGONA", "café"} {
		if r, err := n.Buscar(ctx, q, 10); err != nil || len(r) != 1 {
			t.Fatalf("buscar %q: %v %v", q, r, err)
		}
	}
}
