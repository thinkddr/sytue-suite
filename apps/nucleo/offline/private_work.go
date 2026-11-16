// SPDX-License-Identifier: Apache-2.0
package offline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrPendingJournal = errors.New("offline: pending journal cannot be safely enumerated")

// PendingWork counts durable operations for exactly one application scope.
// An error is never equivalent to a zero count or permission to erase the WAL.
type PendingWork struct {
	Scope storage.Scope
	Count int
}

// PrivateWork is the common teardown boundary shared by all native products.
// Drain must finish before a key adapter is closed. Pending work is retained until
// it is acknowledged or the user explicitly discards this same scope/revision.
type PrivateWork interface {
	PrivateStorageScope() storage.Scope
	Pending(context.Context) (PendingWork, error)
	Drain(context.Context) error
	Resume() error
	Close()
}

// CountOwner counts only this exact owner tuple. Native implementations inspect
// hashed paths rather than decrypting large JSON or staged source blocks.
func (s *Store) CountOwner(owner string, namespaces ...string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, namespace := range namespaces {
		prefix := clave(owner, namespace)
		if counter, ok := s.b.(interface{ countPrefix(string) (int, error) }); ok {
			count, err := counter.countPrefix(prefix)
			if err != nil {
				return total, err
			}
			total += count
			continue
		}
		records, err := s.b.list(prefix)
		if err != nil {
			return total, err
		}
		total += len(records)
	}
	return total, nil
}

// RecordIDs reads only one encrypted record at a time on native backends. It lets
// a product count one resumable operation spanning multiple WAL records without
// retaining every large mutation body in memory.
func (s *Store) RecordIDs(owner, namespace string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := clave(owner, namespace)
	if reader, ok := s.b.(interface {
		recordIDs(string) ([]string, error)
	}); ok {
		return reader.recordIDs(prefix)
	}
	records, err := s.b.list(prefix)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(records))
	for _, raw := range records {
		var record struct {
			ID string `json:"id"`
		}
		err := json.Unmarshal(raw, &record)
		clear(raw)
		if err != nil {
			return nil, err
		}
		if record.ID == "" {
			return nil, ErrPendingJournal
		}
		ids = append(ids, record.ID)
	}
	return ids, nil
}

// RecordIDsPage bounds encrypted-body reads to limit. The native backend lists
// directory names once per call (O(number of records) metadata), then opens only
// this page's files. after is an opaque physical-name hash, not a domain ID.
func (s *Store) RecordIDsPage(ctx context.Context, owner, namespace, after string, limit int) ([]string, string, error) {
	if s == nil || !s.Encrypted() {
		return nil, "", ErrEncryptedStore
	}
	if limit < 1 || limit > 128 {
		return nil, "", ErrPendingJournal
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	reader, ok := s.b.(interface {
		recordIDsPage(context.Context, string, string, int) ([]string, string, error)
	})
	if !ok {
		return nil, "", ErrEncryptedStore
	}
	return reader.recordIDsPage(ctx, clave(owner, namespace), after, limit)
}

// CopyOwnerTo copies one owner's selected journals to an independently sealed
// native store. The caller must hold its source work lease exclusively. Records
// retain their exact values and timestamps; retries never overwrite a different
// destination intent. The source and its encryption key are never changed.
func (s *Store) CopyOwnerTo(ctx context.Context, target *Store, owner string, namespaces ...string) (int, error) {
	if s == nil || target == nil || !s.Encrypted() || !target.Encrypted() {
		return 0, ErrEncryptedStore
	}
	if s == target || owner == "" || len(namespaces) == 0 {
		return 0, ErrPendingJournal
	}
	copied := 0
	for _, namespace := range namespaces {
		if namespace == "" {
			return copied, ErrPendingJournal
		}
		ids, err := s.RecordIDs(owner, namespace)
		if err != nil {
			return copied, err
		}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return copied, err
			}
			if err := s.copyJournalRecord(ctx, target, owner, namespace, id); err != nil {
				return copied, err
			}
			copied++
		}
	}
	return copied, ctx.Err()
}

var ErrJournalConflict = errors.New("offline: destination has conflicting journal state")

func (s *Store) copyJournalRecord(ctx context.Context, target *Store, owner, namespace, id string) error {
	return s.copyRecordKey(ctx, target, clave(owner, namespace, id))
}

func (s *Store) copyRecordKey(ctx context.Context, target *Store, key string) error {
	s.mu.Lock()
	raw, err := s.b.get(key)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	defer clear(raw)
	var record Record
	if json.Unmarshal(raw, &record) != nil || record.ID == "" || path.Base(key) != clave(record.ID) || record.UpdatedAt.IsZero() || !json.Valid(record.Value) {
		clear(record.Value)
		return ErrPendingJournal
	}
	clear(record.Value)
	return target.exclusiveKey(ctx, key, func() error {
		target.mu.Lock()
		defer target.mu.Unlock()
		previous, err := target.b.get(key)
		defer clear(previous)
		if err == nil {
			if !bytes.Equal(raw, previous) {
				return ErrJournalConflict
			}
			return ctx.Err()
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := target.b.put(key, raw); err != nil {
			return err
		}
		checked, err := target.b.get(key)
		defer clear(checked)
		if err != nil {
			return err
		}
		if !bytes.Equal(raw, checked) {
			return ErrJournalConflict
		}
		return ctx.Err()
	})
}
