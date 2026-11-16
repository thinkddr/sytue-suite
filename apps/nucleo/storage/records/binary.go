// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (r *Repository) binaryKind(kind string) bool {
	return r.cfg.BinaryKind != nil && r.cfg.BinaryKind(kind)
}

func (r *Repository) publicationEntity(work publication) entity {
	return entity{Kind: r.cfg.EntityKind, Entity: work.Kind, ID: work.ID, SHA256: work.Input, Epoch: work.Epoch, Baseline: work.Baseline, Seed: work.Seed, Binary: work.Binary, Size: work.Size}
}

func binaryRow(value entity) SnapshotRow {
	return SnapshotRow{Kind: value.Entity, ID: value.ID, Size: value.Size, SHA256: value.SHA256}
}

// binaryInfo authenticates the SDK file header and its binding to the complete
// entity descriptor. Large JSON also uses files, so pointer.Kind alone is never
// used to decide whether an entity is binary.
func (r *Repository) binaryInfo(ctx context.Context, version storage.ObjectVersion) (SnapshotRow, error) {
	value, err := r.binaryDescription(ctx, version)
	if err != nil {
		return SnapshotRow{}, err
	}
	pointer := version.Commit.Content
	info, err := r.cfg.Crypto.StorageFileInfo(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return SnapshotRow{}, err
	}
	if err := r.validateBinaryInfo(version, value, info); err != nil {
		return SnapshotRow{}, err
	}
	return binaryRow(value), nil
}

func (r *Repository) binaryDescription(ctx context.Context, version storage.ObjectVersion) (entity, error) {
	value, err := r.description(version)
	if err != nil {
		return entity{}, err
	}
	pointer := version.Commit.Content
	if !value.Binary || pointer == nil || pointer.Kind != "file" {
		return entity{}, ErrSource
	}
	row := binaryRow(value)
	if row.Validate() != nil || r.cfg.ValidateBinary == nil {
		return entity{}, ErrSource
	}
	if err := r.cfg.ValidateBinary(ctx, row); err != nil {
		return entity{}, err
	}
	return value, nil
}

func (r *Repository) validateBinaryInfo(version storage.ObjectVersion, value entity, info storage.FileRequest) error {
	pointer := version.Commit.Content
	wanted, _ := json.Marshal(value)
	if info.Validate(r.cfg.Scope) != nil || info.ObjectID != version.ObjectID || info.RevisionID != pointer.RevisionID || info.SizeBytes != value.Size || !bytes.Equal(info.Metadata, wanted) {
		return ErrSource
	}
	return nil
}

// Optional maintained-SDK capability. Its root record authenticates the complete
// plaintext digest inside this same private scope without fetching provider media.
type binaryCommitmentReader interface {
	StorageFileCommitment(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error)
}

func (r *Repository) verifyBinaryCommitment(ctx context.Context, version storage.ObjectVersion) (SnapshotRow, error) {
	value, err := r.binaryDescription(ctx, version)
	if err != nil {
		return SnapshotRow{}, err
	}
	if err := r.binaryBinding(version); err != nil {
		return SnapshotRow{}, err
	}
	reader, ok := r.cfg.Crypto.(binaryCommitmentReader)
	if !ok {
		return r.readBinary(ctx, version, io.Discard)
	}
	pointer := version.Commit.Content
	info, sum, err := reader.StorageFileCommitment(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return SnapshotRow{}, err
	}
	if err := r.validateBinaryInfo(version, value, info); err != nil {
		return SnapshotRow{}, err
	}
	if sum != value.SHA256 {
		return SnapshotRow{}, ErrSource
	}
	if err := ctx.Err(); err != nil {
		return SnapshotRow{}, err
	}
	return binaryRow(value), nil
}

func (r *Repository) binaryBinding(version storage.ObjectVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.binding.Valid() || r.binding.Scope != r.cfg.Scope || !version.Reference.Valid() || !slices.Contains(r.binding.Rooms, version.Reference.RoomID) || !slices.Contains(r.binding.Rooms, version.Commit.Content.Reference.RoomID) {
		return ErrSource
	}
	return nil
}

type binarySink struct {
	out       io.Writer
	remaining int64
	sum       hash.Hash
}

func (s *binarySink) Write(p []byte) (int, error) {
	if int64(len(p)) > s.remaining {
		return 0, ErrSource
	}
	n, err := s.out.Write(p)
	if n > 0 {
		_, _ = s.sum.Write(p[:n])
		s.remaining -= int64(n)
	}
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (r *Repository) readBinary(ctx context.Context, version storage.ObjectVersion, output io.Writer) (SnapshotRow, error) {
	if output == nil {
		return SnapshotRow{}, ErrSource
	}
	row, err := r.binaryInfo(ctx, version)
	if err != nil {
		return SnapshotRow{}, err
	}
	pointer := version.Commit.Content
	sink := &binarySink{out: output, remaining: row.Size, sum: sha256.New()}
	request, err := r.cfg.Crypto.DownloadStorageFile(ctx, r.cfg.Gateway, pointer.Reference, version.ObjectID, pointer.RevisionID, sink)
	if err != nil {
		return SnapshotRow{}, err
	}
	if request.ObjectID != version.ObjectID || request.RevisionID != pointer.RevisionID || request.SizeBytes != row.Size || sink.remaining != 0 || hex.EncodeToString(sink.sum.Sum(nil)) != row.SHA256 {
		return SnapshotRow{}, ErrSource
	}
	if err := ctx.Err(); err != nil {
		return SnapshotRow{}, err
	}
	return row, nil
}

// ReadBinary streams authenticated plaintext only to the calling device. The
// writer must treat output as incomplete until this method returns successfully.
func (r *Repository) ReadBinary(parent context.Context, kind, id string, output io.Writer) (SnapshotRow, error) {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return SnapshotRow{}, err
	}
	defer done()
	version, err := r.ReadyVersion(kind, id)
	if err != nil {
		return SnapshotRow{}, err
	}
	return r.readBinary(ctx, version, output)
}

