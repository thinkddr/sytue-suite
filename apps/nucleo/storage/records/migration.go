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
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	catalogtransfer "git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

// SnapshotState is a retained source's explicit versioned contract. A source
// with shared data cannot be silently converted into a personal repository.
type SnapshotState = storage.AppMigrationState

type SnapshotRow = storage.StreamSourceRow

type SnapshotPage = storage.StreamSourcePage

// Source exposes consistent retained JSON, including trash and auxiliary rows.
// Its implementation owns the write fence. Ordinary reads never call Page/Open
// and an absent completion never triggers an automatic plaintext reimport.
type Source interface {
	State(context.Context, storage.Scope) (SnapshotState, error)
	Begin(context.Context, storage.Scope, string) (SnapshotState, error)
	Page(context.Context, storage.Scope, string, storage.LegacySourceCursor) (SnapshotPage, error)
	Open(context.Context, storage.Scope, string, SnapshotRow) (io.ReadCloser, error)
}

// SnapshotEpochSource is optional. A product that retained an encrypted plan
// before Begin can recover that accepted epoch on a new authorized device.
// It never changes a preexisting upload intent or reinterprets legacy as frozen.
type SnapshotEpochSource = catalogtransfer.SourceEpochProvider

type snapshotWork = catalogtransfer.SourceBeginIntent

type snapshotReceipt struct {
	Kind      string `json:"kind"`
	Epoch     string `json:"epoch"`
	Rows      int64  `json:"rows"`
	Inventory string `json:"inventory"`
	Blobs     int64  `json:"blobs,omitempty"`
}

func (r *Repository) validateState(state SnapshotState) error {
	if state.Version != 1 || state.Validate(r.cfg.Scope) != nil || state.Blobs != 0 && (r.cfg.BinaryKind == nil || r.cfg.ValidateBinary == nil) {
		return ErrSource
	}
	if state.Shared {
		return ErrShared
	}
	return nil
}

// Initialize opens an authenticated, completely imported private catalog.
// Media commitments verify preserved identity, not current provider availability;
// ReadBinary and AuditBinaries verify complete bytes. Import is explicit and its
// receipt is published only after the initial complete byte verification.
func (r *Repository) Initialize(parent context.Context) error {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	return r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), "catalog", func() error {
		r.mu.Lock()
		r.ready = false
		r.mu.Unlock()
		if _, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope); err != nil {
			return err
		}
		state, err := r.cfg.Source.State(ctx, r.cfg.Scope)
		if err != nil {
			return err
		}
		if err := r.validateState(state); err != nil {
			return err
		}
		if state.Mode == "legacy" {
			return ErrNotReady
		}
		return r.openAndReconcile(ctx, state)
	})
}

func (r *Repository) completedReceipt(ctx context.Context, state SnapshotState) (snapshotReceipt, error) {
	r.mu.Lock()
	versions := slices.Clone(r.heads[r.cfg.Product+"_snapshot_"+hashBytes([]byte(state.Epoch))])
	r.mu.Unlock()
	if len(versions) == 0 {
		return snapshotReceipt{}, ErrNotReady
	}
	if len(versions) != 1 {
		return snapshotReceipt{}, ErrConflict
	}
	version := versions[0]
	var receipt snapshotReceipt
	if json.Unmarshal(version.Commit.Metadata, &receipt) != nil || receipt.Kind != r.cfg.ReceiptKind || receipt.Epoch != state.Epoch || receipt.Rows != state.Rows || receipt.Blobs != state.Blobs || len(receipt.Inventory) != 64 || version.Commit.State != "live" || version.Commit.Content == nil || version.Commit.Content.Kind != "record" {
		return receipt, ErrSource
	}
	record, err := r.cfg.Crypto.ReadStorageRecord(ctx, version.Commit.Content.Reference, version.ObjectID, "content")
	if err != nil {
		return receipt, err
	}
	defer clear(record.Payload)
	if record.Validate(r.cfg.Scope) != nil || record.RevisionID != version.Commit.Content.RevisionID || !bytes.Equal(record.Payload, version.Commit.Metadata) {
		return receipt, ErrSource
	}
	return receipt, nil
}

func compareEntities(a, b entity) int {
	if a.Entity < b.Entity {
		return -1
	}
	if a.Entity > b.Entity {
		return 1
	}
	if a.ID < b.ID {
		return -1
	}
	if a.ID > b.ID {
		return 1
	}
	return 0
}

