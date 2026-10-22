// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package reminders implementa el worker de avisos de Sytue Calendar: drena
// periódicamente la cola calendar_reminders (disparos de VALARM ya precalculados
// por el store) y publica un evento de dominio `calendar.reminder.due` en el
// outbox (bus) por cada recordatorio vencido. El consumidor del bus (Hub /
// notificaciones) es quien entrega el aviso al usuario; este worker solo decide
// CUÁNDO y publica, sin acoplarse a ningún canal concreto.
package reminders

import (
	"context"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// channel y eventType identifican el aviso publicado (mismo canal lógico que el
// resto de eventos de Calendar).
const (
	channel events.Channel = "calendar"
	// eventType es el 2.º arg (string) de events.NewEvent, no un canal.
	eventType = "calendar.reminder.due"
)

// batchSize acota cuántos recordatorios se reclaman por transacción.
const batchSize = 200

// Worker drena la cola de recordatorios cada `interval`.
type Worker struct {
	store       *store.Store
	interval    time.Duration
	hubNotifier *hubnotify.Client
	publicURL   string
}

// NewWorker construye el worker. Un interval <= 0 se normaliza a 1 minuto.
func NewWorker(st *store.Store, interval time.Duration, notifier *hubnotify.Client, publicURL string) *Worker {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Worker{
		store:       st,
		interval:    interval,
		hubNotifier: notifier,
		publicURL:   publicURL,
	}
}

// Run bloquea drenando la cola cada `interval` hasta que el contexto se cancele.
// El primer drenaje es inmediato (recupera lo vencido durante una caída).
func (w *Worker) Run(ctx context.Context) error {
	log.From(ctx).Info("worker de recordatorios iniciado", "interval", w.interval.String())
	w.drain(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.drain(ctx)
		}
	}
}

// drain reclama y publica lotes hasta vaciar los recordatorios vencidos.
func (w *Worker) drain(ctx context.Context) {
	for {
		n, err := w.store.ClaimDueReminders(ctx, time.Now(), batchSize, w.publish)
		if err != nil {
			log.From(ctx).Warn("drenaje de recordatorios falló", "error", err)
			return
		}
		if n < batchSize {
			return // vaciado
		}
	}
}

// publish escribe el aviso en el outbox usando la MISMA tx que marca el
// recordatorio como disparado (at-least-once: el aviso solo viaja si la tx hace
// commit) y emite la notificación a Sytue Hub.
func (w *Worker) publish(ctx context.Context, q pg.Querier, r store.DueReminder) error {
	ev, err := events.NewEvent(channel, eventType, r.Tenant, map[string]any{
		"reminder_id": r.ID,
		"owner_sub":   r.OwnerSub,
		"event_id":    r.EventID,
		"uid":         r.UID,
		"summary":     r.Summary,
		"dtstart":     r.DTStart,
		"trigger_at":  r.TriggerAt,
		"action":      r.Action,
	})
	if err != nil {
		return err
	}
	if err := events.Publish(ctx, q, ev); err != nil {
		return err
	}

	if w.hubNotifier != nil && r.OwnerSub != "" {
		title := "Recordatorio: " + r.Summary
		if strings.TrimSpace(r.Summary) == "" {
			title = "Recordatorio de evento"
		}
		// La hora NO va escrita: aquí solo se sabe en UTC, y un «10:00» de otra zona es
		// peor que ninguno. Viaja en `start` y cada cliente la pone en la suya.
		w.hubNotifier.SendAsync(ctx, hubnotify.Notification{
			Tenant:  r.Tenant,
			Subject: r.OwnerSub,
			Source:  "calendar",
			Type:    "calendar.reminder.due",
			Level:   "info",
			Title:   title,
			URL:     hubnotify.EnlaceEvento(w.publicURL, r.Tenant, r.EventID.String()),
			Params:  map[string]string{"title": r.Summary, "start": r.DTStart.UTC().Format(time.RFC3339)},
		})
	}

	return nil
}
