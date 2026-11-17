// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

const prefijoEdicion = "edicion:"

var (
	errBorradorEdicion = errors.New("notes: an editor draft cannot be authenticated")
	errBorradorUTF8    = errors.New("notes: editor text contains invalid UTF-8")
)

type borradorEdicion = notas.EditorDraft

//nolint:gocognit,gocyclo // una rama por destino del borrador (compartida, sin almacén, almacén sin cifrar, diario durable), cada una con su bloqueo.
func (p *Pantalla) conservarEdicion(s *sesionEdicion, html string) error {
	if !utf8.ValidString(html) {
		s.bloqueoBorrador = true
		s.errorBorrador = errBorradorUTF8
		s.borrador = nil
		s.durable = nil
		return errBorradorUTF8
	}
	if s.compartida {
		s.bloqueoBorrador = false
		s.errorBorrador = nil
		return nil
	}
	if p.Almacen == nil && !p.activo.Cifrado {
		s.bloqueoBorrador = false
		s.errorBorrador = nil
		return nil
	}
	if p.Almacen != nil && p.vida().work == nil && !p.Almacen.Encrypted() {
		if p.activo.Cifrado {
			s.bloqueoBorrador = true
			s.errorBorrador = offline.ErrEncryptedStore
		}
		return offline.ErrEncryptedStore
	}
	n, ok := p.nota(s.nota)
	if !ok {
		if p.activo.Cifrado {
			s.bloqueoBorrador = true
			s.errorBorrador = errBorradorEdicion
			s.borrador = nil
			s.durable = nil
		}
		return errBorradorEdicion
	}
	pa := Parche{Contenido: &html}
	if !p.activo.Cifrado {
		pa.Enlaces = enlacesSincronizados(n.Crudo, s.editor.Documento(), p.AppURL)
	}
	in, err := p.entradaEditada(n, pa)
	// If encryption fails, the separately retained HTML is still recoverable
	// from the encrypted local journal. Never replay the original input as if
	// it contained the new text.
	blocked := err != nil && p.activo.Cifrado
	blockingErr := err
	if blocked && p.Almacen == nil {
		s.bloqueoBorrador = true
		s.errorBorrador = offline.ErrEncryptedStore
		return s.errorBorrador
	}
	if err != nil && !blocked {
		return err
	}
	if blocked {
		// Preserve the edited HTML only in the encrypted local journal. The
		// original input is never replayed while the migration gate is absent.
		in = n.Crudo.Input()
		in.ExpectedCrypto = p.activo.Crudo.Crypto
	}
	if p.Almacen == nil {
		return nil
	}
	if s.borradorID == "" {
		s.borradorID = prefijoEdicion + rand.Text()
	}
	draft := &borradorEdicion{Version: 2, NoteID: s.nota, NotebookID: n.Cuaderno, Revision: rand.Text(), HTML: html, Input: in, ExpectedUpdatedAt: n.Crudo.UpdatedAt, Blocked: blocked}
	if blocked {
		// Older clients reject this version instead of replaying the original
		// input and deleting the separately retained edited HTML.
		draft.Version = 3
	}
	if !blocked && in.Content == html && in.ContentCipher == "" && in.CipherVersion == 0 && len(in.ExpectedCrypto) == 0 {
		draft.HTML = ""
		draft.FromInput = true
	}
	s.borrador = draft
	s.bloqueoBorrador = blocked
	ctx, _ := p.contextoTrabajo()
	if work := p.vida().work; work != nil {
		receipt, err := work.QueueEditor(ctx, s.borradorID, *draft)
		s.errorBorrador = err
		if err != nil {
			s.durable = nil
			return err
		}
		s.durable = receipt

		p.observarEdicionDurable(ctx, s, receipt)
		if blocked {
			return blockingErr
		}
		return nil
	}
	s.errorBorrador = p.escribirEdicion(ctx, s.borradorID, *draft)
	if p.vida().work == nil && s.errorBorrador != nil {
		if p.erroresEdiciones == nil {
			p.erroresEdiciones = map[string]borradorEdicion{}
		}
		p.erroresEdiciones[s.borradorID] = *draft
	} else {
		delete(p.erroresEdiciones, s.borradorID)
	}
	if s.errorBorrador == nil && blocked {
		return blockingErr
	}
	return s.errorBorrador
}

