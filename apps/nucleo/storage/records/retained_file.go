// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const retainedFileKind = "org.sytue.retained-file.v1"

// RetainedFile binds an unindexed file to the producer's original target.
// It becomes visible only inside a complete authenticated application revision.
// It contains no key or plaintext file body.
type RetainedFile struct {
	Version  int                       `json:"v"`
	Scope    storage.Scope             `json:"scope"`
	Target   storage.PublicationTarget `json:"target"`
	Row      SnapshotRow               `json:"row"`
	Revision string                    `json:"revision"`
	Link     storage.Reference         `json:"link"`
}

type retainedFileHeader struct {
	Kind       string                    `json:"kind"`
	EntityKind string                    `json:"entity_kind"`
	Scope      storage.Scope             `json:"scope"`
	Target     storage.PublicationTarget `json:"target"`
	Row        SnapshotRow               `json:"row"`
}

func (r *Repository) retainedFileKey(operation string, row SnapshotRow) string {
	input, _ := json.Marshal(row)
	return "retained_" + hashBytes([]byte(operation+"\x00"+string(input)))
}

func (r *Repository) retainedFileHeader(work publication) retainedFileHeader {
	return retainedFileHeader{Kind: retainedFileKind, EntityKind: r.cfg.EntityKind, Scope: r.cfg.Scope, Target: work.Target, Row: SnapshotRow{Kind: work.Kind, ID: work.ID, Size: work.Size, SHA256: work.Input}}
}

// StageRetainedFile persists its file intent before invoking open. Uploads and
// FileLinks have no index, so a different device still opens the prior catalog.
// The caller first records the complete domain intent through MutateWithContent.
func (r *Repository) StageRetainedFile(parent context.Context, operation string, target storage.PublicationTarget, row SnapshotRow, open func() (io.ReadCloser, error)) (RetainedFile, error) {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return RetainedFile{}, err
	}
	defer done()
	if operation == "" || len(operation) > 256 || open == nil || row.Validate() != nil || !r.binaryKind(row.Kind) || r.cfg.ValidateBinary == nil {
		return RetainedFile{}, ErrSource
	}
	if err := r.cfg.ValidateBinary(ctx, row); err != nil {
		return RetainedFile{}, err
	}
	key := r.retainedFileKey(operation, row)
	var result RetainedFile
	err = r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), key, func() error {
		var err error
		result, err = r.resumeRetainedFile(ctx, key, target, row, open)
		return err
	})
	return result, err
}

func (r *Repository) resumeRetainedFile(ctx context.Context, key string, target storage.PublicationTarget, row SnapshotRow, open func() (io.ReadCloser, error)) (RetainedFile, error) {
	if err := r.requireReady(); err != nil {
		return RetainedFile{}, err
	}
	r.mu.Lock()
	current := r.binding.Target
	r.mu.Unlock()
	if current != target {
		return RetainedFile{}, ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(target); err != nil {
		return RetainedFile{}, err
	}
	var work publication
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), key, &work)
	if err != nil {
		return RetainedFile{}, err
	}
	if !found {
		work, err = r.newPublicationHeader(row.Kind, row.ID, "live", nil, nil)
		if err != nil {
			return RetainedFile{}, err
		}
		work.Target = target
		work.Input = row.SHA256
		work.Size = row.Size
		work.Binary = true
		work.File = true
		work.Parents = nil
		work.Epoch = ""
		work.Baseline = ""
		work.Seed = false
		if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
			return RetainedFile{}, err
		}
	}
	if err := r.validateRetainedWork(work, target, row); err != nil {
		return RetainedFile{}, err
	}
	if err := r.stagePublicationFile(ctx, key, row, &work, open); err != nil {
		return RetainedFile{}, err
	}
	metadata, _ := json.Marshal(r.retainedFileHeader(work))
	if err := r.publishFileContent(ctx, key, &work, metadata); err != nil {
		return RetainedFile{}, err
	}
	if err := ctx.Err(); err != nil {
		return RetainedFile{}, err
	}
	if work.Index == nil {
		payload, _ := json.Marshal(storage.FileLink{Kind: storage.FileLinkKind, Content: storage.ContentPointer{Kind: "file", RevisionID: work.Revision, Reference: *work.Content}})
		link, err := r.cfg.Crypto.PutStorageRecordAt(ctx, target, work.Revision+"_link", storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: r.object(row.Kind, row.ID), RevisionID: work.Revision, Role: "content", Payload: payload})
		if err != nil {
			return RetainedFile{}, err
		}
		work.Index = &link
		if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
			return RetainedFile{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return RetainedFile{}, err
	}
	result := RetainedFile{Version: 1, Scope: r.cfg.Scope, Target: target, Row: row, Revision: work.Revision, Link: *work.Index}
	err = r.validateRetainedFile(ctx, result)
	return result, err
}

