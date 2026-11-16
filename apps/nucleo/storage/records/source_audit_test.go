// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type sourceAuditFixture struct {
	Source
	scope   storage.Scope
	epoch   string
	row     SnapshotRow
	body    []byte
	stream  io.ReadCloser
	openErr error
}

func (s *sourceAuditFixture) Page(_ context.Context, _ storage.Scope, _ string, _ storage.LegacySourceCursor) (SnapshotPage, error) {
	return SnapshotPage{Scope: s.scope, Epoch: s.epoch, Rows: []SnapshotRow{s.row}}, nil
}

func (s *sourceAuditFixture) Open(context.Context, storage.Scope, string, SnapshotRow) (io.ReadCloser, error) {
	if s.stream != nil {
		return s.stream, s.openErr
	}
	return io.NopCloser(bytes.NewReader(s.body)), nil
}

type trackedAuditStream struct {
	io.Reader
	closes   atomic.Int32
	closeErr error
}

func (s *trackedAuditStream) Close() error {
	s.closes.Add(1)
	return s.closeErr
}

func TestAuditRetainedSourceClosesStreamOnOpenOrCloseError(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "cast"}
	epoch := "11111111-1111-1111-1111-111111111111"
	body := []byte("source")
	row := SnapshotRow{Kind: "media_bytes", ID: "one", Size: int64(len(body)), SHA256: hashBytes(body)}
	for _, scenario := range []string{"open returns body and error", "close returns error"} {
		t.Run(scenario, func(t *testing.T) {
			failure := errors.New("synthetic source failure")
			stream := &trackedAuditStream{Reader: bytes.NewReader(body)}
			fixture := &sourceAuditFixture{scope: scope, epoch: epoch, row: row, stream: stream}
			if scenario == "open returns body and error" {
				fixture.openErr = failure
			} else {
				stream.closeErr = failure
			}
			r := &Repository{cfg: Config{Scope: scope, Product: "cast", Source: fixture, BinaryKind: func(k string) bool { return k == "media_bytes" }}}
			state := SnapshotState{LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Epoch: epoch, Rows: 1, Blobs: 1}}
			got, err := r.auditRetainedSource(t.Context(), state, snapshotReceipt{})
			if !errors.Is(err, failure) || got != (SourceAudit{}) {
				t.Fatal("failed source returned an observation", got, err)
			}
			if stream.closes.Load() != 1 {
				t.Fatal("source stream not closed exactly once", stream.closes.Load())
			}
		})
	}
}

type blockedAuditReader struct {
	once            sync.Once
	started, closed chan struct{}
}

func (b *blockedAuditReader) Read([]byte) (int, error) {
	close(b.started)
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *blockedAuditReader) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

func TestAuditRetainedSourceCancellationClosesBlockedStream(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "cast"}
	epoch := "11111111-1111-1111-1111-111111111111"
	body := &blockedAuditReader{started: make(chan struct{}), closed: make(chan struct{})}
	row := SnapshotRow{Kind: "media_bytes", ID: "one", Size: 1, SHA256: hashBytes([]byte("x"))}
	source := &sourceAuditFixture{scope: scope, epoch: epoch, row: row, stream: body}
	r := &Repository{cfg: Config{Scope: scope, Product: "cast", Source: source, BinaryKind: func(k string) bool { return k == "media_bytes" }}}
	state := SnapshotState{LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Epoch: epoch, Rows: 1, Blobs: 1}}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := r.auditRetainedSource(ctx, state, snapshotReceipt{}); done <- err }()
	<-body.started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation failed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked stream survived cancellation")
	}
}

