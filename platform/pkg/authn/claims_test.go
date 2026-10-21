// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"context"
	"testing"
	"time"
)

// COM-08. Una conexión larga no vive más que su token.
func TestHastaQueCaduqueCortaAlExpirar(t *testing.T) {
	ctx, cancel := HastaQueCaduque(context.Background(), Claims{Expira: time.Now().Add(50 * time.Millisecond)})
	defer cancel()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("el contexto sigue vivo después del exp del token")
	}
	sinExp, cancel2 := HastaQueCaduque(context.Background(), Claims{})
	defer cancel2()
	if _, tiene := sinExp.Deadline(); tiene {
		t.Fatal("sin exp no se inventa un plazo")
	}
}
