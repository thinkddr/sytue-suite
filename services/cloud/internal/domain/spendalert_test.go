// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
	"time"
)

func TestCrossedSpendThresholds(t *testing.T) {
	const limit = 100.0

	t.Run("por debajo del primer umbral no avisa", func(t *testing.T) {
		for _, spend := range []float64{0, 1, 79.99} {
			if got := CrossedSpendThresholds(spend, limit); got != nil {
				t.Errorf("gasto %.2f de %.0f → %v, no debería avisar", spend, limit, got)
			}
		}
	})

	t.Run("al cruzar el 80 % avisa solo del 80", func(t *testing.T) {
		got := CrossedSpendThresholds(80, limit)
		if len(got) != 1 || got[0] != 80 {
			t.Errorf("= %v, quiero [80]", got)
		}
	})

	t.Run("un salto grande devuelve TODOS los cruzados, no solo el mayor", func(t *testing.T) {
		// Escenario real: un despliegue caro entre dos evaluaciones, o el evaluador
		// caído un rato. El cliente debe enterarse igual de que pasó por el 80 %.
		got := CrossedSpendThresholds(250, limit)
		if len(got) != 2 || got[0] != 80 || got[1] != 100 {
			t.Errorf("= %v, quiero [80 100]", got)
		}
	})

	t.Run("sin límite no hay techo que cruzar", func(t *testing.T) {
		for _, lim := range []float64{0, -1} {
			if got := CrossedSpendThresholds(9999, lim); got != nil {
				t.Errorf("límite %.0f → %v, «sin límite» no puede avisar", lim, got)
			}
		}
	})

	t.Run("gasto cero nunca avisa aunque el límite sea ridículo", func(t *testing.T) {
		if got := CrossedSpendThresholds(0, 0.001); got != nil {
			t.Errorf("= %v, sin gasto no hay aviso", got)
		}
	})
}

func TestSpendAlertPeriod(t *testing.T) {
	// El periodo es el mes natural en UTC: es el mismo que factura el motor de
	// facturas, así que el aviso y el cobro hablan del mismo tramo.
	in := time.Date(2026, 7, 28, 23, 59, 59, 0, time.UTC)
	got := SpendAlertPeriod(in)
	want := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("= %s, quiero %s", got, want)
	}

	// Una hora local distinta no debe cambiar de mes: se normaliza a UTC.
	loc := time.FixedZone("UTC+13", 13*3600)
	if got := SpendAlertPeriod(time.Date(2026, 7, 31, 23, 0, 0, 0, loc)); !got.Equal(want) {
		t.Errorf("con zona +13 = %s, quiero %s (mismo mes UTC)", got, want)
	}

	// Y al entrar el mes siguiente, el periodo cambia: ahí se reinician los avisos.
	next := SpendAlertPeriod(time.Date(2026, 8, 1, 0, 0, 1, 0, time.UTC))
	if next.Equal(want) {
		t.Error("agosto debe caer en un periodo distinto de julio")
	}
}
