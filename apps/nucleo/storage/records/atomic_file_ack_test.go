// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// This witness tests the opaque proof handoff. SDK provenance and its real WAL
// require the separate Matrix integration test on the remote environment.
type atomicFileAckProbe struct {
	*atomicProjectionPort
	records     map[storage.Reference]storage.Record
	request     storage.FileRequest
	digest      string
	file        storage.Reference
	atomicCalls int
	legacyCalls int
	accepted    int
	beforeAck   func()
	ackErr      error
	legacyErr   error
	body        []byte
}

func (p *atomicFileAckProbe) ReadStorageRecord(ctx context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	if err := ctx.Err(); err != nil {
		return storage.Record{}, err
	}
	record, ok := p.records[ref]
	if !ok || record.ObjectID != object || record.Role != role {
		return storage.Record{}, storage.ErrIncompleteCatalog
	}
	record.Payload = slices.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	return record, nil
}

func (p *atomicFileAckProbe) StorageFileCommitment(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	if err := ctx.Err(); err != nil {
		return storage.FileRequest{}, "", err
	}
	if ref != p.file || object != p.request.ObjectID || revision != p.request.RevisionID {
		return storage.FileRequest{}, "", ErrSource
	}
	return p.request, p.digest, nil
}

func (p *atomicFileAckProbe) StorageFileInfo(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, error) {
	request, _, err := p.StorageFileCommitment(ctx, ref, object, revision)
	return request, err
}

func (p *atomicFileAckProbe) DownloadStorageFile(ctx context.Context, _ *api.Drive, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	request, _, err := p.StorageFileCommitment(ctx, ref, object, revision)
	if err != nil {
		return storage.FileRequest{}, err
	}
	_, err = out.Write(p.body)
	return request, err
}

func (p *atomicFileAckProbe) AcknowledgeStorageFile(context.Context, *offline.Store, string, storage.Reference) error {
	p.legacyCalls++
	return p.legacyErr
}

func (p *atomicFileAckProbe) AcknowledgeAtomicStorageFile(ctx context.Context, _ *offline.Store, _ string, proof storage.VerifiedAtomicFileCompletion) error {
	p.atomicCalls++
	if p.beforeAck != nil {
		p.beforeAck()
	}
	if _, err := proof.ReadPointer(ctx, p.binding.Scope, p.request, p.file, p.digest); err != nil {
		return err
	}
	if p.ackErr != nil {
		return p.ackErr
	}
	p.accepted++
	return nil
}

type (
	noAtomicFileCapability struct{ Crypto }
	noAtomicAcknowledger   struct {
		Crypto
		binaryCommitmentReader
	}
)

