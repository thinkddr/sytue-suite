// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	documentOperationContextKey struct{}
	recoveredDocumentContextKey struct{}
)

// WithDocumentOperation binds a caller-retained, explicit logical edit ID. Reuse
// the same ID after an uncertain response; a deliberate new edit uses a new ID.
// This identifier provides idempotency only and grants no publication authority.
func WithDocumentOperation(ctx context.Context, id string) (context.Context, error) {
	if ctx == nil || !storage.MigrationEpoch(id) {
		return nil, ErrRecord
	}
	return context.WithValue(ctx, documentOperationContextKey{}, id), nil
}

func documentOperation(ctx context.Context) string {
	value, _ := ctx.Value(documentOperationContextKey{}).(string)
	return value
}

// documentWriteKey preserves the original immutable WAL key byte for byte.
func documentWriteKey(header, payload []byte, state string) string {
	return digest(append(append(bytes.Clone(header), []byte("\x00"+state+"\x00")...), payload...))
}

func documentIntentKey(header, payload []byte, state, operation string) string {
	key := documentWriteKey(header, payload, state)
	if operation != "" {
		key = digest(append(bytes.Clone(header), []byte("\x00operation\x00"+operation)...))
	}
	return key
}

type acceptedDocumentIntent struct {
	Version int           `json:"v"`
	Scope   storage.Scope `json:"scope"`
	Key     string        `json:"key"`
	State   string        `json:"state"`
	Work    writeIntent   `json:"work"`
}

func validateDocumentIntent(scope storage.Scope, binding storage.Binding, key, state string, work *writeIntent) error {
	if work.State != "" && work.State != state || work.validate(scope, work.Header, work.Payload, work.ForceFile) != nil || !work.Target.Valid() || !slices.Contains(binding.Rooms, work.Target.RoomID) || state != "live" && state != "deleted" || work.OperationID != "" && !storage.MigrationEpoch(work.OperationID) {
		return ErrRecord
	}
	raw, err := json.Marshal(work.Header)
	if err != nil {
		return err
	}
	defer clear(raw)
	if documentIntentKey(raw, work.Payload, state, work.OperationID) != key {
		return ErrRecord
	}
	object := objectID(work.Header.Type, work.Header.ID)
	_, err = decodeHeader(storage.ObjectVersion{ObjectID: object, Commit: storage.ObjectCommit{Metadata: raw}})
	return err
}

// recoverAcceptedDocumentIntents never publishes. Accepted local work moves to
// retained encrypted history before its pending entry can disappear. Missing
// revisions remain pending; a later retry must read the original accepted SDK
// revision again, even if a newer edit has since become visible.
func (r *Repository) recoverAcceptedDocumentIntents(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
	if s.Binding.Scope != r.cfg.Scope || !s.Binding.Valid() {
		return ErrRecord
	}
	owner := r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00mail"
	after := ""
	for {
		ids, next, err := r.cfg.Journal.RecordIDsPage(ctx, owner, writesNamespace, after, 128)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := r.recoverAcceptedDocumentIntent(ctx, s, owner, id); err != nil {
				return err
			}
		}
		if next == "" {
			return ctx.Err()
		}
		if next == after {
			return ErrRecord
		}
		after = next
	}
}

func (r *Repository) recoverAcceptedDocumentIntent(ctx context.Context, s storage.AtomicCatalogSnapshot, owner, id string) error {
	// Filter creation has its own original DTO/recovery protocol in this WAL.
	if strings.HasPrefix(id, "filter_create_") && storage.MigrationEpoch(strings.TrimPrefix(id, "filter_create_")) {
		return nil
	}
	if len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
		return ErrRecord
	}
	return r.cfg.Journal.Exclusive(ctx, owner, writesNamespace, id, func() error {
		var work writeIntent
		found, err := r.cfg.Journal.Read(owner, writesNamespace, id, &work)
		defer clear(work.Payload)
		if err != nil || !found {
			return err
		}
		raw, err := json.Marshal(work.Header)
		if err != nil {
			return err
		}
		defer clear(raw)
		state := work.State
		if work.OperationID == "" {
			state = ""
			for _, candidate := range []string{"live", "deleted"} {
				if documentIntentKey(raw, work.Payload, candidate, "") == id {
					state = candidate
				}
			}
		}
		if err := validateDocumentIntent(r.cfg.Scope, s.Binding, id, state, &work); err != nil {
			return err
		}
		recoverCtx := context.WithValue(ctx, recoveredDocumentContextKey{}, true)
		_, err = r.completeDocumentWrite(recoverCtx, s, owner, id, objectID(work.Header.Type, work.Header.ID), &work, state)
		return err
	})
}

func (r *Repository) recoverDocumentHistory(ctx context.Context, s storage.AtomicCatalogSnapshot, owner, key string, desired header, payload []byte, state string, forceFile bool) (bool, error) {
	var receipt acceptedDocumentIntent
	found, err := r.cfg.Journal.Read(owner, storage.MailWriteHistoryNamespace, key, &receipt)
	defer clear(receipt.Work.Payload)
	if err != nil || !found {
		return false, err
	}
	if receipt.Version != 1 || receipt.Scope != r.cfg.Scope || receipt.Key != key || receipt.State != state || receipt.Work.validate(r.cfg.Scope, desired, payload, forceFile) != nil || receipt.Work.Index == nil {
		return false, ErrRecord
	}
	if err := validateDocumentIntent(r.cfg.Scope, s.Binding, key, state, &receipt.Work); err != nil {
		return false, err
	}
	accepted, err := r.completeDocumentWrite(ctx, s, owner, key, objectID(desired.Type, desired.ID), &receipt.Work, state)
	if err != nil {
		return false, err
	}
	if !accepted {
		return false, ErrNotReady
	}
	return true, nil
}

func (r *Repository) retainAcceptedDocumentIntent(ctx context.Context, owner, key, state string, work *writeIntent, version storage.ObjectVersion) error {
	recovered, _ := ctx.Value(recoveredDocumentContextKey{}).(bool)
	if !recovered && work.OperationID == "" {
		return nil
	}
	original := *work
	reference := version.Reference
	original.Index = &reference
	if version.Commit.Content != nil {
		content := version.Commit.Content.Reference
		original.Content = &content
	}
	receipt := acceptedDocumentIntent{Version: 1, Scope: r.cfg.Scope, Key: key, State: state, Work: original}
	return r.cfg.Journal.Write(owner, storage.MailWriteHistoryNamespace, key, receipt)
}