func (r *Repository) baselineInventory(ctx context.Context, state SnapshotState) (int64, int64, string, error) {
	r.mu.Lock()
	history := make([]storage.ObjectVersion, len(r.catalog.History))
	for i, version := range r.catalog.History {
		history[i] = cloneVersion(version)
	}
	r.mu.Unlock()
	baseline := []entity{}
	seen := map[string]bool{}
	var blobs int64
	for _, version := range history {
		var summary entity
		if json.Unmarshal(version.Commit.Metadata, &summary) != nil {
			return 0, 0, "", ErrSource
		}
		if summary.Kind != r.cfg.EntityKind || summary.Epoch != state.Epoch || !summary.Seed {
			continue
		}
		if _, err := r.description(version); err != nil {
			return 0, 0, "", err
		}
		if summary.Baseline != summary.SHA256 || seen[version.ObjectID] {
			return 0, 0, "", ErrSource
		}
		seen[version.ObjectID] = true
		if summary.Binary {
			if _, err := r.verifyBinaryCommitment(ctx, version); err != nil {
				return 0, 0, "", err
			}
			blobs++
		} else {
			raw, err := r.readJSON(ctx, version)
			clear(raw)
			if err != nil {
				return 0, 0, "", err
			}
		}
		baseline = append(baseline, summary)
	}
	slices.SortFunc(baseline, compareEntities)
	digest := r.newSnapshotInventory(state.Epoch)
	for _, row := range baseline {
		_, _ = digest.Write([]byte(row.Entity + "\x00" + row.ID + "\x00" + row.SHA256 + "\x00"))
	}
	return int64(len(baseline)), blobs, hex.EncodeToString(digest.Sum(nil)), nil
}

func (r *Repository) newSnapshotInventory(epoch string) hash.Hash {
	digest := sha256.New()
	_, _ = digest.Write([]byte("org.sytue." + r.cfg.Product + ".inventory.v1\x00" + epoch + "\x00"))
	return digest
}

func (r *Repository) verifyCompleted(ctx context.Context, state SnapshotState) error {
	if err := r.refresh(ctx); err != nil {
		return err
	}
	receipt, err := r.completedReceipt(ctx, state)
	if err != nil {
		return err
	}
	rows, blobs, inventory, err := r.baselineInventory(ctx, state)
	if err != nil {
		return err
	}
	if rows != receipt.Rows || blobs != receipt.Blobs || inventory != receipt.Inventory {
		return ErrSource
	}
	if err := r.cfg.ValidateCatalog(ctx, catalogView{r: r}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Repository) openCompleted(ctx context.Context, state SnapshotState) error {
	if err := r.verifyCompleted(ctx, state); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	r.ready = true
	return nil
}

func (r *Repository) prepareSnapshot(ctx context.Context) (SnapshotState, snapshotWork, error) {
	binding, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope)
	if err != nil {
		return SnapshotState{}, snapshotWork{}, err
	}
	if !binding.Valid() || binding.Scope != r.cfg.Scope {
		return SnapshotState{}, snapshotWork{}, ErrSource
	}
	state, err := r.cfg.Source.State(ctx, r.cfg.Scope)
	if err != nil {
		return state, snapshotWork{}, err
	}
	if err := r.validateState(state); err != nil {
		return state, snapshotWork{}, err
	}
	accepted, work, err := catalogtransfer.BeginSource(ctx, catalogtransfer.SourceBeginConfig{
		Scope: r.cfg.Scope, Owner: r.owner(), Namespace: r.namespace(), IntentKey: "snapshot", Journal: r.cfg.Journal,
		Current: state.LegacyMigrationState, Target: binding.Target, CheckTarget: r.cfg.Crypto.CheckStoragePublicationTarget,
		NewEpoch: func(ctx context.Context) (string, error) { return r.newSnapshotEpoch(ctx, state) },
		Begin: func(ctx context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
			next, err := r.cfg.Source.Begin(ctx, scope, epoch)
			if err != nil {
				return next.LegacyMigrationState, err
			}
			state = next
			if err := r.validateState(state); err != nil {
				return next.LegacyMigrationState, err
			}
			return next.LegacyMigrationState, nil
		},
	})
	if err != nil {
		return state, work, transferError(err)
	}
	state.LegacyMigrationState = accepted
	if err := r.validateState(state); err != nil {
		return state, work, err
	}
	return state, work, nil
}