func (r *Repository) validateBinaryWork(ctx context.Context, work publication, state SnapshotState, row SnapshotRow) error {
	key := "import_" + hashBytes([]byte(state.Epoch+"\x00"+work.Kind+"\x00"+work.ID))
	if err := r.validateSnapshotWork(key, state, work); err != nil {
		return err
	}
	if !r.binaryKind(row.Kind) || row.Validate() != nil || !work.Binary || !work.File || work.Kind != row.Kind || work.ID != row.ID || work.Size != row.Size || work.Input != row.SHA256 || work.State != "live" {
		return ErrSource
	}
	if work.Content != nil && (!work.Content.Valid() || work.Content.RoomID != work.Target.RoomID) {
		return ErrSource
	}
	if work.Staged != nil && (work.Staged.SizeBytes != row.Size || work.Staged.SHA256 != row.SHA256) {
		return ErrSource
	}
	return r.cfg.ValidateBinary(ctx, row)
}

func (r *Repository) binaryPublicationWork(ctx context.Context, key string, state SnapshotState, row SnapshotRow) (publication, error) {
	var work publication
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), key, &work)
	if err != nil {
		return work, err
	}
	if !found {
		lineage := entity{Epoch: state.Epoch, Baseline: row.SHA256, Seed: true}
		work, err = r.newPublicationHeader(row.Kind, row.ID, "live", &lineage, nil)
		if err != nil {
			return work, err
		}
		work.Input, work.Binary, work.File, work.Size = row.SHA256, true, true, row.Size
		if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
			return work, err
		}
	}
	if err := r.validateBinaryWork(ctx, work, state, row); err != nil {
		return work, err
	}
	if err := ctx.Err(); err != nil {
		return work, err
	}
	return work, r.cfg.Crypto.CheckStoragePublicationTarget(work.Target)
}

func (r *Repository) stageBinary(ctx context.Context, key string, state SnapshotState, row SnapshotRow, work *publication) error {
	return r.stagePublicationFile(ctx, key, row, work, func() (io.ReadCloser, error) { return r.cfg.Source.Open(ctx, r.cfg.Scope, state.Epoch, row) })
}

func (r *Repository) publishBinaryContent(ctx context.Context, key string, work *publication) error {
	metadata, _ := json.Marshal(r.publicationEntity(*work))
	return r.publishFileContent(ctx, key, work, metadata)
}

func (r *Repository) copyBinarySnapshotRow(ctx context.Context, state SnapshotState, row SnapshotRow) error {
	if row.Kind == "snapshot" || row.Validate() != nil || !r.binaryKind(row.Kind) || r.cfg.ValidateBinary == nil {
		return ErrSource
	}
	if err := r.cfg.ValidateBinary(ctx, row); err != nil {
		return err
	}
	if found, err := r.reuseSnapshotRow(ctx, state, row); err != nil || found {
		return err
	}
	key := "import_" + hashBytes([]byte(state.Epoch+"\x00"+row.Kind+"\x00"+row.ID))
	work, err := r.binaryPublicationWork(ctx, key, state, row)
	if err != nil {
		return err
	}
	if err := r.stageBinary(ctx, key, state, row, &work); err != nil {
		return err
	}
	if err := r.publishBinaryContent(ctx, key, &work); err != nil {
		return err
	}
	commit := r.publicationCommit(work)
	before := storage.ObjectVersion{ObjectID: r.object(row.Kind, row.ID), Commit: commit}
	if _, err := r.readBinary(ctx, before, io.Discard); err != nil {
		return err
	}
	actual, err := r.publishIndex(ctx, work, commit)
	if err != nil {
		return err
	}
	if _, err := r.readBinary(ctx, actual, io.Discard); err != nil {
		return err
	}
	work.Index = &actual.Reference
	if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
		return err
	}
	return r.acknowledgeBinary(ctx, key, work, actual, false)
}

func (r *Repository) acknowledgeBinary(ctx context.Context, key string, work publication, accepted storage.ObjectVersion, cleanup bool) error {
	var err error
	if cleanup {
		err = r.acknowledgeReconciledFile(ctx, key+"_file", accepted)
	} else {
		err = r.cfg.Crypto.AcknowledgeStorageFile(ctx, r.cfg.Journal, key+"_file", accepted.Reference)
	}
	if err != nil && !errors.Is(err, storage.ErrUploadMissing) {
		return err
	}
	if cleanup && work.Staged != nil {
		return r.cfg.Journal.DeleteStagedFile(r.owner(), r.stagingNamespace(), *work.Staged)
	}
	return nil
}

func (r *Repository) stagingNamespace() string { return r.cfg.Product + "-private-stage-v1" }