func TestAuditRetainedSourceComparesActualBytesAndReceipt(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "cast"}
	epoch := "11111111-1111-1111-1111-111111111111"
	original := []byte("retained bytes")
	row := SnapshotRow{Kind: "media_bytes", ID: "one", Size: int64(len(original)), SHA256: hashBytes(original)}
	for _, scenario := range []string{"valid", "corrupt", "short", "extra", "receipt", "count", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := &sourceAuditFixture{scope: scope, epoch: epoch, row: row, body: bytes.Clone(original)}
			r := &Repository{cfg: Config{Scope: scope, Product: "cast", Source: fixture, BinaryKind: func(k string) bool { return k == "media_bytes" }}}
			state := SnapshotState{LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Epoch: epoch, Rows: 1, Blobs: 1}}
			sum := r.newSnapshotInventory(epoch)
			_, _ = sum.Write([]byte(row.Kind + "\x00" + row.ID + "\x00" + row.SHA256 + "\x00"))
			receipt := snapshotReceipt{Rows: 1, Blobs: 1, Inventory: hex.EncodeToString(sum.Sum(nil))}
			ctx := t.Context()
			switch scenario {
			case "corrupt":
				fixture.body[0] ^= 1
			case "short":
				fixture.body = fixture.body[:len(fixture.body)-1]
			case "extra":
				fixture.body = append(fixture.body, 'x')
			case "receipt":
				receipt.Inventory = hashBytes([]byte("different"))
			case "count":
				state.Rows = 2
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			got, err := r.auditRetainedSource(ctx, state, receipt)
			if scenario == "valid" {
				if err != nil || got.Rows != 1 || got.Blobs != 1 || got.SourceBytes != int64(len(original)) {
					t.Fatal(got, err)
				}
			} else if !errors.Is(err, ErrSource) && !errors.Is(err, context.Canceled) {
				t.Fatal("changed source accepted", got, err)
			}
		})
	}
}

type sourceAuditAtomicPort struct {
	*atomicProjectionPort
	targetErr error
}

func TestSourceAuditCatalogDetectsChangedPreviousAndRole(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "cast"}
	binding := storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: "!room:test", DeviceID: "DEVICE"}, Rooms: []string{"!room:test"}}
	ref := storage.Reference{RoomID: "!room:test", EventID: "$event"}
	entry := storage.RecordEntry{Reference: ref, Record: storage.Record{Version: 1, Scope: scope, ObjectID: "object", RevisionID: "revision", Role: "index", Previous: []string{"earlier"}, Payload: []byte("payload")}}
	before := storage.AtomicCatalogSnapshot{Binding: binding, Entries: map[storage.Reference]storage.RecordEntry{ref: entry}}
	after := storage.AtomicCatalogSnapshot{Binding: binding, Entries: map[storage.Reference]storage.RecordEntry{ref: entry}}
	if !storage.SameAtomicCatalog(before, after) {
		t.Fatal("identical catalog changed")
	}
	entry.Record.Previous = []string{"different"}
	after.Entries[ref] = entry
	if storage.SameAtomicCatalog(before, after) {
		t.Fatal("changed ancestry accepted")
	}
	entry.Record.Previous = []string{"earlier"}
	entry.Record.Role = "content"
	after.Entries[ref] = entry
	if storage.SameAtomicCatalog(before, after) {
		t.Fatal("changed role accepted")
	}
}

func (p *sourceAuditAtomicPort) CheckStoragePublicationTarget(storage.PublicationTarget) error {
	return p.targetErr
}

type sourceAuditState struct {
	Source
	state SnapshotState
	err   error
}

func (s *sourceAuditState) State(context.Context, storage.Scope) (SnapshotState, error) {
	return s.state, s.err
}

