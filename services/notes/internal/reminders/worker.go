// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package reminders es el worker de recordatorios de Sytue Notepads: cada `interval`
// vacía la cola de lo vencido (el store la reclama con FOR UPDATE SKIP LOCKED en todas
// las organizaciones) y avisa a cada persona en el Hub con el enlace a su nota. Tiene
// la forma del de Calendar (`services/calendar/internal/reminders`).
//
// El aviso llega a la campana y, en el teléfono, por el flujo del Hub mientras la app
// está abierta: no hay alarma del sistema, y la interfaz no la promete.
package reminders

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// Lote es cuántos se reclaman por transacción.
const Lote = 200

// Tipos de aviso: con título, y sin él (nota cifrada o sin título), que no puede decir
// de qué nota se trata. Los dos tienen su `notifications.kind.*` en el catálogo común.
const (
	TipoAviso       = "notes.reminder.due"
	TipoAvisoSinNom = "notes.reminder.untitled"
)

// Reclamador es lo que el worker necesita del store.
type Reclamador interface {
	ReclamarRecordatorios(ctx context.Context, ahora time.Time, limite int, avisar func(store.RecordatorioVencido)) (int, error)
}

// Worker vacía la cola cada `interval`.
type Worker struct {
	st        Reclamador
	interval  time.Duration
	hub       *hubnotify.Client
	publicURL string
	// unaVez es el freno de repetidos: si una transacción avisa y luego no confirma, la
	// fila vuelve a salir; el mismo vencimiento no se avisa dos veces en la ventana.
	unaVez *hubnotify.UnaVez
	ahora  func() time.Time
}

// NewWorker construye el worker. Un interval <= 0 se normaliza a un minuto.
func NewWorker(st Reclamador, interval time.Duration, hub *hubnotify.Client, publicURL string) *Worker {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Worker{
		st: st, interval: interval, hub: hub, publicURL: publicURL,
		unaVez: hubnotify.NuevaUnaVez(time.Hour), ahora: time.Now,
	}
}

// Run vacía la cola cada `interval` hasta que se cancele el contexto. El primer ciclo
// es inmediato: lo vencido durante una caída se avisa al arrancar.
func (w *Worker) Run(ctx context.Context) error {
	log.From(ctx).Info("worker de recordatorios de notas iniciado", "interval", w.interval.String())
	w.Vaciar(ctx)
	tic := time.NewTicker(w.interval)
	defer tic.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tic.C:
			w.Vaciar(ctx)
		}
	}
}

// Vaciar reclama lotes hasta que no quede nada vencido.
func (w *Worker) Vaciar(ctx context.Context) {
	for {
		n, err := w.st.ReclamarRecordatorios(ctx, w.ahora(), Lote, func(v store.RecordatorioVencido) { w.avisar(ctx, v) })
		if err != nil {
			log.From(ctx).Warn("vaciar recordatorios de notas", "error", err)
			return
		}
		if n < Lote {
			return
		}
	}
}

// Aviso es el aviso de un recordatorio vencido. La hora viaja en `start` y no escrita:
// aquí solo se sabe en UTC, y cada cliente la pone en su zona.
func Aviso(publicURL string, v store.RecordatorioVencido) hubnotify.Notification {
	n := hubnotify.Notification{
		Tenant: v.Tenant, Subject: v.OwnerSub, Source: "notes", Type: TipoAvisoSinNom, Level: "info",
		// El respaldo de quien no tenga la clave en su catálogo.
		Title:  "Recordatorio de nota",
		URL:    hubnotify.EnlaceNota(publicURL, v.Tenant, v.NotebookID.String(), v.NoteID.String()),
		Params: map[string]string{"start": v.DueAt.UTC().Format(time.RFC3339)},
	}
	// Quien la lee solo porque se la compartieron no ve su cuaderno: se abre en
	// «Compartido conmigo».
	if v.SoloCompartida {
		n.URL = hubnotify.EnlaceNotaCompartida(publicURL, v.Tenant, v.NoteID.String())
	}
	if v.Title != "" {
		n.Type, n.Title = TipoAviso, hubnotify.RecortarTitulo("Recordatorio: "+v.Title)
		n.Params["title"] = hubnotify.RecortarTitulo(v.Title)
	}
	return n
}

func (w *Worker) avisar(ctx context.Context, v store.RecordatorioVencido) {
	if w.hub == nil || v.OwnerSub == "" ||
		!w.unaVez.Primera(v.Tenant+"|"+v.OwnerSub+"|"+v.NoteID.String()+"|"+v.DueAt.UTC().Format(time.RFC3339)) {
		return
	}
	w.hub.SendAsync(ctx, Aviso(w.publicURL, v))
}
