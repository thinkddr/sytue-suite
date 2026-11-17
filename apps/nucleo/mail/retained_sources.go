// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// RetainedSourcePage lists authenticated encrypted originals, including domain
// rows which do not yet have a live projection. It never re-enumerates SQL.
func (r *Repository) RetainedSourcePage(parent context.Context, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	page := storage.StreamSourcePage{Scope: r.cfg.Scope, Epoch: r.cfg.MigrationState.Epoch, Rows: []storage.StreamSourceRow{}}
	if limit < 1 || limit > storage.MaxLegacySourceRows || after != (storage.LegacySourceCursor{}) && !storage.SourceCursorValid(after) {
		return page, ErrRecord
	}
	err := r.run(parent, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		receipt, err := r.retainedReceipt(ctx, s)
		if err != nil {
			return err
		}
		for _, item := range receipt.Sources {
			row := item.Source
			if row.Kind < after.Kind || row.Kind == after.Kind && row.ID <= after.ID {
				continue
			}
			if len(page.Rows) == limit {
				last := page.Rows[len(page.Rows)-1]
				page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
				break
			}
			page.Rows = append(page.Rows, row)
		}
		return nil
	})
	return page, err
}

func (r *Repository) retainedReceipt(ctx context.Context, s storage.AtomicCatalogSnapshot) (ImportReceipt, error) {
	receipt, err := r.loadImportReceipt(ctx, s)
	if err != nil {
		return receipt, err
	}
	if receipt.Kind != migrationKindV2 {
		return receipt, ErrNotReady
	}
	items := make([]importItem, len(receipt.Sources))
	for index, item := range receipt.Sources {
		items[index].Source = item.Source
		if !item.Archive.Valid() {
			return receipt, ErrRecord
		}
	}
	if err := checkSourcePlan(r.cfg.MigrationState, items); err != nil {
		return receipt, err
	}
	return receipt, nil
}

// OpenRetainedSource retains the actual repository work lease until the reader
// is closed. Descriptor, index, manifest length and full checksum must agree.
// It permits export/recovery of aliases, held MIME and exact policy originals
// without making them plaintext server data or implicitly resending anything.
func (r *Repository) OpenRetainedSource(parent context.Context, wanted storage.StreamSourceRow) (io.ReadCloser, error) {
	if !sourceKind(wanted.Kind) || wanted.Validate() != nil {
		return nil, ErrRecord
	}
	readerCtx, release, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	adopted := false
	defer func() {
		if !adopted {
			release()
		}
	}()
	var out io.ReadCloser
	err = r.runAdmitted(readerCtx, true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		receipt, err := r.retainedReceipt(ctx, s)
		if err != nil {
			return err
		}
		selected, err := selectRetainedSource(receipt, wanted)
		if err != nil {
			return err
		}
		version, err := r.resolveRetainedSourceVersion(s, wanted, selected)
		if err != nil {
			return err
		}
		if version.Commit.Content.Kind != "file" {
			data, err := r.verifyArchive(ctx, s, wanted, selected.Archive)
			if err != nil {
				return err
			}
			reader := newBodyReader(data)
			r.retainBodyReader(readerCtx, reader, release)
			out = reader
			adopted = true
			return nil
		}
		pointer := version.Commit.Content
		info, checksum, err := r.cfg.Files.Commitment(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
		if err != nil {
			return err
		}
		if info.Validate(r.cfg.Scope) != nil || info.ObjectID != version.ObjectID || info.RevisionID != pointer.RevisionID || info.SizeBytes != wanted.Size || checksum != wanted.SHA256 {
			return ErrRecord
		}
		reader, err := r.openMIMEReader(ctx, readerCtx, version, wanted.Size)
		if err != nil {
			return err
		}
		r.retainBodyReader(readerCtx, reader, release)
		out = reader
		adopted = true
		return nil
	})
	return out, err
}

func selectRetainedSource(receipt ImportReceipt, wanted storage.StreamSourceRow) (*ImportedSource, error) {
	var selected *ImportedSource
	for index := range receipt.Sources {
		item := &receipt.Sources[index]
		if item.Source.Kind == wanted.Kind && item.Source.ID == wanted.ID {
			if selected != nil || item.Source != wanted {
				return nil, ErrRecord
			}
			selected = item
		}
	}
	if selected == nil {
		return nil, ErrNotFound
	}
	return selected, nil
}

func (r *Repository) resolveRetainedSourceVersion(s storage.AtomicCatalogSnapshot, wanted storage.StreamSourceRow, selected *ImportedSource) (storage.ObjectVersion, error) {
	entry, found := s.Entries[selected.Archive]
	if !found {
		return storage.ObjectVersion{}, ErrNotReady
	}
	version, found, err := storage.AcceptedAtomicObject(s, r.cfg.Scope, entry.Record.ObjectID, "", &selected.Archive)
	if err != nil || !found {
		return storage.ObjectVersion{}, ErrRecord
	}
	typ, id := sourceArchive(wanted)
	h, err := decodeHeader(version)
	if err != nil || h.Type != typ || h.ID != id || version.Commit.State != "live" || version.Commit.Content == nil {
		return storage.ObjectVersion{}, ErrRecord
	}
	return version, nil
}