func TestFinishSourceAuditChecksCompleteAtomicObservation(t *testing.T) {
	for _, scenario := range []string{"unchanged", "orphan", "orphan added", "page and root changed", "missing page", "corrupt page", "missing member", "changed state", "target error", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "cast"}
			room := "!room:test"
			memberRef := storage.Reference{RoomID: room, EventID: "$member"}
			pageRef := storage.Reference{RoomID: room, EventID: "$page"}
			encode := func(value any) []byte {
				t.Helper()
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				return raw
			}
			member := storage.RecordEntry{Reference: memberRef, Record: storage.Record{Version: 1, Scope: scope, ObjectID: "object", RevisionID: "revision", Role: "index", Payload: encode(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: "batch", Commit: storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{"kind":"domain"}`)}})}}
			memberSHA, err := storage.AtomicRecordSHA256(member.Record)
			if err != nil {
				t.Fatal(err)
			}
			page := storage.Record{Version: 1, Scope: scope, ObjectID: storage.AtomicBatchObjectID("batch"), RevisionID: "page", Role: "manifest", Payload: encode(storage.AtomicBatchPage{Kind: storage.ObjectBatchPageKind, BatchID: "batch", Members: []storage.AtomicMember{{Reference: memberRef, ObjectID: "object", RevisionID: "revision", SHA256: memberSHA}}})}
			pageSHA, err := storage.AtomicRecordSHA256(page)
			if err != nil {
				t.Fatal(err)
			}
			batch := storage.AtomicBatchCommit{Kind: storage.ObjectBatchKind, BatchID: "batch", Count: 1, Last: storage.AtomicPagePointer{Reference: pageRef, RevisionID: "page", SHA256: pageSHA}}
			root := storage.RecordEntry{Reference: storage.Reference{RoomID: room, EventID: "$root"}, Record: storage.Record{Version: 1, Scope: scope, ObjectID: storage.AtomicBatchObjectID("batch"), RevisionID: "root", Role: "index", Payload: encode(batch)}}
			orphan := storage.RecordEntry{Reference: storage.Reference{RoomID: room, EventID: "$orphan"}, Record: storage.Record{Version: 1, Scope: scope, ObjectID: "other", RevisionID: "revision", Role: "index", Payload: encode(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: "orphan", Commit: storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{"kind":"domain"}`)}})}}
			p := &sourceAuditAtomicPort{atomicProjectionPort: &atomicProjectionPort{binding: storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: room, DeviceID: "DEVICE"}, Rooms: []string{room}}, entries: []storage.RecordEntry{member, root}, pages: map[storage.Reference]storage.Record{pageRef: page}}}
			if scenario == "orphan" {
				p.entries = append(p.entries, orphan)
			}
			now := time.Now().UTC()
			state := SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: "11111111-1111-1111-1111-111111111111", StartedAt: &now}}
			source := &sourceAuditState{state: state}
			r := &Repository{cfg: Config{Scope: scope, Crypto: p, Source: source}}
			before, err := storage.LoadAtomicCatalog(t.Context(), p, scope)
			if err != nil {
				t.Fatal(err)
			}
			ctx := t.Context()
			switch scenario {
			case "orphan added":
				p.entries = append(p.entries, orphan)
			case "page and root changed":
				page.Payload = append(bytes.Clone(page.Payload), ' ')
				p.pages[pageRef] = page
				batch.Last.SHA256, err = storage.AtomicRecordSHA256(page)
				if err != nil {
					t.Fatal(err)
				}
				root.Record.Payload = encode(batch)
				p.entries[1] = root
			case "missing page":
				delete(p.pages, pageRef)
			case "corrupt page":
				page.Payload = append(bytes.Clone(page.Payload), ' ')
				p.pages[pageRef] = page
			case "missing member":
				p.entries = []storage.RecordEntry{root}
			case "changed state":
				source.state.Epoch = "22222222-2222-2222-2222-222222222222"
			case "target error":
				p.targetErr = ErrSource
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if scenario == "orphan added" || scenario == "page and root changed" {
				after, err := storage.LoadAtomicCatalog(t.Context(), p, scope)
				if err != nil || !reflect.DeepEqual(before.History, after.History) || !reflect.DeepEqual(before.Heads, after.Heads) {
					t.Fatal("fixture unexpectedly changed the visible projection", err)
				}
			}
			err = r.finishSourceAudit(ctx, state, before)
			switch scenario {
			case "unchanged", "orphan":
				if err != nil {
					t.Fatal("unchanged complete observation rejected", err)
				}
			case "orphan added", "page and root changed":
				if !errors.Is(err, ErrConflict) {
					t.Fatal("changed raw catalog was accepted", err)
				}
			case "missing page", "missing member":
				if !errors.Is(err, storage.ErrIncompleteCatalog) {
					t.Fatal("incomplete batch was accepted", err)
				}
			case "corrupt page":
				if !errors.Is(err, storage.ErrRecord) {
					t.Fatal("changed page passed its root hash", err)
				}
			case "changed state", "target error":
				if !errors.Is(err, ErrSource) {
					t.Fatal("state or target change was accepted", err)
				}
			case "canceled":
				if !errors.Is(err, context.Canceled) {
					t.Fatal("canceled audit succeeded", err)
				}
			}
		})
	}
}
