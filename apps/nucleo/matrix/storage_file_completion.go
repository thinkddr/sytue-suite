// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (f storageFileIO) acknowledge(ctx context.Context, job string, catalog EventRef) error {
	return f.acknowledgeWith(ctx, job, catalog, false, f.catalogFilePointer)
}

func (f storageFileIO) acknowledgeLink(ctx context.Context, job string, link EventRef) error {
	return f.acknowledgeWith(ctx, job, link, true, f.linkFilePointer)
}

type fileAcceptance func(context.Context, storageFileUpload, EventRef) (*storage.ContentPointer, error)

func (f storageFileIO) acknowledgeWith(ctx context.Context, job string, catalog EventRef, strictTarget bool, pointerFor fileAcceptance) error {
	if f.records == nil || !f.journal.Encrypted() || !storage.Identifier(job) || !catalog.Valid() {
		return storage.ErrRecord
	}
	if err := f.guard(ctx); err != nil {
		return err
	}
	return f.journal.Exclusive(ctx, f.journalOwner(), storage.PendingUploadsNamespace, job, func() error {
		var state storageFileUpload
		found, err := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, job, &state)
		if err != nil {
			return err
		}
		if !found {
			return ErrStorageUploadMissing
		}
		if err := state.validate(f.scope, state.Request); err != nil {
			return err
		}
		if !state.Verified || state.Published == nil {
			return storage.ErrRecord
		}
		if strictTarget {
			if err := f.acceptedFileTarget(state, catalog); err != nil {
				return err
			}
		}
		pointer, err := pointerFor(ctx, state, catalog)
		if err != nil {
			return err
		}
		if pointer == nil || pointer.Kind != "file" || pointer.RevisionID != state.Request.RevisionID || pointer.Reference != *state.Published {
			return storage.ErrRecord
		}
		if err := f.guard(ctx); err != nil {
			return err
		}
		return f.journal.Delete(f.journalOwner(), storage.PendingUploadsNamespace, job)
	})
}

func (f storageFileIO) acceptedFileTarget(state storageFileUpload, ref EventRef) error {
	publication, ok := f.records.(storagePublicationIO)
	if !ok || ref.RoomID != state.Target.RoomID {
		return storage.ErrRecord
	}
	return publication.CheckStoragePublicationTarget(state.Target)
}

func (f storageFileIO) catalogFilePointer(ctx context.Context, state storageFileUpload, ref EventRef) (*storage.ContentPointer, error) {
	record, err := f.records.ReadStorageRecord(ctx, ref, state.Request.ObjectID, "index")
	if err != nil {
		return nil, err
	}
	version, err := storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: record}, f.scope)
	if err != nil {
		return nil, err
	}
	if version.ObjectID != state.Request.ObjectID {
		return nil, storage.ErrRecord
	}
	return version.Commit.Content, nil
}

func (f storageFileIO) linkFilePointer(ctx context.Context, state storageFileUpload, ref EventRef) (*storage.ContentPointer, error) {
	record, err := f.records.ReadStorageRecord(ctx, ref, state.Request.ObjectID, "content")
	if err != nil {
		return nil, err
	}
	link, err := storage.DecodeFileLink(record, f.scope)
	if err != nil {
		return nil, err
	}
	if record.ObjectID != state.Request.ObjectID || record.RevisionID != state.Request.RevisionID {
		return nil, storage.ErrRecord
	}
	return &link.Content, nil
}

// AcknowledgeStorageFile retires ONLY the local upload journal, after reading an
// authenticated catalog commit that retains its exact fully verified file.
// It never removes source data, content keys in Matrix, or provider ciphertext.
// A missing journal is not proof of completion: the app must inspect its catalog
// before deciding to upload again or acknowledging a durable application change.
func (c *Client) AcknowledgeStorageFile(ctx context.Context, journal *offline.Store, job string, catalog EventRef) error {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return err
	}
	return (storageFileIO{scope: scope, records: c, journal: journal, closed: c.closed.Load}).acknowledge(ctx, job, catalog)
}

// AcknowledgeStorageFileLink retires only a verified upload WAL after its exact
// role-content link was retained by the app's control-plane CAS. The caller must
// compare that accepted reference before invoking this method. An unsuccessful
// CAS/retry must keep the WAL; no source, key or provider file is deleted here.
func (c *Client) AcknowledgeStorageFileLink(ctx context.Context, journal *offline.Store, job string, link EventRef) error {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return err
	}
	return (storageFileIO{scope: scope, records: c, journal: journal, closed: c.closed.Load}).acknowledgeLink(ctx, job, link)
}

// AcknowledgeAtomicStorageFile reuses the original verified upload WAL cleanup.
// Only an opaque complete-batch proof may supply the exact accepted member;
// neither a commit root nor a fabricated single-object version can stand in.
func (c *Client) AcknowledgeAtomicStorageFile(ctx context.Context, journal *offline.Store, job string, completion storage.VerifiedAtomicFileCompletion) error {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return err
	}
	fileIO := storageFileIO{scope: scope, records: c, journal: journal, closed: c.closed.Load}
	return fileIO.acknowledgeWith(ctx, job, completion.MemberReference(), false, func(ctx context.Context, state storageFileUpload, ref EventRef) (*storage.ContentPointer, error) {
		if ref != completion.MemberReference() || state.Published == nil {
			return nil, storage.ErrRecord
		}
		pointer, err := completion.ReadPointer(ctx, scope, state.Request, *state.Published, state.SourceSHA256)
		if err != nil {
			return nil, err
		}
		return &pointer, nil
	})
}
