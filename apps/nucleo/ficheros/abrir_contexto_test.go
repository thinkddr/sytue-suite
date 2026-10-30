// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"context"
	"errors"
	"testing"
)

func TestAbrirContextoCanceladoNoEntregaElFichero(t *testing.T) {
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	if err := AbrirContexto(ctx, "no-debe-consultarse"); !errors.Is(err, context.Canceled) {
		t.Fatalf("una apertura cancelada consultó o entregó el fichero: %v", err)
	}
}