// The notification only schedules autosave after the original source is durable;
// it never publishes from the IO worker or revives an old presenter phase.
func (p *Pantalla) observarEdicionDurable(ctx context.Context, s *sesionEdicion, receipt *notas.DraftReceipt) {
	p.iniciar(func() {
		select {
		case <-receipt.Done():
		case <-ctx.Done():
			return
		}
		durable, err := receipt.Status()
		p.enHilo(func() { //nolint:contextcheck // Rendering schedules future autosave with the retained presenter phase.
			if s.borrador == nil || s.borrador.Revision != receipt.Revision {
				return
			}
			s.errorBorrador = err
			if err != nil {
				p.fallo("note.saveFailed", err)
				return
			}
			if durable && s.esperaDurable {
				p.guardarEdicion(s)
			}
		})
	})
}

func (p *Pantalla) escribirEdicion(ctx context.Context, id string, d borradorEdicion) error {
	if !d.Valid() || !strings.HasPrefix(id, prefijoEdicion) {
		return errBorradorEdicion
	}
	return p.escribirLocal(ctx, espacioBorradores, id, d)
}

func (p *Pantalla) leerEdicion(id string) (borradorEdicion, bool, error) {
	var d borradorEdicion
	found, err := p.Almacen.Read(p.claveAlmacen(), espacioBorradores, id, &d)
	if err == nil && found && !d.Valid() {
		err = errBorradorEdicion
	}
	return d, found, err
}

// Acknowledging an older revision never removes newer keystrokes. If this exact
// write succeeded, a later edit may advance only to its returned version, keeping
// the optimistic precondition against concurrent remote edits.
func (p *Pantalla) confirmarEdicion(ctx context.Context, id string, sent borradorEdicion, updated api.Note) error {
	if work := p.vida().work; work != nil {
		return work.AcknowledgeEditor(ctx, id, sent, updated)
	}
	if updated.ID != sent.NoteID || updated.NotebookID != sent.NotebookID {
		return errBorradorEdicion
	}
	return p.Almacen.Exclusive(ctx, p.claveAlmacen(), espacioBorradores, id, func() error {
		current, found, err := p.leerEdicion(id)
		if err != nil || !found {
			return err
		}
		if current.NoteID != sent.NoteID || current.NotebookID != sent.NotebookID {
			return errBorradorEdicion
		}
		if current.Revision == sent.Revision {
			return p.Almacen.Delete(p.claveAlmacen(), espacioBorradores, id)
		}
		if current.ExpectedUpdatedAt.Equal(sent.ExpectedUpdatedAt) && !updated.UpdatedAt.IsZero() {
			current.ExpectedUpdatedAt = updated.UpdatedAt
			return p.Almacen.Write(p.claveAlmacen(), espacioBorradores, id, current)
		}
		return nil
	})
}

func (p *Pantalla) enviarEdicion(ctx context.Context, id string, send func(context.Context, borradorEdicion) (api.Note, error)) error {
	if p.vida().work != nil {
		return api.ErrPrivateNotesOperation
	}
	return p.enviarEdicionConfirmada(ctx, id, func(ctx context.Context, d borradorEdicion, _ func(context.Context, api.Note) error) (api.Note, error) {
		return send(ctx, d)
	})
}

