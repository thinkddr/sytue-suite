// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"
	"testing"
	"time"
)

// TestUnInformeALaVez (NEG-15): mientras uno se calcula, el siguiente espera y,
// si se le acaba el plazo, se rinde en vez de sumar otros 60 MB al pod.
func TestUnInformeALaVez(t *testing.T) {
	liberar, ok := tomarTurnoInforme(context.Background())
	if !ok {
		t.Fatal("el primero entra")
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelar()
	if _, ok := tomarTurnoInforme(ctx); ok {
		t.Fatal("con un informe en marcha, el segundo no entra")
	}
	liberar()
	otra, ok := tomarTurnoInforme(context.Background())
	if !ok {
		t.Fatal("al acabar el primero, entra el siguiente")
	}
	otra()
}
