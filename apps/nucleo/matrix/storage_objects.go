// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// StorageObjectPage is private client data. Exhaust every allowed storage room's
// history before reducing its index entries with storage.CatalogHeads. A partial
// page is never evidence that a missing object or parent was deleted.
type StorageObjectPage = storage.ObjectPage

func (c *Client) StorageObjectHistory(ctx context.Context, room, cursor string, limit int) (StorageObjectPage, error) {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return StorageObjectPage{}, err
	}
	page, err := c.History(ctx, room, cursor, limit)
	if err != nil {
		return StorageObjectPage{}, err
	}
	out := StorageObjectPage{Next: page.Next}
	for _, value := range page.Events {
		fragment, err := storagePQHistoryFragment(value, c.session.UserID.String())
		if err != nil {
			return StorageObjectPage{}, err
		}
		if fragment {
			continue
		}
		rec, err := c.readPrivateStorageRecord(ctx, value, scope, c.session.UserID.String())
		if err != nil {
			return StorageObjectPage{}, err
		}
		if rec.Role != "index" {
			continue
		}
		entry := storage.RecordEntry{Reference: value.Reference, Record: rec}
		if err := storage.ValidateCatalogEntry(entry, scope); err != nil {
			return StorageObjectPage{}, err
		}
		out.Entries = append(out.Entries, entry)
	}
	if err := ctx.Err(); err != nil {
		return StorageObjectPage{}, err
	}
	if c.closed.Load() {
		return StorageObjectPage{}, ErrClosed
	}
	return out, nil
}

func (c *Client) readStorageObject(ctx context.Context, scope storage.Scope, ref EventRef, object string) (storage.ObjectVersion, error) {
	rec, err := c.ReadStorageRecord(ctx, ref, object, "index")
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: rec}, scope)
}

// StorageObject authenticates one exact catalog revision, including private
// metadata and content pointer. A tombstone does not erase retained history.
func (c *Client) StorageObject(ctx context.Context, ref EventRef, object string) (storage.ObjectVersion, error) {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return c.readStorageObject(ctx, scope, ref, object)
}

func (c *Client) checkStorageObjectContent(ctx context.Context, scope storage.Scope, object string, pointer storage.ContentPointer) error {
	if pointer.Kind == "file" {
		_, err := (storageFileIO{scope: scope, records: c, closed: c.closed.Load}).root(ctx, pointer.Reference, object, pointer.RevisionID)
		return err
	}
	content, err := c.ReadStorageRecord(ctx, pointer.Reference, object, "content")
	if err == nil && content.RevisionID != pointer.RevisionID {
		return storage.ErrRecord
	}
	return err
}

// PutStorageObject publishes an SDK-encrypted catalog revision after verifying
// its parent identities and exact content reference. It never deletes a source
// or a previous revision. A file migration must finish UploadStorageFile's full
// verification before publishing this pointer. Retain transaction on retries;
// new content/metadata needs a new transaction and revision. Concurrent edits keep
// their explicit parent branches instead of choosing a winner by timestamp.
func (c *Client) PutStorageObject(ctx context.Context, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	target, err := c.StoragePublicationTarget()
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return c.PutStorageObjectAt(ctx, target, transaction, object, revision, commit)
}

// PutStorageObjectAt keeps a durable application intent on its original device
// and room endpoint, including retries after a catalog ACK was lost.
func (c *Client) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	if !commit.Valid() || !storage.Identifier(object) || !storage.Identifier(revision) || !storage.Identifier(transaction) {
		return storage.ObjectVersion{}, storage.ErrRecord
	}
	// Freeze all private inputs before network I/O so later lifecycle hooks
	// cannot change a partially authenticated request.
	payload, err := json.Marshal(commit)
	var copied storage.ObjectCommit
	if err != nil || json.Unmarshal(payload, &copied) != nil {
		return storage.ObjectVersion{}, storage.ErrRecord
	}
	commit = copied
	if err := c.CheckStoragePublicationTarget(target); err != nil {
		return storage.ObjectVersion{}, err
	}
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	previous := make([]string, len(commit.Parents))
	for i, ref := range commit.Parents {
		parent, err := c.readStorageObject(ctx, scope, ref, object)
		if err != nil {
			return storage.ObjectVersion{}, err
		}
		if parent.RevisionID == revision {
			return storage.ObjectVersion{}, storage.ErrRecord
		}
		previous[i] = ref.EventID
	}
	if commit.Content != nil {
		if err := c.checkStorageObjectContent(ctx, scope, object, *commit.Content); err != nil {
			return storage.ObjectVersion{}, err
		}
	}
	record := storage.Record{Version: 1, Scope: scope, ObjectID: object, RevisionID: revision, Role: "index", Previous: previous, Payload: payload}
	ref, err := writeStorageRecordVerified(ctx, pinnedStorageRecords{storageRecordIO: c, publication: c, target: target}, transaction, record)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	if c.closed.Load() {
		return storage.ObjectVersion{}, ErrClosed
	}
	return storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: record}, scope)
}
