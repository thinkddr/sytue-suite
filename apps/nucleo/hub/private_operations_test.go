// SPDX-License-Identifier: Apache-2.0
package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// This is an in-memory port, not an SDK, Matrix server or encryption test.
// The real records engine and encrypted local journal run against it.
type memoryStorage struct {
	records.Crypto
	mu        sync.Mutex
	binding   storage.Binding
	records   map[storage.Reference]storage.Record
	objects   map[storage.Reference]storage.ObjectVersion
	tx        map[string]storage.Reference
	next      int
	lost      bool
	pre, post *testBarrier
}

var errLost = errors.New("synthetic lost response")

type testBarrier struct {
	mu      sync.Mutex
	arrived int
	ready   chan struct{}
}

func newTestBarrier() *testBarrier { return &testBarrier{ready: make(chan struct{})} }
func (b *testBarrier) wait(ctx context.Context) error {
	b.mu.Lock()
	b.arrived++
	if b.arrived == 2 {
		close(b.ready)
	}
	b.mu.Unlock()
	select {
	case <-b.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *memoryStorage) StorageIdentityVersion() storage.IdentityVersion {
	return storage.IsolatedIdentity
}

func (m *memoryStorage) PrepareStorage(_ context.Context, scope storage.Scope) (storage.Binding, error) {
	if scope != m.binding.Scope {
		return storage.Binding{}, records.ErrSource
	}
	return m.binding, nil
}

func (m *memoryStorage) StorageBindings(context.Context) (storage.Binding, error) {
	return m.binding, nil
}

func (m *memoryStorage) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if target != m.binding.Target {
		return records.ErrSource
	}
	return nil
}

func (m *memoryStorage) reference() storage.Reference {
	m.next++
	return storage.Reference{RoomID: m.binding.Target.RoomID, EventID: fmt.Sprintf("$event%d", m.next)}
}

func cloneRecord(r storage.Record) storage.Record {
	r.Payload = bytes.Clone(r.Payload)
	r.Previous = append([]string(nil), r.Previous...)
	return r
}

func cloneVersion(v storage.ObjectVersion) storage.ObjectVersion {
	raw, _ := json.Marshal(v)
	var out storage.ObjectVersion
	_ = json.Unmarshal(raw, &out)
	return out
}

