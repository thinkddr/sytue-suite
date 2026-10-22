// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package subscriptions

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// Refresher refresca las suscripciones ICS vencidas. Lo implementa el servicio
// (que orquesta claim cross-réplica + fetch SSRF + parseo + sincronización). El
// worker no conoce el detalle: solo lo dispara periódicamente.
type Refresher interface {
	// RefreshDueSubscriptions procesa las suscripciones cuyo refresco venció y
	// devuelve cuántas procesó. Cross-réplica seguro (claim tipo cola con lease).
	RefreshDueSubscriptions(ctx context.Context) (int, error)
}

// Worker refresca los feeds ICS suscritos cada `interval`.
type Worker struct {
	refresher Refresher
	interval  time.Duration
}

// NewWorker construye el worker. Un interval <= 0 se normaliza a 1 minuto.
func NewWorker(r Refresher, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Worker{refresher: r, interval: interval}
}

// Run bloquea refrescando los feeds vencidos cada `interval` hasta que el
// contexto se cancele. El primer ciclo es inmediato (recupera lo vencido durante
// una caída).
func (w *Worker) Run(ctx context.Context) error {
	log.From(ctx).Info("worker de suscripciones ICS iniciado", "interval", w.interval.String())
	w.tick(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	if _, err := w.refresher.RefreshDueSubscriptions(ctx); err != nil {
		log.From(ctx).Warn("refresco de suscripciones ICS falló", "error", err)
	}
}
