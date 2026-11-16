// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// materializeSettings upgrades an authenticated V2 archive without changing its
// immutable receipt or contacting SQL. Each first projection uses the existing
// exact-byte WAL, so a lost publication ACK resumes the same transaction. A
// later edit/deletion is never overwritten by its original archived value.
func (r *Repository) materializeSettings(ctx context.Context, snapshot storage.AtomicCatalogSnapshot, receipt ImportReceipt) error {
	vacations := 0
	for _, item := range receipt.Sources {
		typ, id := item.Source.Kind, item.Source.ID
		if typ != "filter" && typ != "vacation" {
			continue
		}
		if typ == "vacation" {
			vacations++
			if vacations > 1 {
				return ErrConflict
			}
			id = "vacation"
		}
		raw, err := r.verifyArchive(ctx, snapshot, item.Source, item.Archive)
		if err != nil {
			return err
		}
		err = r.materializeSetting(ctx, snapshot, typ, id, item.Source.ID, raw)
		clear(raw)
		if err != nil {
			return err
		}
		snapshot, err = storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) materializeSetting(ctx context.Context, snapshot storage.AtomicCatalogSnapshot, typ, id, sourceID string, raw []byte) error {
	var identity struct {
		ID        string `json:"id"`
		AccountID string `json:"account_id"`
	}
	if json.Unmarshal(raw, &identity) != nil || typ == "filter" && identity.ID != sourceID || typ == "vacation" && identity.AccountID != sourceID {
		return ErrRecord
	}
	versions := heads(snapshot, objectID(typ, id))
	if len(versions) > 1 {
		return ErrConflict
	}
	if len(versions) == 1 {
		if versions[0].Commit.State == "deleted" {
			return nil
		}
		current, err := r.readDocument(ctx, versions[0])
		if err != nil {
			return err
		}
		same := bytes.Equal(current, raw)
		clear(current)
		if !same {
			return nil
		}
	}
	return r.writeDocument(ctx, snapshot, typ, id, raw, "live")
}

func settingRevision(s storage.AtomicCatalogSnapshot, typ, id string) (string, error) {
	versions := heads(s, objectID(typ, id))
	if len(versions) > 1 {
		return "", ErrConflict
	}
	if len(versions) == 0 {
		return "", nil
	}
	return digest([]byte(versions[0].Reference.RoomID + "\x00" + versions[0].Reference.EventID)), nil
}

func (r *Repository) Filters(parent context.Context) ([]api.MailFilter, error) {
	out := []api.MailFilter{}
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		values, err := r.list(ctx, s, "filter")
		if err != nil {
			return err
		}
		defer clearDocuments(values)
		times := map[string]time.Time{}
		for id, raw := range values {
			var value api.MailFilter
			var dates struct {
				CreatedAt time.Time `json:"created_at"`
			}
			if json.Unmarshal(raw, &value) != nil || value.ID != id || json.Unmarshal(raw, &dates) != nil {
				return ErrRecord
			}
			value.PrivateRevision, err = settingRevision(s, "filter", id)
			if err != nil {
				return err
			}
			times[id] = dates.CreatedAt
			out = append(out, value)
		}
		slices.SortFunc(out, func(a, b api.MailFilter) int {
			if n := cmp.Compare(a.Priority, b.Priority); n != 0 {
				return n
			}
			if n := times[a.ID].Compare(times[b.ID]); n != 0 {
				return n
			}
			return cmp.Compare(a.ID, b.ID)
		})
		return nil
	})
	return out, err
}

func (r *Repository) validateFilter(ctx context.Context, s storage.AtomicCatalogSnapshot, value api.MailFilter) error {
	f := model.Filter{Name: value.Name, Enabled: value.Enabled, Priority: value.Priority, MatchFrom: value.MatchFrom, MatchSubject: value.MatchSubject, MarkRead: value.MarkRead, MarkFlagged: value.MarkFlagged}
	if value.MoveTo != "" {
		id, err := uuid.Parse(value.MoveTo)
		if err != nil {
			return model.ErrInvalidFilter
		}
		f.MoveTo = &id
		raw, err := r.document(ctx, s, "mailbox", value.MoveTo)
		clear(raw)
		if errors.Is(err, ErrNotFound) {
			return model.ErrInvalidFilter
		}
		if err != nil {
			return err
		}
	}
	return f.Validate()
}

func filterCreateKey(id string) string { return "filter_create_" + id }

type filterCreation struct {
	Scope   storage.Scope             `json:"scope"`
	Target  storage.PublicationTarget `json:"target"`
	Request []byte                    `json:"request"`
	Payload []byte                    `json:"payload"`
}

// CreateFilter returns the assigned ID even after a failed acknowledgment. A
// caller must retain/reuse that ID when retrying; identical rules remain valid
// distinct creations when the caller deliberately starts a new operation.
func (r *Repository) CreateFilter(parent context.Context, value api.MailFilter) (api.MailFilter, error) {
	if value.ID == "" {
		value.ID = uuid.NewString()
	}
	if _, err := uuid.Parse(value.ID); err != nil {
		return value, ErrRecord
	}
	value.PrivateRevision = ""
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		if err := r.validateFilter(ctx, s, value); err != nil {
			return err
		}
		request, err := json.Marshal(value)
		if err != nil {
			return err
		}
		defer clear(request)
		owner := r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00mail"
		return r.cfg.Journal.Exclusive(ctx, owner, writesNamespace, filterCreateKey(value.ID), func() error {
			return r.resumeCreateFilter(ctx, s, owner, request, &value)
		})
	})
	return value, err
}