func (p *Pantalla) enviarEdicionConfirmada(ctx context.Context, id string, send func(context.Context, borradorEdicion, func(context.Context, api.Note) error) (api.Note, error)) error {
	// This separate sender lock never blocks a keystroke's short record lock on IO.
	return p.Almacen.Exclusive(ctx, p.claveAlmacen(), espacioBorradores, id+":envio", func() error {
		if p.vida().work != nil {
			return p.entregarEdicionPrivada(ctx, id, send)
		}
		d, found, err := p.edicionParaEnvio(ctx, id)
		if errors.Is(err, notas.ErrEditorPending) {
			return nil
		}
		if err != nil || !found {
			return err
		}
		if d.Blocked {
			return api.ErrDatosCifradosNotas
		}
		if err := d.Input.ValidarEnlacesCifrados(); err != nil {
			return err
		}
		if !d.ExpectedUpdatedAt.IsZero() {
			expected := d.ExpectedUpdatedAt
			d.Input.ExpectedUpdatedAt = &expected
		}
		acknowledge := func(ctx context.Context, updated api.Note) error { return p.confirmarEdicion(ctx, id, d, updated) }
		updated, err := send(ctx, d, acknowledge)
		if err != nil {
			return err
		}
		return acknowledge(ctx, updated)
	})
}

func (p *Pantalla) edicionParaEnvio(ctx context.Context, id string) (borradorEdicion, bool, error) {
	if work := p.vida().work; work != nil {
		return work.DurableEditor(ctx, id)
	}
	return p.leerEdicion(id)
}

//nolint:gocognit // recorre las ediciones pendientes y decide por cada una entre diario durable y almacén, sin soltar ninguna a medias.
func (p *Pantalla) vaciarEdiciones(ctx context.Context) error {
	if p.Almacen == nil || p.svc == nil {
		return nil
	}
	blocked := map[string]bool{}
	var blockedErr error
	if work := p.vida().work; work != nil {
		deliveries, err := work.EditorDeliveries(ctx, "")
		if err != nil {
			return err
		}
		for _, delivery := range deliveries {
			if err := p.enviarEdicionConfirmada(ctx, delivery.ID, p.enviarBorradorPrivado); err != nil {
				if errors.Is(err, api.ErrDatosCifradosNotas) {
					blocked[delivery.ID] = true
					blockedErr = err
					continue
				}
				return err
			}
		}
	}

	records, err := p.Almacen.List(p.claveAlmacen(), espacioBorradores)
	if err != nil {
		return err
	}
	for _, record := range records {
		if !strings.HasPrefix(record.ID, prefijoEdicion) {
			continue
		}
		if blocked[record.ID] {
			continue
		}
		var d borradorEdicion
		if err := json.Unmarshal(record.Value, &d); err != nil || !d.Valid() {
			return errBorradorEdicion
		}
		err = p.enviarEdicionConfirmada(ctx, record.ID, func(ctx context.Context, d borradorEdicion, acknowledge func(context.Context, api.Note) error) (api.Note, error) {
			if p.vida().work != nil {
				return p.svc.UpdateNoteAcknowledged(ctx, d.NoteID, d.NotebookID, d.Input, acknowledge)
			}
			return p.svc.UpdateNote(ctx, d.NoteID, d.NotebookID, d.Input)
		})
		if err != nil {
			if errors.Is(err, api.ErrDatosCifradosNotas) {
				blockedErr = err
				continue
			}
			return err
		}
	}
	return blockedErr
}

// recuperarEdicion restores a local source without replacing the remote original.
func (p *Pantalla) recuperarEdicion(n Nota) (string, borradorEdicion, *notas.DraftReceipt, bool, error) {
	if work := p.vida().work; work != nil {
		if id, draft, receipt, found := work.PendingEditor(n.ID, n.Cuaderno); found {
			return id, draft, receipt, true, nil
		}
	}
	if p.Almacen == nil {
		return "", borradorEdicion{}, nil, false, nil
	}
	rows, err := p.Almacen.List(p.claveAlmacen(), espacioBorradores)
	if err != nil {
		return "", borradorEdicion{}, nil, false, err
	}
	// Store.List returns the newest durable record first.
	for _, row := range rows {
		if !strings.HasPrefix(row.ID, prefijoEdicion) {
			continue
		}
		var d borradorEdicion
		if err := json.Unmarshal(row.Value, &d); err != nil || !d.Valid() {
			return "", d, nil, false, errBorradorEdicion
		}
		if d.NoteID == n.ID && d.NotebookID == n.Cuaderno {
			return row.ID, d, nil, true, nil
		}
	}
	return "", borradorEdicion{}, nil, false, nil
}