func (m *memoryStorage) PutStorageRecordAt(_ context.Context, target storage.PublicationTarget, txn string, record storage.Record) (storage.Reference, error) {
	if err := m.CheckStoragePublicationTarget(target); err != nil {
		return storage.Reference{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ref, ok := m.tx[txn]; ok {
		if !record.MatchesExact(m.records[ref]) {
			return storage.Reference{}, records.ErrConflict
		}
		return ref, nil
	}
	ref := m.reference()
	m.tx[txn] = ref
	m.records[ref] = cloneRecord(record)
	return ref, nil
}

func (m *memoryStorage) ReadStorageRecord(_ context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.records[ref]
	if !ok || record.ObjectID != object || record.Role != role {
		return storage.Record{}, records.ErrSource
	}
	return cloneRecord(record), nil
}

func (m *memoryStorage) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	if err := m.CheckStoragePublicationTarget(target); err != nil {
		return storage.ObjectVersion{}, err
	}
	m.mu.Lock()
	pre, post := m.pre, m.post
	m.mu.Unlock()
	if pre != nil {
		if err := pre.wait(ctx); err != nil {
			return storage.ObjectVersion{}, err
		}
	}
	m.mu.Lock()
	ref, ok := m.tx[txn]
	if ok {
		v, exists := m.objects[ref]
		if !exists || v.ObjectID != object || v.RevisionID != revision || !commit.MatchesExact(v.Commit) {
			m.mu.Unlock()
			return storage.ObjectVersion{}, records.ErrConflict
		}
	}
	if !ok {
		ref = m.reference()
		m.tx[txn] = ref
		version := storage.ObjectVersion{Reference: ref, ObjectID: object, RevisionID: revision, Commit: commit}
		m.objects[ref] = cloneVersion(version)
	}
	version := cloneVersion(m.objects[ref])
	lost := m.lost
	m.lost = false
	m.mu.Unlock()
	if post != nil {
		if err := post.wait(ctx); err != nil {
			return storage.ObjectVersion{}, err
		}
	}
	if lost {
		return storage.ObjectVersion{}, errLost
	}
	return version, nil
}

func (m *memoryStorage) StorageObject(_ context.Context, ref storage.Reference, object string) (storage.ObjectVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.objects[ref]
	if !ok || v.ObjectID != object {
		return storage.ObjectVersion{}, records.ErrSource
	}
	return cloneVersion(v), nil
}

func (m *memoryStorage) StorageObjectHistory(_ context.Context, room, cursor string, _ int) (storage.ObjectPage, error) {
	if room != m.binding.Target.RoomID || cursor != "" {
		return storage.ObjectPage{}, records.ErrSource
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	page := storage.ObjectPage{}
	for _, v := range m.objects {
		payload, err := json.Marshal(v.Commit)
		if err != nil {
			return storage.ObjectPage{}, err
		}
		previous := make([]string, 0, len(v.Commit.Parents))
		for _, parent := range v.Commit.Parents {
			previous = append(previous, parent.EventID)
		}
		page.Entries = append(page.Entries, storage.RecordEntry{Reference: v.Reference, Record: storage.Record{Version: 1, Scope: m.binding.Scope, ObjectID: v.ObjectID, RevisionID: v.RevisionID, Role: "index", Previous: previous, Payload: payload}})
	}
	return page, nil
}

type emptySource struct {
	records.Source
	state records.SnapshotState
}

func (s *emptySource) State(_ context.Context, scope storage.Scope) (records.SnapshotState, error) {
	if scope != s.state.Scope {
		return records.SnapshotState{}, records.ErrSource
	}
	return s.state, nil
}

func (s *emptySource) Begin(_ context.Context, scope storage.Scope, epoch string) (records.SnapshotState, error) {
	if scope != s.state.Scope {
		return records.SnapshotState{}, records.ErrSource
	}
	if s.state.Mode == "legacy" {
		now := time.Now().UTC()
		s.state.Mode, s.state.Epoch, s.state.StartedAt = "frozen", epoch, &now
	}
	if s.state.Epoch != epoch {
		return records.SnapshotState{}, records.ErrSource
	}
	return s.state, nil
}

func (s *emptySource) Page(_ context.Context, scope storage.Scope, epoch string, _ storage.LegacySourceCursor) (records.SnapshotPage, error) {
	return records.SnapshotPage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{}}, nil
}

func setupPrivate(t *testing.T) (*Repository, *memoryStorage, *emptySource) {
	t.Helper()
	scope := noticeFixture().Scope
	binding := storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: "!hub:test", DeviceID: "DEVICE"}, Rooms: []string{"!hub:test"}}
	port := &memoryStorage{binding: binding, records: map[storage.Reference]storage.Record{}, objects: map[storage.Reference]storage.ObjectVersion{}, tx: map[string]storage.Reference{}}
	source := &emptySource{state: records.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}}
	r := newPrivateClient(t, port, source)
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal("import empty fixture:", err)
	}
	return r, port, source
}