func (r *Repository) existingCreatedFilter(ctx context.Context, s storage.AtomicCatalogSnapshot, request []byte, value *api.MailFilter) (bool, error) {
	versions := heads(s, objectID("filter", value.ID))
	if len(versions) > 1 {
		return false, ErrConflict
	}
	if len(versions) == 0 {
		return false, nil
	}
	if versions[0].Commit.State != "live" {
		return false, ErrConflict
	}
	raw, err := r.readDocument(ctx, versions[0])
	if err != nil {
		return false, err
	}
	defer clear(raw)
	var current api.MailFilter
	if json.Unmarshal(raw, &current) != nil {
		return false, ErrRecord
	}
	encoded, _ := json.Marshal(current)
	defer clear(encoded)
	if !bytes.Equal(request, encoded) {
		return false, ErrConflict
	}
	value.PrivateRevision, err = settingRevision(s, "filter", value.ID)
	return true, err
}

func (r *Repository) resumeCreateFilter(ctx context.Context, s storage.AtomicCatalogSnapshot, owner string, request []byte, value *api.MailFilter) error {
	done, err := r.existingCreatedFilter(ctx, s, request, value)
	if err != nil {
		return err
	}
	key := filterCreateKey(value.ID)
	var work filterCreation
	found, err := r.cfg.Journal.Read(owner, writesNamespace, key, &work)
	if err != nil {
		return err
	}
	if done && !found {
		return nil
	}
	if !found {
		payload, err := patchField(request, "created_at", time.Now().UTC())
		if err != nil {
			return err
		}
		work = filterCreation{Scope: r.cfg.Scope, Target: s.Binding.Target, Request: bytes.Clone(request), Payload: payload}
	}
	defer clear(work.Payload)
	defer clear(work.Request)
	if work.Scope != r.cfg.Scope || work.Target != s.Binding.Target || !bytes.Equal(work.Request, request) || !json.Valid(work.Payload) {
		return ErrConflict
	}
	if err := r.cfg.Journal.Write(owner, writesNamespace, key, work); err != nil {
		return err
	}
	if err := r.writeDocument(ctx, s, "filter", value.ID, work.Payload, "live"); err != nil {
		return err
	}
	current, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return err
	}
	value.PrivateRevision, err = settingRevision(current, "filter", value.ID)
	if err != nil {
		return err
	}
	return r.cfg.Journal.Delete(owner, writesNamespace, key)
}

func (r *Repository) UpdateFilter(parent context.Context, value api.MailFilter) error {
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		raw, err := r.document(ctx, s, "filter", value.ID)
		if err != nil {
			return err
		}
		defer clear(raw)
		current, err := settingRevision(s, "filter", value.ID)
		if err != nil {
			return err
		}
		if value.PrivateRevision == "" || value.PrivateRevision != current {
			return ErrConflict
		}
		if err := r.validateFilter(ctx, s, value); err != nil {
			return err
		}
		// Patch only fields the public editor owns, preserving account_id,
		// created_at, extensions, and arbitrary precision JSON numbers.
		fields := apiFilterFields(value)
		fields["updated_at"] = time.Now().UTC()
		payload, err := patchFields(raw, fields)
		if err != nil {
			return err
		}
		defer clear(payload)
		return r.writeDocument(ctx, s, "filter", value.ID, payload, "live")
	})
}

func apiFilterFields(v api.MailFilter) map[string]any {
	var move any
	if v.MoveTo != "" {
		move = v.MoveTo
	}
	return map[string]any{"name": v.Name, "enabled": v.Enabled, "priority": v.Priority, "match_from": v.MatchFrom, "match_subject": v.MatchSubject, "move_to": move, "mark_read": v.MarkRead, "mark_flagged": v.MarkFlagged}
}

func patchFields(raw []byte, values map[string]any) ([]byte, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, ErrRecord
	}
	for name, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		fields[name] = encoded
	}
	return json.Marshal(fields)
}

func (r *Repository) DeleteFilter(parent context.Context, id string) error {
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		raw, err := r.document(ctx, s, "filter", id)
		clear(raw)
		if err != nil {
			return err
		}
		return r.writeDocument(ctx, s, "filter", id, []byte(`null`), "deleted")
	})
}

func (r *Repository) Vacation(parent context.Context) (api.MailVacation, error) {
	var out api.MailVacation
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		raw, err := r.document(ctx, s, "vacation", "vacation")
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		defer clear(raw)
		if json.Unmarshal(raw, &out) != nil {
			return ErrRecord
		}
		out.PrivateRevision, err = settingRevision(s, "vacation", "vacation")
		return err
	})
	return out, err
}

func (r *Repository) SetVacation(parent context.Context, value api.MailVacation) error {
	return r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		current, err := settingRevision(s, "vacation", "vacation")
		if err != nil {
			return err
		}
		if value.PrivateRevision != current {
			return ErrConflict
		}
		raw, err := r.document(ctx, s, "vacation", "vacation")
		if errors.Is(err, ErrNotFound) && current == "" {
			raw = []byte(`{}`)
		} else if err != nil {
			return err
		}
		defer clear(raw)
		payload, err := patchFields(raw, map[string]any{"enabled": value.Enabled, "subject": value.Subject, "body": value.Body, "starts_at": value.StartsAt, "ends_at": value.EndsAt, "updated_at": time.Now().UTC()})
		if err != nil {
			return err
		}
		defer clear(payload)
		return r.writeDocument(ctx, s, "vacation", "vacation", payload, "live")
	})
}
