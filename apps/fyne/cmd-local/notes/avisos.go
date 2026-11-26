// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"log/slog"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/local/avisos"
	privateNotes "git.sytue.eu/sytue/one/apps/nucleo/notes"
)

// Los recordatorios de las notas los entrega el SISTEMA a su hora (PLAN §5.12): al
// abrir, y cada cuarto de hora con la app abierta, se programa el próximo de cada nota
// (`ListReminders` ya da el siguiente de los que repiten) con `local.App.Recordar`.

const cadaRecordar = 15 * time.Minute

func recordar(ctx context.Context, l *local.App, repo *privateNotes.Repository) {
	programar := func() {
		lista, err := avisosDeNotas(ctx, repo)
		if err == nil {
			err = l.Recordar("notes", lista)
		}
		if err != nil {
			slog.Warn("notes: no se pudieron programar los recordatorios", "error", err)
		}
	}
	go func() {
		programar()
		t := time.NewTicker(cadaRecordar)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				programar()
			}
		}
	}()
}

func avisosDeNotas(ctx context.Context, repo *privateNotes.Repository) ([]avisos.Aviso, error) {
	rs, err := repo.ListReminders(ctx)
	if err != nil {
		return nil, err
	}
	titulos := map[string]string{}
	vistos := map[string]bool{}
	var out []avisos.Aviso
	for _, r := range rs {
		if !vistos[r.NotebookID] {
			vistos[r.NotebookID] = true
			if notas, err := repo.ListNotes(ctx, r.NotebookID); err == nil {
				for _, n := range notas {
					titulos[n.ID] = n.Title
				}
			}
		}
		// El mismo que avisa el Hub con la app cerrada.
		out = append(out, avisos.DeNota(r.NoteID, titulos[r.NoteID], r.DueAt, r.AllDay))
	}
	return out, nil
}
