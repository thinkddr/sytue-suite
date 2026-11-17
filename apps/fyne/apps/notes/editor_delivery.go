// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
)

// The caller holds the existing cross-process sender lock. Older accepted
// requests finish their local cleanup before a newer draft obtains a CAS input.
func (p *Pantalla) entregarEdicionPrivada(ctx context.Context, id string, send func(context.Context, borradorEdicion, func(context.Context, api.Note) error) (api.Note, error)) error {
	work := p.vida().work
	deliveries, err := work.EditorDeliveries(ctx, id)
	if err != nil {
		return err
	}
	for _, delivery := range deliveries {
		if err := p.reanudarEntregaEdicion(ctx, delivery); err != nil {
			return err
		}
	}
	draft, found, err := work.DurableEditor(ctx, id)
	if errors.Is(err, notas.ErrEditorPending) {
		return nil
	}
	if err != nil || !found {
		return err
	}
	if draft.Blocked {
		return api.ErrDatosCifradosNotas
	}
	if err := draft.Input.ValidarEnlacesCifrados(); err != nil {
		return err
	}
	if !draft.ExpectedUpdatedAt.IsZero() {
		expected := draft.ExpectedUpdatedAt
		draft.Input.ExpectedUpdatedAt = &expected
	}
	delivery, err := work.BeginEditorDelivery(ctx, id, draft)
	if err != nil {
		return err
	}
	acknowledged := false
	_, err = send(ctx, draft, func(ctx context.Context, updated api.Note) error {
		if err := work.MarkEditorDelivery(ctx, delivery, updated); err != nil {
			return err
		}
		acknowledged = true
		return nil
	})
	if err != nil {
		return err
	}
	if !acknowledged {
		return api.ErrPrivateNotesOperation
	}
	return work.FinishEditorDelivery(ctx, delivery)
}

func (p *Pantalla) reanudarEntregaEdicion(ctx context.Context, delivery notas.EditorDelivery) error {
	draft := delivery.Draft
	if draft.Blocked {
		return api.ErrDatosCifradosNotas
	}
	if err := draft.Input.ValidarEnlacesCifrados(); err != nil {
		return err
	}
	if !draft.ExpectedUpdatedAt.IsZero() {
		expected := draft.ExpectedUpdatedAt
		draft.Input.ExpectedUpdatedAt = &expected
	}
	acknowledge := func(ctx context.Context, updated api.Note) error {
		return p.vida().work.MarkEditorDelivery(ctx, delivery, updated)
	}
	_, found, err := p.svc.ResumeNoteAcknowledged(ctx, draft.NoteID, draft.NotebookID, draft.Input, acknowledge)
	if err != nil {
		return err
	}
	if !found && delivery.AcceptedAt.IsZero() {
		if _, err := p.svc.UpdateNoteAcknowledged(ctx, draft.NoteID, draft.NotebookID, draft.Input, acknowledge); err != nil {
			return err
		}
	}
	return p.vida().work.FinishEditorDelivery(ctx, delivery)
}

func (p *Pantalla) enviarBorradorPrivado(ctx context.Context, draft borradorEdicion, acknowledge func(context.Context, api.Note) error) (api.Note, error) {
	return p.svc.UpdateNoteAcknowledged(ctx, draft.NoteID, draft.NotebookID, draft.Input, acknowledge)
}
