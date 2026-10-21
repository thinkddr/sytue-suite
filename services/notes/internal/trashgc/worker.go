// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package trashgc implementa la auto-purga de la papelera de Sytue Notepads: cada
// `interval` destruye definitivamente lo borrado cuya retención venció. El worker
// no sabe purgar —eso es del store, que además es quien puede ser cross-tenant—:
// solo despierta y dispara.
//
// Espeja el trashgc de Sytue Drive, con la misma forma y el mismo interfaz
// estrecho, para que las dos papeleras de la suite se comporten igual.
package trashgc

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// Purger destruye lo caducado. Lo implementa el store.
type Purger interface {
	// PurgeExpiredTrash purga lo que lleve en papelera más de `retention` y
	// devuelve cuántas filas se fueron.
	PurgeExpiredTrash(ctx context.Context, retention time.Duration) (int64, error)
}

// Worker purga la papelera caducada cada `interval`.
type Worker struct {
	purger    Purger
	interval  time.Duration
	retention time.Duration
}

// NewWorker construye el worker. Un `interval <= 0` se normaliza a una hora; una
// `retention <= 0` deja el worker inerte, y de eso decide `Run`.
func NewWorker(p Purger, interval, retention time.Duration) *Worker {
	if interval <= 0 {
		interval = time.Hour
	}
	return &Worker{purger: p, interval: interval, retention: retention}
}

// Run bloquea purgando cada `interval` hasta que se cancele el contexto.
//
// El primer ciclo es INMEDIATO, como en Drive: si el servicio estuvo caído tres
// días, lo caducado durante ese tiempo se recoge al arrancar y no en la siguiente
// hora en punto.
func (w *Worker) Run(ctx context.Context) error {
	// Con la retención apagada no se levanta siquiera el ticker: un worker que
	// despierta cada hora para no hacer nada solo ensucia el log y confunde a
	// quien lo lee buscando por qué no se purga.
	if w.retention <= 0 {
		log.From(ctx).Info("auto-purga de papelera DESACTIVADA (TRASH_RETENTION=0): nada caduca")
		return nil
	}
	log.From(ctx).Info("worker de auto-purga de papelera iniciado",
		"interval", w.interval.String(), "retention", w.retention.String())
	w.tick(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

// tick purga una vez. Un fallo se registra y NO tumba el worker: la papelera es
// una tarea de fondo, y que Postgres esté ocupado un minuto no es razón para
// dejar de intentarlo hasta el siguiente reinicio del servicio.
func (w *Worker) tick(ctx context.Context) {
	n, err := w.purger.PurgeExpiredTrash(ctx, w.retention)
	if err != nil {
		log.From(ctx).Error("auto-purga de papelera", "err", err)
		return
	}
	// Solo se anota cuando hubo algo: en régimen normal esto son cero filas casi
	// siempre, y una línea por hora diciendo «0» entierra las que importan.
	if n > 0 {
		log.From(ctx).Info("papelera purgada", "filas", n)
	}
}
