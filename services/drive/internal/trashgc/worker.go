// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package trashgc implementa el worker de AUTO-PURGA de papelera de Sytue Drive:
// cada `interval` borra DEFINITIVAMENTE los elementos en papelera cuya retención
// venció (deleted_at < now()-retención). El worker no conoce el detalle —claim
// cross-réplica, purga por tenant, GC de blobs huérfanos—: solo dispara al
// servicio periódicamente. Cross-réplica seguro (varias instancias reclaman con
// FOR UPDATE SKIP LOCKED sin purgar lo mismo dos veces).
package trashgc

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// Purger purga los elementos de la papelera cuya retención venció. Lo implementa el
// servicio (que orquesta claim cross-réplica + purga por tenant + GC de blobs).
type Purger interface {
	// PurgeExpiredTrash purga lo caducado y devuelve cuántos elementos purgó.
	PurgeExpiredTrash(ctx context.Context) (int, error)
}

// Worker purga la papelera caducada cada `interval`.
type Worker struct {
	purger   Purger
	interval time.Duration
}

// NewWorker construye el worker. Un interval <= 0 se normaliza a 1 hora.
func NewWorker(p Purger, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = time.Hour
	}
	return &Worker{purger: p, interval: interval}
}

// Run bloquea purgando la papelera caducada cada `interval` hasta que el contexto
// se cancele. El primer ciclo es inmediato (recupera lo caducado durante una caída).
func (w *Worker) Run(ctx context.Context) error {
	log.From(ctx).Info("worker de auto-purga de papelera iniciado", "interval", w.interval.String())
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
	n, err := w.purger.PurgeExpiredTrash(ctx)
	if err != nil {
		log.From(ctx).Warn("auto-purga de papelera falló", "error", err)
		return
	}
	if n > 0 {
		log.From(ctx).Info("auto-purga de papelera", "purgados", n)
	}
}
