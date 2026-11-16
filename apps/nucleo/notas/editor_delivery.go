// SPDX-License-Identifier: Apache-2.0
package notas

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const EditorDeliveryPrefix = "confirmacion:"

// EditorDelivery retains an exact original request independently of subsequent
// typing. Its acknowledgment is written only by the verified mutation callback.
// Both source and receipt remain encrypted in the existing Notes draft journal.
type EditorDelivery struct {
	Version    int           `json:"v"`
	Scope      storage.Scope `json:"scope"`
	ID         string        `json:"id"`
	Draft      EditorDraft   `json:"draft"`
	CreatedAt  time.Time     `json:"created_at"`
	AcceptedAt time.Time     `json:"accepted_at,omitempty"`
}

func (d EditorDelivery) Key() string {
	return EditorDeliveryPrefix + base64.RawURLEncoding.EncodeToString([]byte(d.ID)) + ":" + base64.RawURLEncoding.EncodeToString([]byte(d.Draft.Revision))
}

func (d EditorDelivery) valid(scope storage.Scope) bool {
	return d.Version == 1 && d.Scope == scope && strings.HasPrefix(d.ID, "edicion:") && len(d.ID) > len("edicion:") && d.Draft.Valid() && !d.CreatedAt.IsZero()
}

// BeginEditorDelivery persists the original source BEFORE invoking the SDK. The
// main editor record may then become v2 without losing the request that applied v1.
func (w *LocalWork) BeginEditorDelivery(ctx context.Context, id string, draft EditorDraft) (EditorDelivery, error) {
	_, done, err := w.Start()
	if err != nil {
		return EditorDelivery{}, errors.Join(ErrLocalAdmission, err)
	}
	defer done()
	release, err := w.acquire(ctx)
	if err == nil && release == nil {
		err = offline.ErrWorkLease
	}
	if err != nil {
		return EditorDelivery{}, errors.Join(ErrLocalAdmission, err)
	}
	defer release()
	delivery := EditorDelivery{Version: 1, Scope: w.scope, ID: id, Draft: draft.Clone(), CreatedAt: time.Now().UTC()}
	if !delivery.valid(w.scope) {
		return EditorDelivery{}, ErrEditorDraft
	}
	w.writes.Lock()
	defer w.writes.Unlock()
	err = w.journal.Exclusive(ctx, w.Owner(), EspacioBorradores, delivery.Key(), func() error {
		var current EditorDelivery
		found, err := w.journal.Read(w.Owner(), EspacioBorradores, delivery.Key(), &current)
		if err != nil {
			return err
		}
		if found {
			if !current.valid(w.scope) || current.Key() != delivery.Key() || !sameEditorDraft(current.Draft, draft) {
				return ErrEditorDraft
			}
			delivery = current
			return nil
		}
		return w.journal.Write(w.Owner(), EspacioBorradores, delivery.Key(), delivery)
	})
	return delivery, err
}

// MarkEditorDelivery records acceptance without deleting or rebasing the editor
// source. records still owns its mutation WAL until this callback returns.
func (w *LocalWork) MarkEditorDelivery(ctx context.Context, delivery EditorDelivery, updated api.Note) error {
	if !delivery.valid(w.scope) || updated.ID != delivery.Draft.NoteID || updated.NotebookID != delivery.Draft.NotebookID || updated.UpdatedAt.IsZero() {
		return ErrEditorDraft
	}
	w.writes.Lock()
	defer w.writes.Unlock()
	return w.journal.Exclusive(ctx, w.Owner(), EspacioBorradores, delivery.Key(), func() error {
		var current EditorDelivery
		found, err := w.journal.Read(w.Owner(), EspacioBorradores, delivery.Key(), &current)
		if err != nil {
			return err
		}
		if !found || !current.valid(w.scope) || current.Key() != delivery.Key() || !sameEditorDraft(current.Draft, delivery.Draft) {
			return ErrEditorDraft
		}
		if !current.AcceptedAt.IsZero() && !current.AcceptedAt.Equal(updated.UpdatedAt) {
			return ErrEditorDraft
		}
		current.AcceptedAt = updated.UpdatedAt
		return w.journal.Write(w.Owner(), EspacioBorradores, current.Key(), current)
	})
}

// FinishEditorDelivery runs only after records returned success, or after its
// resume-only port found no WAL AND this durable receipt proved acceptance.
// A failed cleanup keeps the receipt and exact request for idempotent retry.
func (w *LocalWork) FinishEditorDelivery(ctx context.Context, delivery EditorDelivery) error {
	if !delivery.valid(w.scope) {
		return ErrEditorDraft
	}
	w.writes.Lock()
	defer w.writes.Unlock()
	return w.journal.Exclusive(ctx, w.Owner(), EspacioBorradores, delivery.Key(), func() error {
		var current EditorDelivery
		found, err := w.journal.Read(w.Owner(), EspacioBorradores, delivery.Key(), &current)
		if err != nil || !found {
			return err
		}
		if !current.valid(w.scope) || current.Key() != delivery.Key() || !sameEditorDraft(current.Draft, delivery.Draft) || current.AcceptedAt.IsZero() {
			return ErrEditorDraft
		}
		updated := api.Note{ID: current.Draft.NoteID, NotebookID: current.Draft.NotebookID, UpdatedAt: current.AcceptedAt}
		if err := w.acknowledgeEditorLocked(ctx, current.ID, current.Draft, updated); err != nil {
			return err
		}
		return w.journal.Delete(w.Owner(), EspacioBorradores, current.Key())
	})
}

func (w *LocalWork) EditorDeliveries(ctx context.Context, id string) ([]EditorDelivery, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w.writes.Lock()
	defer w.writes.Unlock()
	records, err := w.journal.List(w.Owner(), EspacioBorradores)
	if err != nil {
		return nil, err
	}
	deliveries := []EditorDelivery{}
	for _, record := range records {
		if !strings.HasPrefix(record.ID, EditorDeliveryPrefix) {
			continue
		}
		var delivery EditorDelivery
		if json.Unmarshal(record.Value, &delivery) != nil || !delivery.valid(w.scope) || delivery.Key() != record.ID {
			return nil, ErrEditorDraft
		}
		if id == "" || delivery.ID == id {
			deliveries = append(deliveries, delivery)
		}
	}
	slices.SortFunc(deliveries, func(a, b EditorDelivery) int {
		if n := a.CreatedAt.Compare(b.CreatedAt); n != 0 {
			return n
		}
		return strings.Compare(a.Key(), b.Key())
	})
	return deliveries, ctx.Err()
}

func sameEditorDraft(a, b EditorDraft) bool {
	first, err := json.Marshal(a)
	if err != nil {
		return false
	}
	defer clear(first)
	second, err := json.Marshal(b)
	if err != nil {
		return false
	}
	defer clear(second)
	return bytes.Equal(first, second)
}
