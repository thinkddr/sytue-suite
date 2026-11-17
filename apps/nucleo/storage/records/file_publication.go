// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (r *Repository) stagePublicationFile(ctx context.Context, key string, row SnapshotRow, work *publication, open func() (io.ReadCloser, error)) error {
	if work.Content != nil || work.Staged != nil {
		return nil
	}
	stream, err := open()
	if err != nil {
		return err
	}
	staged, captureErr := r.cfg.Journal.StageFile(ctx, r.owner(), r.stagingNamespace(), stream, row.Size)
	closeErr := stream.Close()
	if captureErr != nil {
		return captureErr
	}
	if closeErr != nil {
		return closeErr
	}
	if staged.SizeBytes != row.Size || staged.SHA256 != row.SHA256 {
		_ = r.cfg.Journal.DeleteStagedFile(r.owner(), r.stagingNamespace(), staged)
		return ErrSource
	}
	work.Staged = &staged
	return r.cfg.Journal.Write(r.owner(), r.namespace(), key, *work)
}

func (r *Repository) publishFileContent(ctx context.Context, key string, work *publication, metadata []byte) error {
	if work.Content != nil {
		return nil
	}
	if work.Staged == nil {
		return ErrSource
	}
	reader, err := r.cfg.Journal.OpenStagedFile(ctx, r.owner(), r.stagingNamespace(), *work.Staged)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	ref, err := r.cfg.Crypto.UploadStorageFileAt(ctx, r.cfg.Gateway, r.cfg.Journal, key+"_file", work.Target, storage.FileRequest{ObjectID: r.object(work.Kind, work.ID), RevisionID: work.Revision, SizeBytes: work.Size, Metadata: metadata}, reader)
	if err != nil {
		return err
	}
	if !ref.Valid() || ref.RoomID != work.Target.RoomID {
		return ErrSource
	}
	work.Content = &ref
	return r.cfg.Journal.Write(r.owner(), r.namespace(), key, *work)
}
