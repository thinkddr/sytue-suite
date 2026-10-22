// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// El cálculo de compute_gb_hours: réplicas × memoria(GiB) × horas de la ventana,
// con redondeo a 6 decimales y saneo de entradas no positivas.
func TestComputeGBHours(t *testing.T) {
	cases := []struct {
		name     string
		replicas int
		memGiB   float64
		window   time.Duration
		want     float64
	}{
		{"1 réplica media hora", 1, 0.5, 30 * time.Minute, 0.25},  // 1×0.5×0.5
		{"3 réplicas una hora", 3, 0.5, time.Hour, 1.5},           // 3×0.5×1
		{"2 réplicas 90m", 2, 0.5, 90 * time.Minute, 1.5},         // 2×0.5×1.5
		{"redondeo a 6 decimales", 1, 0.5, time.Second, 0.000139}, // 0.5×(1/3600)=0.0001388..→0.000139
		{"ventana cero", 1, 0.5, 0, 0},
		{"ventana negativa", 1, 0.5, -time.Hour, 0},
		{"réplicas cero", 0, 0.5, time.Hour, 0},
		{"memoria cero", 1, 0, time.Hour, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeGBHours(tc.replicas, tc.memGiB, tc.window); got != tc.want {
				t.Errorf("computeGBHours(%d, %v, %v) = %v; want %v",
					tc.replicas, tc.memGiB, tc.window, got, tc.want)
			}
		})
	}
}

// reconcileCompute traduce cada ventana reclamada en un registro compute_gb_hours
// con la cantidad calculada y el periodo [Since, now).
func TestReconcileComputeEmitsUsage(t *testing.T) {
	pid := uuid.New()
	since := time.Now().UTC().Add(-time.Hour)
	fs := &fakeCtrlStore{computeWindows: []store.ComputeUsage{
		{Tenant: "acme", ProjectID: pid, Replicas: 2, Since: since},
	}}
	c := newTestController(fs, &recPlatform{}, nil)

	if err := c.reconcileCompute(context.Background()); err != nil {
		t.Fatalf("reconcileCompute: %v", err)
	}
	if len(fs.usage) != 1 {
		t.Fatalf("esperaba 1 registro de uso; got %d", len(fs.usage))
	}
	u := fs.usage[0]
	if u.Metric != "compute_gb_hours" || u.Tenant != "acme" || u.Unit != "gb_hours" {
		t.Errorf("registro inesperado: %+v", u)
	}
	if u.ProjectID == nil || *u.ProjectID != pid {
		t.Errorf("project_id = %v; want %v", u.ProjectID, pid)
	}
	if !u.PeriodFrom.Equal(since) {
		t.Errorf("period_from = %v; want %v (Since de la ventana)", u.PeriodFrom, since)
	}
	// 2 réplicas × 0.5 GiB × ~1h ≈ 1.0 gb_hours (la ventana real es 1h + epsilon).
	if u.Quantity < 0.99 || u.Quantity > 1.02 {
		t.Errorf("cantidad = %v; esperaba ≈1.0 (2×0.5×1h)", u.Quantity)
	}
}

// Una ventana que calcula 0 (p. ej. réplicas 0) no emite registro.
func TestReconcileComputeSkipsZero(t *testing.T) {
	fs := &fakeCtrlStore{computeWindows: []store.ComputeUsage{
		{Tenant: "acme", ProjectID: uuid.New(), Replicas: 0, Since: time.Now().Add(-time.Hour)},
	}}
	c := newTestController(fs, &recPlatform{}, nil)
	if err := c.reconcileCompute(context.Background()); err != nil {
		t.Fatalf("reconcileCompute: %v", err)
	}
	if len(fs.usage) != 0 {
		t.Errorf("ventana con cantidad 0 no debe emitir uso; got %+v", fs.usage)
	}
}