func (r *Repository) newSnapshotEpoch(ctx context.Context, state SnapshotState) (string, error) {
	if state.Mode != "legacy" {
		return state.Epoch, nil
	}
	if source, ok := r.cfg.Source.(SnapshotEpochSource); ok {
		epoch, err := source.SnapshotEpoch(ctx, r.cfg.Scope)
		if err != nil {
			return "", err
		}
		if epoch != "" {
			if !storage.MigrationEpoch(epoch) {
				return "", ErrSource
			}
			return epoch, nil
		}
	}
	return uuid.NewString(), nil
}

func (r *Repository) copySnapshotRow(ctx context.Context, state SnapshotState, row SnapshotRow) error {
	if r.binaryKind(row.Kind) {
		return r.copyBinarySnapshotRow(ctx, state, row)
	}
	if !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}) || row.Kind == "snapshot" || row.Size < 2 || row.Size > MaxEntityJSON {
		return ErrSource
	}
	if found, err := r.reuseSnapshotRow(ctx, state, row); err != nil || found {
		return err
	}
	stream, err := r.cfg.Source.Open(ctx, r.cfg.Scope, state.Epoch, row)
	if err != nil {
		return err
	}
	raw, readErr := io.ReadAll(io.LimitReader(stream, row.Size+1))
	_ = stream.Close()
	defer clear(raw)
	if readErr != nil {
		return readErr
	}
	if int64(len(raw)) != row.Size || hashBytes(raw) != row.SHA256 || !json.Valid(raw) {
		return ErrSource
	}
	if err := r.cfg.ValidateRow(ctx, row.Kind, row.ID, raw); err != nil {
		return err
	}
	lineage := entity{Epoch: state.Epoch, Baseline: row.SHA256, Seed: true}
	stateValue, err := rawState(raw)
	if err != nil {
		return err
	}
	err = r.publishJSON(ctx, "import_"+hashBytes([]byte(state.Epoch+"\x00"+row.Kind+"\x00"+row.ID)), row.Kind, row.ID, stateValue, raw, &lineage, nil)
	return err
}

func (r *Repository) copySnapshot(ctx context.Context, state SnapshotState) (snapshotReceipt, error) {
	cursor := storage.LegacySourceCursor{}
	sum := r.newSnapshotInventory(state.Epoch)
	var count, blobs int64
	for {
		page, err := r.cfg.Source.Page(ctx, r.cfg.Scope, state.Epoch, cursor)
		if err != nil {
			return snapshotReceipt{}, err
		}
		if page.Validate(r.cfg.Scope, state.Epoch, cursor) != nil {
			return snapshotReceipt{}, ErrSource
		}
		for _, row := range page.Rows {
			if err := r.copySnapshotRow(ctx, state, row); err != nil {
				return snapshotReceipt{}, err
			}
			_, _ = sum.Write([]byte(row.Kind + "\x00" + row.ID + "\x00" + row.SHA256 + "\x00"))
			count++
			if r.binaryKind(row.Kind) {
				blobs++
			}
			if count > state.Rows || blobs > state.Blobs {
				return snapshotReceipt{}, ErrSource
			}
			cursor = storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}
		}
		if page.Next == nil {
			break
		}
	}
	if count != state.Rows || blobs != state.Blobs {
		return snapshotReceipt{}, ErrSource
	}
	return snapshotReceipt{Kind: r.cfg.ReceiptKind, Epoch: state.Epoch, Rows: count, Blobs: blobs, Inventory: hex.EncodeToString(sum.Sum(nil))}, nil
}

// ImportLegacySnapshot is explicit and never deletes the retained source.
func (r *Repository) ImportLegacySnapshot(parent context.Context) error {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	return r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), "catalog", func() error {
		r.mu.Lock()
		r.ready = false
		r.mu.Unlock()
		state, work, err := r.prepareSnapshot(ctx)
		if err != nil {
			return err
		}
		if err := r.openAndReconcile(ctx, state); err == nil {
			return nil
		} else if !errors.Is(err, ErrNotReady) {
			return err
		}
		if state.Mode != "frozen" {
			return ErrNotReady
		}
		receipt, err := r.copySnapshot(ctx, state)
		if err != nil {
			return err
		}
		current, err := r.cfg.Source.State(ctx, r.cfg.Scope)
		if err != nil {
			return err
		}
		if r.validateState(current) != nil || current.Mode != "frozen" || current.Epoch != state.Epoch || current.Rows != state.Rows || current.Blobs != state.Blobs {
			return ErrSource
		}
		if err := r.refresh(ctx); err != nil {
			return err
		}
		if err := r.cfg.ValidateCatalog(ctx, catalogView{r: r}); err != nil {
			return err
		}
		if err := r.publishReceipt(ctx, work, receipt); err != nil {
			return err
		}
		return r.openAndReconcile(ctx, state)
	})
}

