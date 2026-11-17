// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PendingContent is one durable content mutation, including its original target.
// JSON and Captured belong to the caller, which must clear them after use.
type PendingContent struct {
	EntityID         string
	InputFingerprint string
	JSON, Captured   []byte
	Target           storage.PublicationTarget
}

// PendingContentPage owns at most one bounded page of durable input. Next is
// bound to the same authenticated scope and selectors, and may advance across
// unrelated journal records even when Items is empty.
type PendingContentPage struct {
	Items []PendingContent
	Next  string
}

type pendingContentCursor struct {
	Scope     storage.Scope `json:"scope"`
	Operation string        `json:"operation"`
	Entity    string        `json:"entity"`
	After     string        `json:"after"`
}

// PendingContents enumerates durable content work without publishing, fetching
// provider bytes, or imposing a global limit on the number of pending jobs.
// Each call reads at most limit journal bodies under the catalog exclusion.
func (r *Repository) PendingContents(parent context.Context, operation, entity, cursor string, limit int) (PendingContentPage, error) {
	result := PendingContentPage{Items: []PendingContent{}}
	if operation == "" || entity == "" || limit < 1 || limit > 128 || len(cursor) > 4096 {
		return result, ErrSource
	}
	after := ""
	if cursor != "" {
		raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
		var value pendingContentCursor
		if err != nil || json.Unmarshal(raw, &value) != nil || value.Scope != r.cfg.Scope || value.Operation != operation || value.Entity != entity || !storage.Identifier(value.After) {
			return result, ErrSource
		}
		after = value.After
	}
	ctx, done, err := r.Start(parent)
	if err != nil {
		return result, err
	}
	defer done()
	err = r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), "catalog", func() error {
		if err := r.requireReady(); err != nil {
			return err
		}
		var err error
		result, err = r.pendingContentsPage(ctx, operation, entity, after, limit)
		return err
	})
	if err != nil {
		for _, item := range result.Items {
			clear(item.JSON)
			clear(item.Captured)
		}
		return PendingContentPage{}, err
	}
	return result, nil
}

func (r *Repository) pendingContentsPage(ctx context.Context, operation, entity, after string, limit int) (result PendingContentPage, err error) {
	result.Items = []PendingContent{}
	defer func() {
		if err != nil {
			for _, item := range result.Items {
				clear(item.JSON)
				clear(item.Captured)
			}
			result = PendingContentPage{}
		}
	}()
	ids, next, err := r.cfg.Journal.RecordIDsPage(ctx, r.owner(), r.namespace(), after, limit)
	if err != nil {
		return result, err
	}
	for _, key := range ids {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !strings.HasPrefix(key, "mutation_") || strings.HasSuffix(key, "_publication") {
			continue
		}
		pending, err := r.pendingContentRecord(ctx, key, operation, entity, "")
		if err != nil {
			return result, err
		}
		if pending != nil {
			result.Items = append(result.Items, *pending)
		}
	}
	if next != "" {
		raw, _ := json.Marshal(pendingContentCursor{Scope: r.cfg.Scope, Operation: operation, Entity: entity, After: next})
		result.Next = base64.RawURLEncoding.EncodeToString(raw)
	}
	return result, nil
}

// ReadPendingContent reads one exact operation/entity/id without publishing or
// refreshing the catalog. Multiple intents are an explicit conflict. Enumeration
// is bounded and serialized with mutation writes under the existing work gate.
func (r *Repository) ReadPendingContent(parent context.Context, operation, entity, id string) (*PendingContent, error) {
	if operation == "" || entity == "" || id == "" {
		return nil, ErrSource
	}
	ctx, done, err := r.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	var result *PendingContent
	err = r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), "catalog", func() error {
		if err := r.requireReady(); err != nil {
			return err
		}
		var err error
		result, err = r.readPendingContent(ctx, operation, entity, id)
		return err
	})
	return result, err
}

func (r *Repository) readPendingContent(ctx context.Context, operation, entity, id string) (_ *PendingContent, err error) {
	ids, err := r.cfg.Journal.RecordIDs(r.owner(), r.namespace())
	if err != nil {
		return nil, err
	}
	var result *PendingContent
	defer func() {
		if err != nil && result != nil {
			clear(result.JSON)
			clear(result.Captured)
		}
	}()
	count := 0
	for _, key := range ids {
		if !strings.HasPrefix(key, "mutation_") || strings.HasSuffix(key, "_publication") {
			continue
		}
		count++
		if count > 128 {
			return nil, ErrSource
		}
		pending, err := r.pendingContentRecord(ctx, key, operation, entity, id)
		if err != nil {
			return nil, err
		}
		if pending == nil {
			continue
		}
		if result != nil {
			clear(pending.JSON)
			clear(pending.Captured)
			return nil, ErrConflict
		}
		result = pending
	}
	return result, nil
}

func (r *Repository) pendingContentRecord(ctx context.Context, key, operation, entity, id string) (_ *PendingContent, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var work mutation
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), key, &work)
	defer clear(work.JSON)
	defer clear(work.ContentInput)
	if err != nil || !found {
		return nil, err
	}
	if work.Operation != operation || work.Entity != entity || id != "" && work.ID != id {
		return nil, nil //nolint:nilnil // Absence is the read contract; no durable attempt matches this selector.
	}
	if !work.Content || key != "mutation_"+hashBytes([]byte(operation+"\x00"+work.Input)) {
		return nil, ErrSource
	}
	if err := r.validateMutation(ctx, work, operation, work.Input); err != nil {
		return nil, err
	}
	if err := r.contentTarget(work); err != nil {
		return nil, err
	}
	return &PendingContent{EntityID: work.ID, InputFingerprint: work.Input, JSON: append([]byte(nil), work.JSON...), Captured: append([]byte(nil), work.ContentInput...), Target: *work.Target}, nil
}