func atomicFileAckFixture(t *testing.T, committed bool) (*Repository, *atomicFileAckProbe, storage.ObjectVersion, storage.Reference) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	room := "!private:test"
	member := storage.Reference{RoomID: room, EventID: "$member"}
	file := storage.Reference{RoomID: room, EventID: "$file"}
	pageRef := storage.Reference{RoomID: room, EventID: "$page"}
	rootRef := storage.Reference{RoomID: room, EventID: "$root"}
	encode := func(value any) []byte {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{"private":true}`), Content: &storage.ContentPointer{Kind: "file", RevisionID: "file_revision", Reference: file}}
	entry := storage.RecordEntry{Reference: member, Record: storage.Record{Version: 1, Scope: scope, ObjectID: "object", RevisionID: "index_revision", Role: "index", Payload: encode(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: "batch", Commit: commit})}}
	memberSHA, err := storage.AtomicRecordSHA256(entry.Record)
	if err != nil {
		t.Fatal(err)
	}
	page := storage.Record{Version: 1, Scope: scope, ObjectID: storage.AtomicBatchObjectID("batch"), RevisionID: "page", Role: "manifest", Payload: encode(storage.AtomicBatchPage{Kind: storage.ObjectBatchPageKind, BatchID: "batch", Members: []storage.AtomicMember{{Reference: member, ObjectID: "object", RevisionID: "index_revision", SHA256: memberSHA}}})}
	pageSHA, err := storage.AtomicRecordSHA256(page)
	if err != nil {
		t.Fatal(err)
	}
	root := storage.RecordEntry{Reference: rootRef, Record: storage.Record{Version: 1, Scope: scope, ObjectID: storage.AtomicBatchObjectID("batch"), RevisionID: "root", Role: "index", Payload: encode(storage.AtomicBatchCommit{Kind: storage.ObjectBatchKind, BatchID: "batch", Count: 1, Last: storage.AtomicPagePointer{Reference: pageRef, RevisionID: "page", SHA256: pageSHA}})}}
	port := &atomicFileAckProbe{atomicProjectionPort: &atomicProjectionPort{binding: storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: room, DeviceID: "DEVICE"}, Rooms: []string{room}}, entries: []storage.RecordEntry{entry}}, records: map[storage.Reference]storage.Record{member: entry.Record, pageRef: page, rootRef: root.Record}, request: storage.FileRequest{ObjectID: "object", RevisionID: "file_revision", SizeBytes: 7, Metadata: []byte(`{"file":true}`)}, digest: strings.Repeat("a", 64), file: file}
	if committed {
		port.entries = append(port.entries, root)
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	r := &Repository{cfg: Config{Scope: scope, Product: scope.Product, Crypto: port, Journal: journal, Gateway: &api.Drive{}}}
	if err := r.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, actual, err := storage.DecodeAtomicPrepared(entry, scope)
	if err != nil {
		t.Fatal(err)
	}
	return r, port, actual, pageRef
}

func TestReconciledAtomicFileRequiresCompleteLiveProof(t *testing.T) {
	for _, scenario := range []string{"valid", "orphan", "no capability", "no atomic acknowledger", "missing page", "changed root", "changed page", "changed member", "changed binding", "wrong commitment", "forged version", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			r, port, actual, page := atomicFileAckFixture(t, scenario != "orphan")
			defer r.Close()
			ctx := t.Context()
			switch scenario {
			case "no capability":
				r.cfg.Crypto = noAtomicFileCapability{Crypto: port}
			case "no atomic acknowledger":
				r.cfg.Crypto = noAtomicAcknowledger{Crypto: port, binaryCommitmentReader: port}
			case "missing page":
				delete(port.records, page)
			case "changed root":
				ref := storage.Reference{RoomID: actual.Reference.RoomID, EventID: "$root"}
				port.beforeAck = func() { v := port.records[ref]; v.Payload = append(v.Payload, ' '); port.records[ref] = v }
			case "changed page":
				port.beforeAck = func() { v := port.records[page]; v.Payload = append(v.Payload, ' '); port.records[page] = v }
			case "changed member":
				port.beforeAck = func() {
					v := port.records[actual.Reference]
					v.Payload = append(v.Payload, ' ')
					port.records[actual.Reference] = v
				}
			case "changed binding":
				port.beforeAck = func() { port.binding.Target.DeviceID = "OTHER" }
			case "wrong commitment":
				port.beforeAck = func() { port.digest = strings.Repeat("b", 64) }
			case "forged version":
				actual.Commit.Metadata = []byte(`{"private":false}`)
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := r.acknowledgeReconciledFile(ctx, "job", actual)
			if scenario == "valid" {
				if err != nil || port.atomicCalls != 1 || port.accepted != 1 || port.legacyCalls != 0 {
					t.Fatal("valid prepared member was not acknowledged with proof", err)
				}
			} else if err == nil || port.accepted != 0 || port.legacyCalls != 0 {
				t.Fatal("unverified atomic file was acknowledged", scenario, err)
			}
			if scenario == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation was not propagated", err)
			}
		})
	}
}

func TestReconciledOrdinaryFileKeepsLegacyAcknowledgment(t *testing.T) {
	r, port, actual, _ := atomicFileAckFixture(t, false)
	defer r.Close()
	ref := storage.Reference{RoomID: actual.Reference.RoomID, EventID: "$ordinary"}
	payload, err := json.Marshal(actual.Commit)
	if err != nil {
		t.Fatal(err)
	}
	entry := storage.RecordEntry{Reference: ref, Record: storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: actual.ObjectID, RevisionID: "ordinary_revision", Role: "index", Payload: payload}}
	port.entries = []storage.RecordEntry{entry}
	if err := r.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	actual, err = storage.DecodeObject(entry, r.cfg.Scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.acknowledgeReconciledFile(t.Context(), "job", actual); err != nil || port.legacyCalls != 1 || port.atomicCalls != 0 {
		t.Fatal("ordinary accepted revision did not use legacy acknowledgment", err)
	}
	staged, err := r.cfg.Journal.StageFile(t.Context(), r.owner(), r.stagingNamespace(), bytes.NewReader([]byte("legacy")), 6)
	if err != nil {
		t.Fatal(err)
	}
	port.legacyErr = storage.ErrUploadMissing
	if err := r.acknowledgeBinary(t.Context(), "job", publication{Staged: &staged}, actual, true); err != nil {
		t.Fatal("legacy missing-journal behavior changed", err)
	}
	if reader, err := r.cfg.Journal.OpenStagedFile(t.Context(), r.owner(), r.stagingNamespace(), staged); !errors.Is(err, offline.ErrStagedFile) {
		if reader != nil {
			_ = reader.Close()
		}
		t.Fatal("legacy accepted staging was not retired", err)
	}
}

func TestReconcileAtomicBinaryRetainsStagingUntilAcknowledged(t *testing.T) {
	for _, scenario := range []string{"accepted", "missing upload journal", "changed page", "canceled before acknowledgment"} {
		t.Run(scenario, func(t *testing.T) {
			r, port, actual, pageRef := atomicFileAckFixture(t, true)
			defer r.Close()
			r.cfg.Product = "photos"
			r.cfg.EntityKind = "photos.entity"
			r.cfg.BinaryKind = func(kind string) bool { return kind == "original" }
			r.cfg.ValidateBinary = func(_ context.Context, row SnapshotRow) error { return row.Validate() }
			port.body = []byte("payload")
			staged, err := r.cfg.Journal.StageFile(t.Context(), r.owner(), r.stagingNamespace(), bytes.NewReader(port.body), int64(len(port.body)))
			if err != nil {
				t.Fatal(err)
			}
			state := SnapshotState{LegacyMigrationState: storage.LegacyMigrationState{Scope: r.cfg.Scope, Epoch: "11111111-1111-1111-1111-111111111111"}}
			work := publication{Version: 1, Scope: r.cfg.Scope, Kind: "original", ID: "asset", Revision: "file_revision", Input: staged.SHA256, State: "live", Target: port.binding.Target, Content: &port.file, Index: &actual.Reference, Epoch: state.Epoch, Baseline: staged.SHA256, Seed: true, File: true, Binary: true, Size: int64(len(port.body)), Staged: &staged}
			object := r.object(work.Kind, work.ID)
			commit := r.publicationCommit(work)
			payload, err := json.Marshal(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: "batch", Commit: commit})
			if err != nil {
				t.Fatal(err)
			}
			entry := storage.RecordEntry{Reference: actual.Reference, Record: storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: object, RevisionID: work.Revision + "_index", Role: "index", Payload: payload}}
			memberSHA, err := storage.AtomicRecordSHA256(entry.Record)
			if err != nil {
				t.Fatal(err)
			}
			page := port.records[pageRef]
			page.Payload, err = json.Marshal(storage.AtomicBatchPage{Kind: storage.ObjectBatchPageKind, BatchID: "batch", Members: []storage.AtomicMember{{Reference: entry.Reference, ObjectID: object, RevisionID: entry.Record.RevisionID, SHA256: memberSHA}}})
			if err != nil {
				t.Fatal(err)
			}
			pageSHA, err := storage.AtomicRecordSHA256(page)
			if err != nil {
				t.Fatal(err)
			}
			root := port.entries[1]
			root.Record.Payload, err = json.Marshal(storage.AtomicBatchCommit{Kind: storage.ObjectBatchKind, BatchID: "batch", Count: 1, Last: storage.AtomicPagePointer{Reference: pageRef, RevisionID: "page", SHA256: pageSHA}})
			if err != nil {
				t.Fatal(err)
			}
			port.entries = []storage.RecordEntry{entry, root}
			port.records[entry.Reference], port.records[pageRef], port.records[root.Reference] = entry.Record, page, root.Record
			port.request = storage.FileRequest{ObjectID: object, RevisionID: work.Revision, SizeBytes: work.Size, Metadata: slices.Clone(commit.Metadata)}
			port.digest = staged.SHA256
			if err := r.refresh(t.Context()); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch scenario {
			case "missing upload journal":
				port.ackErr = storage.ErrUploadMissing
			case "changed page":
				port.beforeAck = func() {
					value := port.records[pageRef]
					value.Payload = append(value.Payload, ' ')
					port.records[pageRef] = value
				}
			case "canceled before acknowledgment":
				port.beforeAck = cancel
			}
			key := "import_" + hashBytes([]byte(state.Epoch+"\x00"+work.Kind+"\x00"+work.ID))
			err = r.reconcileSnapshotPublication(ctx, key, state, work)
			reader, openErr := r.cfg.Journal.OpenStagedFile(t.Context(), r.owner(), r.stagingNamespace(), staged)
			if reader != nil {
				_ = reader.Close()
			}
			if scenario == "accepted" {
				if err != nil || !errors.Is(openErr, offline.ErrStagedFile) || port.atomicCalls != 1 {
					t.Fatal("verified reconciliation did not retire staging", err, openErr)
				}
			} else if err == nil || openErr != nil || port.accepted != 0 {
				t.Fatal("failed verification removed staging or accepted upload", err, openErr)
			}
			if scenario == "missing upload journal" && !errors.Is(err, ErrSource) {
				t.Fatal("missing atomic WAL was treated as proof of completion", err)
			}
			if port.legacyCalls != 0 {
				t.Fatal("prepared index reached legacy acknowledgment")
			}
		})
	}
}