func rawState(raw []byte) (string, error) {
	var row struct {
		DeletedAt *time.Time `json:"deleted_at"`
	}
	if json.Unmarshal(raw, &row) != nil {
		return "", ErrSource
	}
	if row.DeletedAt != nil {
		return "trash", nil
	}
	return "live", nil
}

func (r *Repository) publishReceipt(ctx context.Context, work snapshotWork, receipt snapshotReceipt) error {
	payload, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	obj := r.cfg.Product + "_snapshot_" + hashBytes([]byte(work.Epoch))
	revision := work.Epoch
	content, err := r.cfg.Crypto.PutStorageRecordAt(ctx, work.Target, "snapshot_"+revision+"_content", storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: obj, RevisionID: revision, Role: "content", Payload: payload})
	if err != nil {
		return err
	}
	checked, err := r.cfg.Crypto.ReadStorageRecord(ctx, content, obj, "content")
	if err != nil {
		return err
	}
	if checked.Validate(r.cfg.Scope) != nil || checked.RevisionID != revision || !bytes.Equal(checked.Payload, payload) {
		return ErrSource
	}
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: payload, Content: &storage.ContentPointer{Kind: "record", RevisionID: revision, Reference: content}}
	version, err := r.cfg.Crypto.PutStorageObjectAt(ctx, work.Target, "snapshot_"+revision+"_index", obj, revision+"_index", commit)
	if err != nil {
		return err
	}
	actual, err := r.cfg.Crypto.StorageObject(ctx, version.Reference, obj)
	if err != nil {
		return err
	}
	a, _ := json.Marshal(actual.Commit)
	b, _ := json.Marshal(commit)
	if actual.ObjectID != obj || actual.RevisionID != revision+"_index" || !bytes.Equal(a, b) {
		return ErrSource
	}
	return nil
}

// A fresh authorized device has no previous device's journal. Reuse only one
// exact immutable seed, and leave all unexpected histories or branches intact.
func (r *Repository) reuseSnapshotRow(ctx context.Context, state SnapshotState, row SnapshotRow) (bool, error) {
	obj := r.object(row.Kind, row.ID)
	r.mu.Lock()
	versions := make([]storage.ObjectVersion, 0, len(r.byObject[obj]))
	for _, version := range r.byObject[obj] {
		versions = append(versions, cloneVersion(version))
	}
	heads := slices.Clone(r.heads[obj])
	r.mu.Unlock()
	if len(versions) == 0 {
		return false, nil
	}
	if len(versions) != 1 || len(heads) != 1 || versions[0].Reference != heads[0].Reference {
		return false, ErrConflict
	}
	seed := versions[0]
	summary, err := r.description(seed)
	if err != nil {
		return false, err
	}
	if !summary.Seed || summary.Epoch != state.Epoch || summary.Entity != row.Kind || summary.ID != row.ID || summary.Baseline != row.SHA256 || summary.SHA256 != row.SHA256 {
		return false, ErrSource
	}
	if summary.Binary {
		actual, err := r.readBinary(ctx, seed, io.Discard)
		if err != nil {
			return false, err
		}
		if actual != row || seed.Commit.State != "live" {
			return false, ErrSource
		}
		return true, nil
	}
	return r.verifyJSONSnapshotSeed(ctx, seed, row)
}

func (r *Repository) verifyJSONSnapshotSeed(ctx context.Context, seed storage.ObjectVersion, row SnapshotRow) (bool, error) {
	raw, err := r.readJSON(ctx, seed)
	if err != nil {
		return false, err
	}
	defer clear(raw)
	actual, err := rawState(raw)
	if err != nil || actual != seed.Commit.State || int64(len(raw)) != row.Size || hashBytes(raw) != row.SHA256 {
		return false, ErrSource
	}
	return true, nil
}
