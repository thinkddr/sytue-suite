// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type privateDraftProvider interface {
	PrivateDraftRevisions() (api.PrivateMailDraftRevisions, bool)
}

type privateDraftEditor struct {
	port                                  api.PrivateMailDraftRevisions
	editor                                string
	generation, savedGeneration           uint64
	saved                                 bool
	pending                               *privateDraftRevision
	closeAfter, discardEdits, notifySaved bool
	next                                  *Borrador
	outgoing                              *privateOutgoingEditor
}

type privateDraftRevision struct {
	input      api.MailDraftRevision
	attempt    api.MailDraftAttempt
	prepared   bool
	generation uint64
}

type privateDraftResult struct {
	attempt  api.MailDraftAttempt
	prepared bool
	message  api.MailMessage
}

func privateDraftFor(service Servicio) *privateDraftEditor {
	provider, ok := service.(privateDraftProvider)
	if !ok {
		return nil
	}
	port, active := provider.PrivateDraftRevisions()
	if !active {
		return nil
	}
	return &privateDraftEditor{port: port, editor: uuid.NewString()}
}

func (b *Buzon) restaurarBorradorPrivado(state *privateDraftEditor) {
	if state == nil || b.redaccion == nil {
		return
	}
	// Keep A's original revision while renewing only its private owner port.
	state.port = nil
	if current := b.redaccion.privado; current != nil {
		state.port = current.port
	}
	b.redaccion.privado = state
}

// guardarBorradorPrivado keeps one composed MIME and Message-ID until its exact
// revision is acknowledged. Edits made while waiting are a later revision.
func (b *Buzon) guardarBorradorPrivado(rd *redaccion, silencioso, yCerrar bool) {
	state := rd.privado
	if state != nil && state.outgoing != nil {
		b.reintentarEnvioPrivado(rd)
		return
	}
	if state == nil || !b.redaccionVigente(rd) || rd.enviando {
		return
	}
	state.closeAfter = state.closeAfter || yCerrar
	state.notifySaved = state.notifySaved || yCerrar
	if rd.guardando {
		return
	}
	if state.port == nil {
		b.avisar(ui.AvisoPeligro, "compose.draftFailed", nil)
		return
	}
	if state.pending == nil {
		if state.saved && state.savedGeneration == state.generation {
			b.terminarBorradorPrivado(rd)
			return
		}
		de, _, raw, err := b.componer(rd.r.B, true)
		if err != nil {
			if !silencioso || state.closeAfter {
				b.avisar(ui.AvisoPeligro, "compose.draftFailed", nil)
			}
			return
		}
		state.pending = &privateDraftRevision{input: api.MailDraftRevision{Editor: state.editor, Revision: uuid.NewString(), ID: rd.r.B.ID, From: de, MIME: raw}, generation: state.generation}
	}
	pending := state.pending
	port := state.port
	input := pending.input
	input.MIME = bytes.Clone(input.MIME)
	attempt, prepared := pending.attempt, pending.prepared
	rd.guardando = true
	b.pintarBotonesRedactor(rd)
	pedir(b, func(ctx context.Context) (privateDraftResult, error) {
		result := privateDraftResult{attempt: attempt, prepared: prepared}
		defer clear(input.MIME)
		if !result.prepared {
			var err error
			result.attempt, err = port.PrepareDraftRevision(ctx, input)
			if err != nil {
				return result, err
			}
			result.prepared = true
		}
		var err error
		result.message, err = port.CommitDraftRevision(ctx, result.attempt)
		return result, err
	}, func(result privateDraftResult, err error) {
		b.resultadoBorradorPrivado(rd, pending, result, err, silencioso)
	})
}

func (b *Buzon) resultadoBorradorPrivado(rd *redaccion, pending *privateDraftRevision, result privateDraftResult, err error, silencioso bool) {
	state := rd.privado
	rd.guardando = false
	if result.prepared {
		pending.prepared = true
		pending.attempt = result.attempt
		clear(pending.input.MIME)
		pending.input.MIME = nil
	}
	b.pintarBotonesRedactor(rd)
	if err != nil {
		if !silencioso || state.closeAfter {
			b.avisar(ui.AvisoPeligro, "compose.draftFailed", nil)
		}
		return
	}
	rd.r.B.ID = result.message.ID
	state.saved, state.savedGeneration = true, pending.generation
	state.pending = nil
	b.alGuardarBorrador()
	if !state.discardEdits && state.generation != state.savedGeneration {
		b.guardarBorradorPrivado(rd, silencioso, false)
		return
	}
	b.terminarBorradorPrivado(rd)
}

func (b *Buzon) terminarBorradorPrivado(rd *redaccion) {
	state := rd.privado
	if state == nil || !state.closeAfter {
		return
	}
	if state.notifySaved {
		b.avisar(ui.AvisoExito, "compose.draftSaved", nil)
	}
	next := state.next
	b.cerrar(rd)
	if next != nil {
		b.abrirRedactor(*next)
	}
}

// cerrarBorradorPrivado waits for the retained attempt. An explicit footer
// discard may drop newer unadmitted edits while preserving the earlier attempt.
func (b *Buzon) cerrarBorradorPrivado(rd *redaccion, discard bool) {
	state := rd.privado
	if state != nil && state.outgoing != nil {
		if discard {
			b.descartarEnvioPrivado(rd)
		} else {
			b.reintentarEnvioPrivado(rd)
		}
		return
	}
	if state == nil || !b.redaccionVigente(rd) {
		return
	}
	if !rd.r.B.ConContenido() && rd.r.B.ID == "" && state.pending == nil {
		state.closeAfter = true
		b.terminarBorradorPrivado(rd)
		return
	}
	state.closeAfter = true
	state.discardEdits = discard
	b.guardarBorradorPrivado(rd, false, false)
}
