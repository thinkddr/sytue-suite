// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package trashgc implementa la auto-purga de la papelera de Sytue Fotos: cada
// `interval` destruye definitivamente lo borrado cuya retención venció, y se
// lleva las derivadas que quedan sin dueño.
//
// El worker no sabe purgar —eso es del store, que además es quien puede ser
// cross-tenant— ni sabe de almacenes de objetos: solo despierta, dispara y
// pasa lo purgado al limpiador. Espeja el trashgc de Sytue Drive y el de
// Notepads, con la misma forma, para que las papeleras de la suite se comporten
// igual.
package trashgc

import (
	"context"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// Purger destruye lo caducado. Lo implementa el store.
type Purger interface {
	// PurgeExpiredTrash purga lo que lleve en papelera más de `retention` y
	// devuelve qué se fue.
	PurgeExpiredTrash(ctx context.Context, retention time.Duration) ([]store.Purgada, error)
}

// Limpiador se lleva las derivadas de una foto que ya no existe. Devuelve
// cuántas no pudo borrar; no devuelve error porque un fallo aquí no cambia lo
// que hay que hacer: la fila ya se fue y lo que quede es caché huérfana.
type Limpiador interface {
	BorrarDerivadas(ctx context.Context, tenant string, activo uuid.UUID, huella string) int
}

// Worker purga la papelera caducada cada `interval`.
type Worker struct {
	purger    Purger
	limpiador Limpiador
	interval  time.Duration
	retention time.Duration
}

// NewWorker construye el worker. Un `interval <= 0` se normaliza a una hora;
// una `retention <= 0` deja el worker inerte, y de eso decide `Run`.
func NewWorker(p Purger, l Limpiador, interval, retention time.Duration) *Worker {
	if interval <= 0 {
		interval = time.Hour
	}
	return &Worker{purger: p, limpiador: l, interval: interval, retention: retention}
}

// Run bloquea purgando cada `interval` hasta que se cancele el contexto.
//
// El primer ciclo es INMEDIATO, como en Drive y en Notepads: si el servicio
// estuvo caído tres días, lo caducado durante ese tiempo se recoge al arrancar
// y no en la siguiente hora en punto.
func (w *Worker) Run(ctx context.Context) error {
	// Con la retención apagada no se levanta siquiera el ticker: un worker que
	// despierta cada hora para no hacer nada solo ensucia el log y confunde a
	// quien lo lee buscando por qué no se purga.
	if w.retention <= 0 {
		log.From(ctx).Info("auto-purga de la papelera DESACTIVADA (TRASH_RETENTION=0): nada caduca")
		return nil
	}
	log.From(ctx).Info("worker de auto-purga de la papelera iniciado",
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
	purgadas, err := w.purger.PurgeExpiredTrash(ctx, w.retention)
	if err != nil {
		log.From(ctx).Error("auto-purga de la papelera", "err", err)
		return
	}
	if len(purgadas) == 0 {
		// En régimen normal esto es cero casi siempre, y una línea por hora
		// diciendo «0» entierra las que importan.
		return
	}
	fallos := 0
	if w.limpiador != nil {
		for _, p := range purgadas {
			fallos += w.limpiador.BorrarDerivadas(ctx, p.Tenant, p.ID, p.Huella)
		}
	}
	log.From(ctx).Info("papelera purgada", "fotos", len(purgadas), "derivadas sin borrar", fallos)
}