func newPrivateClient(t *testing.T, port *memoryStorage, source *emptySource) *Repository {
	t.Helper()
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(Config{Context: t.Context(), Scope: port.binding.Scope, Crypto: port, Gateway: &api.Drive{}, Journal: journal, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}

func TestPrivateOperationsRecoverLostResponseWithoutResettingRead(t *testing.T) {
	r, port, source := setupPrivate(t)
	n := noticeFixture()
	port.mu.Lock()
	port.lost = true
	port.mu.Unlock()
	if err := r.Put(t.Context(), n); !errors.Is(err, errLost) {
		t.Fatal("lost response not reported", err)
	}
	pending, err := r.Pending(t.Context())
	if err != nil || pending.Count == 0 {
		t.Fatal("lost response discarded WAL", pending, err)
	}
	other := newPrivateClient(t, port, source)
	if err := other.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := other.MarkRead(t.Context(), n.ID); err != nil {
		t.Fatal(err)
	}
	port.mu.Lock()
	count := len(port.objects)
	port.mu.Unlock()
	if err := r.Put(t.Context(), n); err != nil {
		t.Fatal("retry", err)
	}
	got, err := r.List(t.Context())
	if err != nil || len(got) != 1 || !got[0].Read {
		t.Fatal("retry reset read or lost notice", got, err)
	}
	pending, err = r.Pending(t.Context())
	if err != nil || pending.Count != 0 {
		t.Fatal("successful retry retained WAL", pending, err)
	}
	if err := r.Put(t.Context(), n); err != nil {
		t.Fatal("identical retry", err)
	}
	if err := r.MarkRead(t.Context(), n.ID); err != nil {
		t.Fatal("read retry", err)
	}
	port.mu.Lock()
	if len(port.objects) != count {
		port.mu.Unlock()
		t.Fatal("idempotent retry published another revision")
	}
	port.mu.Unlock()
	n.Title = "different"
	if err := r.Put(t.Context(), n); !errors.Is(err, records.ErrConflict) {
		t.Fatal("different payload reused ID", err)
	}
}

func TestPrivateConcurrentCreatesKeepConflictingHeads(t *testing.T) {
	r, port, source := setupPrivate(t)
	other := newPrivateClient(t, port, source)
	if err := other.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	pre, post := newTestBarrier(), newTestBarrier()
	port.mu.Lock()
	port.pre, port.post = pre, post
	port.mu.Unlock()
	a, b := noticeFixture(), noticeFixture()
	b.Title = "other content"
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	result := make(chan error, 2)
	go func() { result <- r.Put(ctx, a) }()
	go func() { result <- other.Put(ctx, b) }()
	for range 2 {
		if err := <-result; !errors.Is(err, records.ErrConflict) {
			t.Fatal("concurrent head was not reported", err)
		}
	}
	port.mu.Lock()
	port.pre, port.post = nil, nil
	port.mu.Unlock()
	if _, err := r.List(t.Context()); !errors.Is(err, records.ErrConflict) {
		t.Fatal("conflict hidden by list", err)
	}
}

func TestPrivateConcurrentReadUpdatesDoNotChooseAHead(t *testing.T) {
	r, port, source := setupPrivate(t)
	n := noticeFixture()
	if err := r.Put(t.Context(), n); err != nil {
		t.Fatal(err)
	}
	other := newPrivateClient(t, port, source)
	if err := other.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	port.mu.Lock()
	port.pre, port.post = newTestBarrier(), newTestBarrier()
	port.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	result := make(chan error, 2)
	go func() { result <- r.MarkRead(ctx, n.ID) }()
	go func() { result <- other.MarkRead(ctx, n.ID) }()
	for range 2 {
		if err := <-result; !errors.Is(err, records.ErrConflict) {
			t.Fatal("read conflict was not reported", err)
		}
	}
	port.mu.Lock()
	port.pre, port.post = nil, nil
	port.mu.Unlock()
	if _, err := r.List(t.Context()); !errors.Is(err, records.ErrConflict) {
		t.Fatal("read conflict hidden", err)
	}
}

func TestPrivateCancelledAndClosedWorkDoesNotSucceed(t *testing.T) {
	r, _, _ := setupPrivate(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := r.Put(ctx, noticeFixture()); err == nil {
		t.Fatal("cancelled create succeeded")
	}
	if _, err := r.List(ctx); err == nil {
		t.Fatal("cancelled list succeeded")
	}
	r.Close()
	if err := r.Put(t.Context(), noticeFixture()); err == nil {
		t.Fatal("closed repository accepted create")
	}
}

// «Marcar como no leído» y «Marcar todas» de un producto también en el almacén privado
// de One: el aviso vuelve a sin leer sin perder nada, dos veces no publica otra
// revisión, y «Marcar todas» de Drive deja el correo como estaba.
func TestPrivateMarkUnreadAndMarkAllOfOneSource(t *testing.T) {
	r, port, _ := setupPrivate(t)
	correo := noticeFixture()
	drive := noticeFixture()
	drive.ID, drive.Source, drive.Type, drive.Title = "notice2", "drive", "drive.shared", "Shared"
	for _, n := range []Notice{correo, drive} {
		if err := r.Put(t.Context(), n); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.MarkAllReadDe(t.Context(), "drive"); err != nil {
		t.Fatal(err)
	}
	leidos := func() map[string]bool {
		t.Helper()
		got, err := r.List(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, n := range got {
			out[n.ID] = n.Read
		}
		return out
	}
	if l := leidos(); !l["notice2"] || l["notice1"] {
		t.Fatal("«Marcar todas» de Drive tocó lo que no era de Drive", l)
	}
	if err := r.MarkUnread(t.Context(), "notice2"); err != nil {
		t.Fatal(err)
	}
	if l := leidos(); l["notice2"] {
		t.Fatal("el aviso sigue leído", l)
	}
	port.mu.Lock()
	antes := len(port.objects)
	port.mu.Unlock()
	if err := r.MarkUnread(t.Context(), "notice2"); err != nil {
		t.Fatal("otra vez", err)
	}
	port.mu.Lock()
	despues := len(port.objects)
	port.mu.Unlock()
	if despues != antes {
		t.Fatal("desmarcar uno ya sin leer publicó otra revisión")
	}
	if err := r.MarkUnread(t.Context(), "../x"); !errors.Is(err, ErrNotice) {
		t.Fatal("identificador inválido", err)
	}
}
