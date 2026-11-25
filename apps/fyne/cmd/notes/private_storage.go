// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"math"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	privateNotes "git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

type notesRepository interface {
	api.NotesData
	api.NotesViewsData
	api.NotesExportData
	cifrado.RecordApplicationRepository
}

// One registered owner orders the shared record engine and Notes' disjoint local
// draft namespaces. The presenter cannot resume until its repository has resumed.
// Neither component registers or counts the other's journal entries again.
type notepadsRepository struct {
	notesRepository
	local *notas.LocalWork
	life  context.Context
}

func (r *notepadsRepository) Pending(ctx context.Context) (offline.PendingWork, error) {
	result, err := r.notesRepository.Pending(ctx)
	if err != nil {
		return result, err
	}
	local, err := r.local.Pending(ctx)
	if err != nil {
		return result, err
	}
	scope := r.PrivateStorageScope()
	if result.Scope != scope || local.Scope != scope || result.Count < 0 || local.Count < 0 || local.Count > math.MaxInt-result.Count {
		return offline.PendingWork{Scope: scope}, cifrado.ErrPrivateResources
	}
	result.Count += local.Count
	return result, nil
}

func (r *notepadsRepository) Drain(ctx context.Context) error {
	if err := r.local.Drain(ctx); err != nil {
		return err
	}
	return r.notesRepository.Drain(ctx)
}

func (r *notepadsRepository) Resume() error {
	if err := r.notesRepository.Resume(); err != nil {
		return err
	}
	if err := r.local.Resume(); err != nil {
		return errors.Join(err, r.notesRepository.Drain(r.life))
	}
	return nil
}

// The RepositoryCycle has already joined and validated the sole owner's Pending.
// Closing retains every durable source in the existing encrypted journal.
func (r *notepadsRepository) Close() { r.local.Close(); r.notesRepository.Close() }

func notesPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], suite *sesion.Suite) (cifrado.ApplicationPlan[*notepadsRepository], error) {
	if resources == nil || suite == nil {
		return cifrado.ApplicationPlan[*notepadsRepository]{}, cifrado.ErrPrivateResources
	}
	source, err := privateNotes.NewHTTPSource(api.NewNotes(suite.URL("notes"), resources.HTTP(), nil))
	if err != nil {
		return cifrado.ApplicationPlan[*notepadsRepository]{}, err
	}
	return buildNotesPlan(ctx, resources, source, api.NewDrive(suite.URL("drive"), resources.HTTP(), nil), api.NewRooms(suite.URL("rooms"), resources.HTTP(), nil), suite.AvisarEscritura)
}

func buildNotesPlan(ctx context.Context, resources *cifrado.Resources[*mx.Client], source records.Source, gateway *api.Drive, service transfer.CutoverService, onWrite func()) (cifrado.ApplicationPlan[*notepadsRepository], error) {
	var empty cifrado.ApplicationPlan[*notepadsRepository]
	if resources == nil {
		return empty, cifrado.ErrPrivateResources
	}
	if err := resources.Check(ctx); err != nil {
		return empty, err
	}
	return cifrado.NewRecordApplicationPlan(ctx, resources, cifrado.RecordApplicationConfig[*notepadsRepository]{
		Gateway: gateway, Source: source, Service: service, Historical: privateNotes.NewHistoricalSource, OnWrite: onWrite,
		New: func(cfg records.Config) (*notepadsRepository, error) { //nolint:contextcheck // A failed constructor closes only new, unregistered components before any IO was admitted.
			repository, err := privateNotes.New(cfg)
			if err != nil {
				return nil, err
			}
			local, err := notas.NewLocalWork(resources.Context(), resources.PrivateStorageScope(), resources.Journal, resources.WorkLease())
			if err != nil {
				repository.Close()
				return nil, err
			}
			return &notepadsRepository{notesRepository: repository, local: local, life: resources.Context()}, nil
		},
	})
}
