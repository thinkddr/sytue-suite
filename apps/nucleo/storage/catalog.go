// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"bytes"
	"context"
	"errors"
	"slices"
)

// ErrCatalogSource denotes invalid or changed authenticated routing. Port and
// record-validation errors remain unchanged so callers can distinguish them.
var ErrCatalogSource = errors.New("storage: invalid or changed catalog source")

// CatalogReader is the maintained SDK's scoped index-history surface. It does
// not hand keys or trust decisions to applications.
type CatalogReader interface {
	StorageBindings(context.Context) (Binding, error)
	StorageObjectHistory(context.Context, string, string, int) (ObjectPage, error)
}

// CatalogSnapshot is complete only after every retained room has reached its
// end and its authenticated routing has been checked again. It preserves every
// index revision; Heads never substitutes for the retained history.
type CatalogSnapshot struct {
	Binding Binding
	Entries map[Reference]RecordEntry
	Heads   []ObjectVersion
}

func loadCatalogEntries(ctx context.Context, reader CatalogReader, expected Scope) (Binding, map[Reference]RecordEntry, error) {
	binding, entries, _, err := loadCatalogEntriesKnown(ctx, reader, expected, nil)
	return binding, entries, err
}

// loadCatalogEntriesKnown es loadCatalogEntries sin volver a validar las entradas de
// las que same (si no es nil) dice que son una ya validada para expected; devuelve
// cuáles.
func loadCatalogEntriesKnown(ctx context.Context, reader CatalogReader, expected Scope, same func(RecordEntry) bool) (Binding, map[Reference]RecordEntry, map[Reference]bool, error) {
	if err := ctx.Err(); err != nil {
		return Binding{}, nil, nil, err
	}
	if reader == nil || !expected.Valid() {
		return Binding{}, nil, nil, ErrCatalogSource
	}
	binding, err := reader.StorageBindings(ctx)
	if err != nil {
		return Binding{}, nil, nil, err
	}
	if !binding.Valid() || binding.Scope != expected {
		return Binding{}, nil, nil, ErrCatalogSource
	}
	binding.Rooms = slices.Clone(binding.Rooms)
	entries := map[Reference]RecordEntry{}
	known := map[Reference]bool{}
	for _, room := range binding.Rooms {
		if err := loadCatalogRoom(ctx, reader, expected, room, entries, same, known); err != nil {
			return Binding{}, nil, nil, err
		}
	}
	current, err := reader.StorageBindings(ctx)
	if err != nil {
		return Binding{}, nil, nil, err
	}
	if !binding.Same(current) {
		return Binding{}, nil, nil, ErrCatalogSource
	}
	return binding, entries, known, nil
}

// LoadCatalog shares the complete-room scan used by private application
// repositories. An interrupted, cyclic or changed source never produces a
// successful empty/partial projection. Returned payloads and routing are owned
// by this snapshot; later changes in a port's buffers cannot change them.
func LoadCatalog(ctx context.Context, reader CatalogReader, expected Scope) (CatalogSnapshot, error) {
	binding, entries, err := loadCatalogEntries(ctx, reader, expected)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	records := make([]RecordEntry, 0, len(entries))
	for _, entry := range entries {
		records = append(records, entry)
	}
	heads, err := CatalogHeads(expected, records)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return CatalogSnapshot{}, err
	}
	return CatalogSnapshot{Binding: binding, Entries: entries, Heads: heads}, nil
}

func loadCatalogRoom(ctx context.Context, reader CatalogReader, scope Scope, room string, entries map[Reference]RecordEntry, same func(RecordEntry) bool, known map[Reference]bool) error {
	cursor := ""
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := reader.StorageObjectHistory(ctx, room, cursor, 100)
		if err != nil {
			return err
		}
		for _, entry := range page.Entries {
			if entry.Reference.RoomID != room {
				return ErrCatalogSource
			}
			if _, duplicate := entries[entry.Reference]; duplicate {
				return ErrCatalogSource
			}
			if len(entry.Record.Payload) > MaxRecordBytes {
				return ErrRecord
			}
			if same != nil && same(entry) {
				known[entry.Reference] = true
			} else if err := ValidateCatalogEntry(entry, scope); err != nil {
				return err
			}
			entries[entry.Reference] = cloneCatalogEntry(entry)
		}
		if page.Next == "" {
			return nil
		}
		if page.Next == cursor || seen[page.Next] {
			return ErrCatalogSource
		}
		seen[page.Next] = true
		cursor = page.Next
	}
}

func cloneCatalogEntry(entry RecordEntry) RecordEntry {
	entry.Record.Payload = bytes.Clone(entry.Record.Payload)
	entry.Record.Previous = slices.Clone(entry.Record.Previous)
	return entry
}
