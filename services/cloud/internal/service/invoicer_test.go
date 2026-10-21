// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"testing"
	"time"
)

// TestLastClosedMonth fija el periodo que factura el emisor: el mes natural
// ANTERIOR en UTC, semiabierto por la derecha. El caso de enero (retroceder de
// año) y el de un instante en zona horaria adelantada son los que se rompen
// solos si alguien cambia el cálculo por aritmética de días.
func TestLastClosedMonth(t *testing.T) {
	madrid := time.FixedZone("CEST", 2*3600)
	casos := []struct {
		nombre string
		ahora  time.Time
		desde  string
		hasta  string
	}{
		{"mes corriente", time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC), "2026-07-01", "2026-08-01"},
		{"primer día del mes", time.Date(2026, 8, 1, 0, 5, 0, 0, time.UTC), "2026-07-01", "2026-08-01"},
		{"enero retrocede de año", time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "2025-12-01", "2026-01-01"},
		{"marzo tras febrero corto", time.Date(2024, 3, 3, 0, 0, 0, 0, time.UTC), "2024-02-01", "2024-03-01"},
		// 01:30 en Madrid es aún el 31 de julio en UTC: el periodo cerrado es junio.
		{"zona adelantada", time.Date(2026, 8, 1, 1, 30, 0, 0, madrid), "2026-06-01", "2026-07-01"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			from, to := lastClosedMonth(c.ahora)
			if got := from.Format("2006-01-02"); got != c.desde {
				t.Errorf("desde = %s, se esperaba %s", got, c.desde)
			}
			if got := to.Format("2006-01-02"); got != c.hasta {
				t.Errorf("hasta = %s, se esperaba %s", got, c.hasta)
			}
			if from.Location() != time.UTC || to.Location() != time.UTC {
				t.Errorf("el periodo debe ir en UTC: %s → %s", from.Location(), to.Location())
			}
			if !from.Before(to) {
				t.Errorf("periodo vacío o invertido: %s → %s", from, to)
			}
		})
	}
}

// TestInvoiceGraceRetrasaElCierre comprueba la perilla de gracia: durante las
// primeras horas del mes el periodo cerrado NO se factura todavía, para no
// congelar un uso que aún está llegando (la factura es idempotente y no se
// corrige sola). Es la condición exacta que evalúa issueClosedPeriod.
func TestInvoiceGraceRetrasaElCierre(t *testing.T) {
	casos := []struct {
		nombre string
		ahora  time.Time
		espera bool // true = todavía dentro de la gracia (no se factura)
	}{
		{"justo al cerrar el mes", time.Date(2026, 8, 1, 0, 0, 1, 0, time.UTC), true},
		{"a las 23 h del día 1", time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC), true},
		{"pasada la gracia", time.Date(2026, 8, 2, 0, 0, 1, 0, time.UTC), false},
		{"a mitad de mes", time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, to := lastClosedMonth(c.ahora)
			if got := c.ahora.UTC().Before(to.Add(invoiceGrace)); got != c.espera {
				t.Errorf("dentro de la gracia = %v, se esperaba %v", got, c.espera)
			}
		})
	}
}