func (r *Repository) validateRetainedWork(work publication, target storage.PublicationTarget, row SnapshotRow) error {
	if work.Version != 1 || work.Scope != r.cfg.Scope || work.Target != target || !target.Valid() || work.Kind != row.Kind || work.ID != row.ID || work.Input != row.SHA256 || work.Size != row.Size || !work.Binary || !work.File || work.State != "live" || !storage.Identifier(work.Revision) || !retainedWorkOrigin(work) {
		return ErrSource
	}
	return validateRetainedReferences(work, target, row)
}

func retainedWorkOrigin(work publication) bool {
	return len(work.Parents) == 0 && work.Epoch == "" && work.Baseline == "" && !work.Seed
}

func validateRetainedReferences(work publication, target storage.PublicationTarget, row SnapshotRow) error {
	if work.Content != nil && (!work.Content.Valid() || work.Content.RoomID != target.RoomID) {
		return ErrSource
	}
	if work.Index != nil && (!work.Index.Valid() || work.Index.RoomID != target.RoomID || work.Content == nil) {
		return ErrSource
	}
	if work.Staged != nil && (work.Staged.SizeBytes != row.Size || work.Staged.SHA256 != row.SHA256) {
		return ErrSource
	}
	return nil
}

// AcknowledgeRetainedFiles reads the exact selected application revision and
// verifies its complete intended set before acknowledging any file upload.
func (r *Repository) AcknowledgeRetainedFiles(parent context.Context, operation string, files []RetainedFile, accepted storage.ObjectVersion, selectFiles func([]byte) ([]RetainedFile, error)) error {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	if selectFiles == nil || len(files) == 0 || len(files) > 256 || operation == "" || len(operation) > 256 {
		return ErrSource
	}
	if err := r.verifyRetainedSet(ctx, files, accepted, selectFiles); err != nil {
		return err
	}
	sdk, ok := r.cfg.Crypto.(retainedFileAcknowledger)
	if !ok {
		return ErrSource
	}
	for _, file := range files {
		if err := r.acknowledgeRetainedFile(ctx, sdk, operation, file); err != nil {
			return err
		}
	}
	return nil
}

type retainedFileAcknowledger interface {
	AcknowledgeStorageFileLink(context.Context, *offline.Store, string, storage.Reference) error
}

func (r *Repository) acknowledgeRetainedFile(ctx context.Context, sdk retainedFileAcknowledger, operation string, file RetainedFile) error {
	key := r.retainedFileKey(operation, file.Row)
	var work publication
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), key, &work)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err := r.validateRetainedWork(work, file.Target, file.Row); err != nil {
		return err
	}
	if work.Index == nil || *work.Index != file.Link || work.Revision != file.Revision {
		return ErrSource
	}
	if err := sdk.AcknowledgeStorageFileLink(ctx, r.cfg.Journal, key+"_file", file.Link); err != nil && !errors.Is(err, storage.ErrUploadMissing) {
		return err
	}
	if work.Staged != nil {
		if err := r.cfg.Journal.DeleteStagedFile(r.owner(), r.stagingNamespace(), *work.Staged); err != nil {
			return err
		}
	}
	if err := r.cfg.Journal.Delete(r.owner(), r.namespace(), key); err != nil {
		return err
	}
	return nil
}

func (r *Repository) verifyRetainedSet(ctx context.Context, files []RetainedFile, accepted storage.ObjectVersion, selectFiles func([]byte) ([]RetainedFile, error)) error {
	summary, err := r.description(accepted)
	if err != nil || summary.Binary {
		return ErrSource
	}
	selected, err := r.ReadyVersion(summary.Entity, summary.ID)
	if err != nil {
		return err
	}
	wanted, _ := json.Marshal(accepted)
	actual, _ := json.Marshal(selected)
	if !bytes.Equal(wanted, actual) {
		return ErrConflict
	}
	raw, err := r.readJSON(ctx, accepted)
	if err != nil {
		return err
	}
	defer clear(raw)
	retained, err := selectFiles(raw)
	if err != nil {
		return err
	}
	if len(retained) != len(files) {
		return ErrSource
	}
	expected := make(map[RetainedFile]bool, len(files))
	for _, file := range files {
		if expected[file] {
			return ErrSource
		}
		expected[file] = true
	}
	for _, file := range retained {
		if !expected[file] {
			return ErrSource
		}
		delete(expected, file)
		if err := r.validateRetainedFile(ctx, file); err != nil {
			return err
		}
	}
	if len(expected) != 0 {
		return ErrSource
	}

	return nil
}
