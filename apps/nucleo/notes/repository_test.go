// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var testScope = storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}

const testEpoch = "12345678-1234-1234-1234-123456789abc"

type trustedNotesPort struct {
	identity                   storage.IdentityVersion
	binding                    storage.Binding
	records                    map[storage.Reference]storage.Record
	txns                       map[string]storage.Reference
	files                      map[storage.Reference][]byte
	fileRequests               map[storage.Reference]storage.FileRequest
	jobs                       map[string]storage.Reference
	writes, uploads, downloads int
	prepareErr                 error
	loseIndex                  bool
}

func newTrustedNotesPort() *trustedNotesPort {
	return &trustedNotesPort{identity: storage.IsolatedIdentity, binding: storage.Binding{Scope: testScope, Target: storage.PublicationTarget{RoomID: "!notes:synthetic.test", DeviceID: "SYNTHETIC"}, Rooms: []string{"!notes:synthetic.test"}}, records: map[storage.Reference]storage.Record{}, txns: map[string]storage.Reference{}, files: map[storage.Reference][]byte{}, fileRequests: map[storage.Reference]storage.FileRequest{}, jobs: map[string]storage.Reference{}}
}

func (p *trustedNotesPort) StorageIdentityVersion() storage.IdentityVersion { return p.identity }

func (p *trustedNotesPort) StorageBindings(context.Context) (storage.Binding, error) {
	return p.binding, nil
}

func (p *trustedNotesPort) PrepareStorage(_ context.Context, scope storage.Scope) (storage.Binding, error) {
	if p.prepareErr != nil {
		return storage.Binding{}, p.prepareErr
	}
	if scope != p.binding.Scope {
		return storage.Binding{}, ErrSource
	}
	return p.binding, nil
}

func (p *trustedNotesPort) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if target != p.binding.Target {
		return ErrSource
	}
	return nil
}

func (p *trustedNotesPort) StorageObjectHistory(_ context.Context, room, cursor string, _ int) (storage.ObjectPage, error) {
	if cursor != "" {
		return storage.ObjectPage{}, ErrSource
	}
	var out []storage.RecordEntry
	for ref, record := range p.records {
		if ref.RoomID == room && record.Role == "index" {
			out = append(out, storage.RecordEntry{Reference: ref, Record: record})
		}
	}
	return storage.ObjectPage{Entries: out}, nil
}

func (p *trustedNotesPort) ReadStorageRecord(_ context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	record, ok := p.records[ref]
	if !ok || record.ObjectID != object || record.Role != role {
		return storage.Record{}, ErrNotFound
	}
	record.Payload = slices.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	return record, nil
}

func (p *trustedNotesPort) PutStorageRecordAt(_ context.Context, target storage.PublicationTarget, txn string, record storage.Record) (storage.Reference, error) {
	if err := p.CheckStoragePublicationTarget(target); err != nil {
		return storage.Reference{}, err
	}
	if record.Validate(testScope) != nil {
		return storage.Reference{}, ErrSource
	}
	if ref, ok := p.txns[txn]; ok {
		return ref, nil
	}
	ref := storage.Reference{RoomID: target.RoomID, EventID: "$event" + strconv.Itoa(len(p.records)+len(p.files)+1)}
	record.Payload = slices.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	p.records[ref] = record
	p.txns[txn] = ref
	p.writes++
	return ref, nil
}

func (p *trustedNotesPort) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	payload, err := json.Marshal(commit)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	previous := make([]string, len(commit.Parents))
	for i, parent := range commit.Parents {
		previous[i] = parent.EventID
	}
	ref, err := p.PutStorageRecordAt(ctx, target, txn, storage.Record{Version: 1, Scope: testScope, ObjectID: object, RevisionID: revision, Role: "index", Payload: payload, Previous: previous})
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	if p.loseIndex {
		p.loseIndex = false
		return storage.ObjectVersion{}, errors.New("synthetic lost accepted index ACK")
	}
	return p.StorageObject(ctx, ref, object)
}

func (p *trustedNotesPort) StorageObject(ctx context.Context, ref storage.Reference, object string) (storage.ObjectVersion, error) {
	record, err := p.ReadStorageRecord(ctx, ref, object, "index")
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: record}, testScope)
}

func (p *trustedNotesPort) UploadStorageFileAt(_ context.Context, _ *api.Drive, _ *offline.Store, job string, target storage.PublicationTarget, request storage.FileRequest, source io.ReaderAt) (storage.Reference, error) {
	if err := p.CheckStoragePublicationTarget(target); err != nil {
		return storage.Reference{}, err
	}
	if ref, ok := p.jobs[job]; ok {
		return ref, nil
	}
	data, err := io.ReadAll(io.NewSectionReader(source, 0, request.SizeBytes))
	if err != nil {
		return storage.Reference{}, err
	}
	ref := storage.Reference{RoomID: target.RoomID, EventID: "$file" + strconv.Itoa(len(p.records)+len(p.files)+1)}
	p.files[ref] = data
	p.fileRequests[ref] = request
	p.jobs[job] = ref
	p.uploads++
	return ref, nil
}

func (p *trustedNotesPort) StorageFileInfo(_ context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, error) {
	request, ok := p.fileRequests[ref]
	if !ok || request.ObjectID != object || request.RevisionID != revision {
		return storage.FileRequest{}, ErrNotFound
	}
	return request, nil
}

func (p *trustedNotesPort) DownloadStorageFile(ctx context.Context, _ *api.Drive, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	request, err := p.StorageFileInfo(ctx, ref, object, revision)
	if err != nil {
		return request, err
	}
	p.downloads++
	_, err = out.Write(p.files[ref])
	return request, err
}

func (p *trustedNotesPort) AcknowledgeStorageFile(ctx context.Context, _ *offline.Store, job string, index storage.Reference) error {
	ref, ok := p.jobs[job]
	if !ok {
		return storage.ErrUploadMissing
	}
	record := p.records[index]
	version, err := storage.DecodeObject(storage.RecordEntry{Reference: index, Record: record}, testScope)
	if err != nil {
		return err
	}
	if version.Commit.Content == nil || version.Commit.Content.Reference != ref {
		return ErrSource
	}
	delete(p.jobs, job)
	return ctx.Err()
}

type retainedNotesSource struct {
	state                            SnapshotState
	rows                             map[string][]byte
	stateCalls, begins, pages, opens int
}

func (s *retainedNotesSource) State(context.Context, storage.Scope) (SnapshotState, error) {
	s.stateCalls++
	return s.state, nil
}

func (s *retainedNotesSource) Begin(_ context.Context, scope storage.Scope, epoch string) (SnapshotState, error) {
	s.begins++
	now := time.Now().UTC()
	s.state = SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: int64(len(s.rows))}}
	return s.state, nil
}

func (s *retainedNotesSource) Page(_ context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (SnapshotPage, error) {
	s.pages++
	keys := make([]string, 0, len(s.rows))
	for key := range s.rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := SnapshotPage{Scope: scope, Epoch: epoch}
	for _, key := range keys {
		parts := strings.SplitN(key, "/", 2)
		if parts[0] < after.Kind || parts[0] == after.Kind && parts[1] <= after.ID {
			continue
		}
		body := s.rows[key]
		out.Rows = append(out.Rows, SnapshotRow{Kind: parts[0], ID: parts[1], Size: int64(len(body)), SHA256: hashBytes(body)})
	}
	return out, nil
}

func (s *retainedNotesSource) Open(_ context.Context, _ storage.Scope, _ string, row SnapshotRow) (io.ReadCloser, error) {
	s.opens++
	body, ok := s.rows[row.Kind+"/"+row.ID]
	if !ok {
		return nil, ErrSource
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

func notesFixture(t *testing.T) (*Repository, *trustedNotesPort, *retainedNotesSource) {
	t.Helper()
	port := newTrustedNotesPort()
	source := &retainedNotesSource{state: SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: testScope, Mode: "legacy"}}, rows: map[string][]byte{
		"notebook/book": []byte(`{"id":"book","owner_sub":"alice","title":"private notebook","indexable":true,"color":"#123456","layout":"list","visibility":"private","updated_at":"2026-10-29T10:00:00.123456789Z","future":{"integer":900719925474099312345}}`),
		"note/note":     []byte(`{"id":"note","owner_sub":"alice","notebook_id":"book","kind":"text","width":260,"title":"private note","content":"<p>reunión secreta</p>","data":{"integer":900719925474099312345,"scientific":1.234567890123456789e+99},"created_at":"2026-10-29T10:00:00.123456789Z","updated_at":"2026-10-29T10:00:00.123456789Z","future":{"nested":9007199254740993}}`),
	}}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(Config{Context: t.Context(), Scope: testScope, Crypto: port, Gateway: api.NewDrive("https://unused.invalid", nil, nil), Journal: journal, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, port, source
}

func TestPrivateNotesRequiresCompleteImportAndPreservesExactDataAcrossMutations(t *testing.T) {
	r, port, source := notesFixture(t)
	if err := r.Initialize(t.Context()); !errors.Is(err, ErrNotReady) || source.begins != 0 || source.pages != 0 || source.opens != 0 || port.writes != 0 {
		t.Fatal("ordinary open imported legacy data", err)
	}
	if _, err := r.ListNotebooks(t.Context()); !errors.Is(err, ErrNotReady) {
		t.Fatal("uninitialized repository exposed data", err)
	}
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal("explicit snapshot failed", err)
	}
	if source.begins != 1 || source.pages != 1 || source.opens != 2 {
		t.Fatal("source import was repeated", source.begins, source.pages, source.opens)
	}
	original := slices.Clone(source.rows["note/note"])
	raw, err := r.ReadRaw(t.Context(), "note", "note")
	if err != nil || !bytes.Equal(raw, original) {
		t.Fatal("migration changed exact source JSON", err)
	}
	clear(raw)
	books, err := r.ListNotebooks(t.Context())
	if err != nil || len(books) != 1 || books[0].Title != "private notebook" {
		t.Fatal("private notebooks failed", err)
	}
	notes, err := r.ListNotes(t.Context(), "book")
	if err != nil || len(notes) != 1 {
		t.Fatal("private note list failed", err)
	}
	input := notes[0].Input()
	input.Title = "edited privately"
	port.loseIndex = true
	if _, err := r.UpdateNote(t.Context(), "note", "book", input); err == nil {
		t.Fatal("lost accepted index ACK was ignored")
	}
	updated, err := r.UpdateNote(t.Context(), "note", "book", input)
	if err != nil || updated.Title != input.Title {
		t.Fatal("accepted write did not resume", err)
	}
	raw, err = r.ReadRaw(t.Context(), "note", "note")
	if err != nil || !bytes.Contains(raw, []byte(`900719925474099312345`)) || !bytes.Contains(raw, []byte(`1.234567890123456789e+99`)) || !bytes.Contains(raw, []byte(`"future":{"nested":9007199254740993}`)) {
		t.Fatal("mutation rounded or dropped unknown source fields", err)
	}
	clear(raw)
	before := port.writes
	if _, err := r.UpdateNote(t.Context(), "note", "book", input); !errors.Is(err, ErrConflict) || port.writes != before {
		t.Fatal("stale input overwrote current data", err)
	}
	if err := r.DeleteNote(t.Context(), "note"); err != nil {
		t.Fatal(err)
	}
	if trash, err := r.ListTrash(t.Context(), 100); err != nil || len(trash) != 1 {
		t.Fatal("private trash missing", err)
	}
	if err := r.RestoreNote(t.Context(), "note"); err != nil {
		t.Fatal(err)
	}
	if hits, err := r.Search(t.Context(), "reunion", 100, false); err != nil || len(hits) != 1 {
		t.Fatal("device-side accent insensitive search failed", err)
	}
	if !bytes.Equal(original, source.rows["note/note"]) || source.opens != 2 || source.pages != 1 {
		t.Fatal("private operation modified or reread legacy data")
	}
	other, err := New(r.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.Initialize(t.Context()); err != nil {
		t.Fatal("completed import did not reopen after private edits", err)
	}
	if source.opens != 2 || source.pages != 1 {
		t.Fatal("new device reimported the retained source")
	}
}

func TestPrivateNotesRejectsPartialReceiptAndMissingBaselineWithoutReimport(t *testing.T) {
	for _, fault := range []string{"premature-receipt", "missing-baseline", "changed-hash"} {
		t.Run(fault, func(t *testing.T) {
			r, port, source := notesFixture(t)
			if fault == "premature-receipt" {
				now := time.Now().UTC()
				source.state = SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: testScope, Mode: "frozen", Epoch: testEpoch, StartedAt: &now, Rows: 2}}
				if err := r.publishReceipt(t.Context(), snapshotWork{Epoch: testEpoch, Target: port.binding.Target}, snapshotReceipt{Kind: receiptKind, Epoch: testEpoch, Rows: 2, Inventory: hashBytes(nil)}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := r.ImportLegacySnapshot(t.Context()); err != nil {
					t.Fatal(err)
				}
				for ref, record := range port.records {
					if record.Role != "index" || record.ObjectID != object("notebook", "book") {
						continue
					}
					if fault == "missing-baseline" {
						delete(port.records, ref)
					} else {
						var commit storage.ObjectCommit
						_ = json.Unmarshal(record.Payload, &commit)
						var metadata entity
						_ = json.Unmarshal(commit.Metadata, &metadata)
						metadata.SHA256 = hashBytes([]byte("different bytes"))
						metadata.Baseline = metadata.SHA256
						commit.Metadata, _ = json.Marshal(metadata)
						record.Payload, _ = json.Marshal(commit)
						port.records[ref] = record
					}
				}
			}
			other, err := New(r.cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			opens, pages := source.opens, source.pages
			if err := other.Initialize(t.Context()); err == nil {
				t.Fatal("partial or substituted baseline activated")
			}
			if _, err := other.ListNotebooks(t.Context()); !errors.Is(err, ErrNotReady) {
				t.Fatal("partial data became readable", err)
			}
			if source.opens != opens || source.pages != pages {
				t.Fatal("partial open reimported the retained source")
			}
		})
	}
}

func TestPrivateNotesRejectsSharedOrUnverifiedSourceBeforeFreeze(t *testing.T) {
	for _, fault := range []string{"shared", "verification", "scope"} {
		t.Run(fault, func(t *testing.T) {
			r, port, source := notesFixture(t)
			switch fault {
			case "shared":
				source.state.Shared = true
			case "verification":
				port.prepareErr = errors.New("synthetic device unverified")
			case "scope":
				source.state.Scope.Product = "mail"
			}
			if err := r.ImportLegacySnapshot(t.Context()); err == nil {
				t.Fatal("invalid source imported")
			}
			if source.begins != 0 || source.pages != 0 || source.opens != 0 || port.writes != 0 {
				t.Fatal("source changed before custody and scope were established")
			}
		})
	}
}

func TestPrivateNotesLargeExactRowsUsePrivateFilesAndReopen(t *testing.T) {
	r, port, source := notesFixture(t)
	raw := append([]byte(`{"id":"note","owner_sub":"alice","notebook_id":"book","kind":"text","title":"large","content":"`), []byte(strings.Repeat("private ", 8192))...)
	raw = append(raw, []byte(`","future":{"n":900719925474099312345}}`)...)
	source.rows["note/note"] = raw
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	if port.uploads != 1 || port.downloads == 0 || len(port.jobs) != 0 {
		t.Fatal("large source did not use acknowledged private file path", port.uploads, port.downloads, len(port.jobs))
	}
	got, err := r.ReadRaw(t.Context(), "note", "note")
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("large JSON changed", err)
	}
	clear(got)
	calls := source.opens
	reopened, err := New(r.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Initialize(t.Context()); err != nil {
		t.Fatal("large baseline reopen", err)
	}
	if source.opens != calls {
		t.Fatal("reopen read source again")
	}
	for ref, request := range port.fileRequests {
		request.SizeBytes = MaxEntityJSON + 1
		port.fileRequests[ref] = request
	}
	downloads := port.downloads
	if _, err := r.ReadRaw(t.Context(), "note", "note"); !errors.Is(err, ErrSource) || port.downloads != downloads {
		t.Fatal("oversize manifest fetched body", err)
	}
}

func TestPrivateNotesRejectsBrokenSourceGraphBeforeCompletion(t *testing.T) {
	cases := map[string]string{
		"missing identity": `{"owner_sub":"alice","notebook_id":"book"}`,
		"wrong identity":   `{"id":"other","owner_sub":"alice","notebook_id":"book"}`,
		"malformed trash":  `{"id":"note","owner_sub":"alice","notebook_id":"book","deleted_at":"not a date"}`,
		"missing notebook": `{"id":"note","owner_sub":"alice","notebook_id":"missing"}`,
		"missing group":    `{"id":"note","owner_sub":"alice","notebook_id":"book","group_id":"missing"}`,
		"cyclic group":     `{"id":"note","owner_sub":"alice","notebook_id":"book","group_id":"note"}`,
		"external room":    `{"id":"note","owner_sub":"alice","notebook_id":"book","room_id":"shared"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			r, port, source := notesFixture(t)
			source.rows["note/note"] = []byte(body)
			if err := r.ImportLegacySnapshot(t.Context()); err == nil {
				t.Fatal("invalid retained source completed")
			}
			for _, record := range port.records {
				var header struct {
					Kind string `json:"kind"`
				}
				if record.Role == "index" {
					var commit storage.ObjectCommit
					if json.Unmarshal(record.Payload, &commit) == nil {
						_ = json.Unmarshal(commit.Metadata, &header)
					}
				}
				if header.Kind == receiptKind {
					t.Fatal("completion published before source graph validated")
				}
			}
			if _, err := r.ListNotebooks(t.Context()); !errors.Is(err, ErrNotReady) {
				t.Fatal("failed source activated repository", err)
			}
		})
	}
}

func TestPrivateNotesRejectsInvalidGroupingBeforePublication(t *testing.T) {
	r, port, _ := notesFixture(t)
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	list, err := r.ListNotes(t.Context(), "book")
	if err != nil {
		t.Fatal(err)
	}
	input := list[0].Input()
	self := "note"
	input.GroupID = &self
	writes := port.writes
	if _, err := r.UpdateNote(t.Context(), "note", "book", input); !errors.Is(err, ErrSource) || port.writes != writes {
		t.Fatal("invalid group published private content", err)
	}
	missing := "missing"
	input.GroupID = &missing
	if _, err := r.UpdateNote(t.Context(), "note", "book", input); !errors.Is(err, ErrSource) || port.writes != writes {
		t.Fatal("missing group published private content", err)
	}
}

func hashBytes(data []byte) string  { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func object(kind, id string) string { return "notes_" + hashBytes([]byte(kind+"\x00"+id)) }

type (
	entity struct {
		Kind     string `json:"kind"`
		Entity   string `json:"entity"`
		ID       string `json:"id"`
		SHA256   string `json:"sha256"`
		Epoch    string `json:"epoch"`
		Baseline string `json:"baseline"`
		Seed     bool   `json:"seed"`
	}
	snapshotWork struct {
		Epoch  string
		Target storage.PublicationTarget
	}
	snapshotReceipt struct {
		Kind      string `json:"kind"`
		Epoch     string `json:"epoch"`
		Rows      int64  `json:"rows"`
		Inventory string `json:"inventory"`
	}
)

// Publish an adversarial receipt directly at the trusted boundary. The ordinary
// application never exposes this path, so the test can model a partial catalog.
func (r *Repository) publishReceipt(ctx context.Context, work snapshotWork, receipt snapshotReceipt) error {
	payload, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	obj := "notes_snapshot_" + hashBytes([]byte(work.Epoch))
	revision := work.Epoch
	ref, err := r.cfg.Crypto.PutStorageRecordAt(ctx, work.Target, "snapshot_"+revision+"_content", storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: obj, RevisionID: revision, Role: "content", Payload: payload})
	if err != nil {
		return err
	}
	_, err = r.cfg.Crypto.PutStorageObjectAt(ctx, work.Target, "snapshot_"+revision+"_index", obj, revision+"_index", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: payload, Content: &storage.ContentPointer{Kind: "record", RevisionID: revision, Reference: ref}})
	return err
}

func TestPrivateNotesRejectsLegacyDestinationBeforeAnyIO(t *testing.T) {
	for _, version := range []storage.IdentityVersion{storage.LegacyIdentity, 0, 3} {
		t.Run(strconv.Itoa(int(version)), func(t *testing.T) {
			r, port, source := notesFixture(t)
			port.identity = version
			if _, err := New(r.cfg); !errors.Is(err, ErrSource) {
				t.Fatal("non-isolated destination accepted", err)
			}
			if source.stateCalls != 0 || source.begins != 0 || source.pages != 0 || source.opens != 0 || port.writes != 0 || port.uploads != 0 {
				t.Fatal("legacy destination touched source or provider before rejection")
			}
		})
	}
}

func TestPrivateNotesRejectsBinarySourceAndUntranslatedAttachmentsBeforeCompletion(t *testing.T) {
	t.Run("binary source", func(t *testing.T) {
		r, port, source := notesFixture(t)
		now := time.Now().UTC()
		source.state = SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: testScope, Mode: "frozen", Epoch: testEpoch, StartedAt: &now, Rows: 2, Blobs: 1}}
		if err := r.ImportLegacySnapshot(t.Context()); !errors.Is(err, ErrSource) {
			t.Fatal("binary source without binary port was accepted", err)
		}
		if source.begins != 0 || source.pages != 0 || source.opens != 0 || port.writes != 0 || port.uploads != 0 {
			t.Fatal("binary source was partially imported")
		}
	})
	for _, kind := range []string{"image", "file", "drive", "doc"} {
		t.Run(kind, func(t *testing.T) {
			r, port, source := notesFixture(t)
			body := []byte(`{"id":"note","owner_sub":"alice","notebook_id":"book","kind":"` + kind + `","data":{"url":"https://drive.example.test/legacy-plaintext"}}`)
			source.rows["note/note"] = body
			if err := r.ImportLegacySnapshot(t.Context()); !errors.Is(err, ErrAttachmentPending) {
				t.Fatal("URL-only attachment imported as private content", err)
			}
			for _, record := range port.records {
				if record.Role == "index" && strings.HasPrefix(record.ObjectID, "notes_snapshot_") {
					t.Fatal("untranslated attachment completed migration")
				}
			}
			if _, err := r.ListNotes(t.Context(), "book"); !errors.Is(err, ErrNotReady) {
				t.Fatal("attachment activated private repository", err)
			}
		})
	}
}

func TestPrivateNotesFailedReinitializationClosesPreviouslyReadyView(t *testing.T) {
	r, port, _ := notesFixture(t)
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	for ref, record := range port.records {
		if record.Role == "index" && record.ObjectID == object("note", "note") {
			delete(port.records, ref)
		}
	}
	if err := r.Initialize(t.Context()); err == nil {
		t.Fatal("incomplete catalog reopened")
	}
	if _, err := r.ListNotebooks(t.Context()); !errors.Is(err, ErrNotReady) {
		t.Fatal("failed refresh retained a ready projection", err)
	}
	if _, err := r.ReadRaw(t.Context(), "note", "note"); !errors.Is(err, ErrNotReady) {
		t.Fatal("failed refresh exposed body", err)
	}
}

func TestPrivateNotesMutationsApplySharedNormalizationOnTheDevice(t *testing.T) {
	r, port, _ := notesFixture(t)
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	book, err := r.CreateNotebook(t.Context(), api.NotebookInput{Title: "  another  ", Visibility: "personal"})
	if err != nil || book.Title != "another" || book.Layout != "tree" || book.Color != "#86198f" || book.Visibility != "personal" {
		t.Fatal("notebook device normalization differs", book, err)
	}
	note, err := r.CreateNote(t.Context(), book.ID, api.NoteInput{Title: "  safe title  ", Content: `<p onclick="steal()">safe<script>steal()</script></p>`, Data: json.RawMessage(`{"future":{"n":900719925474099312345}}`)})
	if err != nil || note.Title != "safe title" || note.Kind != "text" || note.Width != 260 || strings.Contains(note.Content, "steal") {
		t.Fatal("note device normalization differs", note, err)
	}
	if !bytes.Contains(note.Data, []byte(`900719925474099312345`)) {
		t.Fatal("normalized input rounded JSON")
	}
	input := note.Input()
	input.Title = "  edited  "
	updated, err := r.UpdateNote(t.Context(), note.ID, book.ID, input)
	if err != nil || updated.Title != "edited" {
		t.Fatal("device update failed", err)
	}
	writes := port.writes
	if _, err := r.CreateNote(t.Context(), book.ID, api.NoteInput{Kind: "unknown"}); err == nil || port.writes != writes {
		t.Fatal("invalid kind reached private publisher", err)
	}
	badWidth := 1
	if _, err := r.CreateNote(t.Context(), book.ID, api.NoteInput{Width: &badWidth}); err == nil || port.writes != writes {
		t.Fatal("invalid width reached private publisher", err)
	}
	if _, err := r.CreateNotebook(t.Context(), api.NotebookInput{Title: "bad", Color: "not-a-color"}); err == nil || port.writes != writes {
		t.Fatal("invalid notebook reached private publisher", err)
	}
}

func TestPrivateNotesNormalizationPreservesCASAndUnknownCryptoParameters(t *testing.T) {
	now := time.Date(2026, 10, 30, 1, 2, 3, 987654321, time.UTC)
	crypto := json.RawMessage(`{"v":1,"kdf":"PBKDF2-SHA256","salt":"abcdefghijklmnop12345678","iterations":600000,"check":"encv1:fixture","future":{"integer":900719925474099312345}}`)
	input := api.NotebookInput{Title: " private ", Encrypted: true, Crypto: crypto, ExpectedUpdatedAt: &now}
	normalized, err := normalizeNotebookInput(input)
	if err != nil || normalized.ExpectedUpdatedAt == nil || !normalized.ExpectedUpdatedAt.Equal(now) || !bytes.Equal(normalized.Crypto, crypto) {
		t.Fatal("normalization altered CAS or crypto extension", err)
	}
	r, _, _ := notesFixture(t)
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	book, err := r.CreateNotebook(t.Context(), api.NotebookInput{Title: "timestamp"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.ReadRaw(t.Context(), "notebook", book.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	var value struct {
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if json.Unmarshal(raw, &value) != nil || value.CreatedAt.IsZero() || !value.CreatedAt.Equal(value.UpdatedAt) {
		t.Fatal("new private notebook lost source timestamps")
	}
}

func TestPrivateNotesExplicitImportHandoffReusesExactSeedWithEmptyJournal(t *testing.T) {
	first, port, source := notesFixture(t)
	body := []byte(`{"id":"note","owner_sub":"alice","notebook_id":"book","kind":"text","width":260,"title":"large seed","padding":"` + strings.Repeat("private source ", 2048) + `"}`)
	source.rows["note/note"] = body
	port.loseIndex = true
	if err := first.ImportLegacySnapshot(t.Context()); err == nil {
		t.Fatal("synthetic accepted partial index did not interrupt import")
	}
	if source.opens != 1 || source.begins != 1 || port.uploads != 1 {
		t.Fatal("partial seed fixture did not reach its lost ACK", source.opens, source.begins, port.uploads)
	}
	first.Close()
	nextJournal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := first.cfg
	cfg.Journal = nextJournal
	port.binding.Target.DeviceID = "NEW_DEVICE"
	next, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err := next.Initialize(t.Context()); !errors.Is(err, ErrNotReady) {
		t.Fatal("partial catalog activated without explicit resume", err)
	}
	if err := next.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal("fresh-journal handoff failed", err)
	}
	if source.opens != 2 || source.begins != 1 || port.uploads != 1 {
		t.Fatal("handoff reimported an accepted body", source.opens, source.begins, port.uploads)
	}
	raw, err := next.ReadRaw(t.Context(), "note", "note")
	if err != nil || !bytes.Equal(raw, body) {
		t.Fatal("resumed body changed", err)
	}
	clear(raw)
	count := 0
	for _, record := range port.records {
		if record.Role == "index" && record.ObjectID == object("note", "note") {
			count++
		}
	}
	if count != 1 {
		t.Fatal("handoff duplicated seed history", count)
	}
}

func TestPrivateNotesImportHandoffPreservesConflictsAndRejectsChangedSource(t *testing.T) {
	for _, fault := range []string{"duplicate", "edit", "source-changed", "not-seed", "wrong-epoch", "wrong-state"} {
		t.Run(fault, func(t *testing.T) {
			first, port, source := notesFixture(t)
			port.loseIndex = true
			if err := first.ImportLegacySnapshot(t.Context()); err == nil {
				t.Fatal("partial fixture did not fail")
			}
			switch fault {
			case "duplicate":
				for ref, record := range port.records {
					if record.Role == "index" {
						record.RevisionID = "duplicate-seed"
						record.Payload = slices.Clone(record.Payload)
						port.records[storage.Reference{RoomID: ref.RoomID, EventID: "$duplicate"}] = record
						break
					}
				}
			case "edit":
				var parent storage.Reference
				var old storage.Record
				for ref, record := range port.records {
					if record.Role == "index" {
						parent = ref
						old = record
						break
					}
				}
				var commit storage.ObjectCommit
				if json.Unmarshal(old.Payload, &commit) != nil {
					t.Fatal("index fixture")
				}
				var summary entity
				_ = json.Unmarshal(commit.Metadata, &summary)
				summary.Seed = false
				commit.Metadata, _ = json.Marshal(summary)
				commit.Parents = []storage.Reference{parent}
				if _, err := port.PutStorageObjectAt(t.Context(), port.binding.Target, "synthetic-edit", old.ObjectID, "synthetic-edited-index", commit); err != nil {
					t.Fatal(err)
				}
			case "source-changed":
				source.rows["note/note"] = bytes.Replace(source.rows["note/note"], []byte("private note"), []byte("different frozen source"), 1)
			case "not-seed", "wrong-epoch", "wrong-state":
				for ref, record := range port.records {
					if record.Role != "index" {
						continue
					}
					var commit storage.ObjectCommit
					_ = json.Unmarshal(record.Payload, &commit)
					var summary entity
					_ = json.Unmarshal(commit.Metadata, &summary)
					if fault == "not-seed" {
						summary.Seed = false
					}
					if fault == "wrong-epoch" {
						summary.Epoch = testEpoch
					}
					if fault == "wrong-state" {
						commit.State = "trash"
					}
					commit.Metadata, _ = json.Marshal(summary)
					record.Payload, _ = json.Marshal(commit)
					port.records[ref] = record
					break
				}
			}
			first.Close()
			journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
			if err != nil {
				t.Fatal(err)
			}
			cfg := first.cfg
			cfg.Journal = journal
			port.binding.Target.DeviceID = "NEW_DEVICE"
			next, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Close()
			writes, opens := port.writes, source.opens
			records := len(port.records)
			if err := next.ImportLegacySnapshot(t.Context()); err == nil {
				t.Fatal("incompatible handoff was accepted")
			}
			if port.writes != writes || source.opens != opens || len(port.records) != records {
				t.Fatal("handoff changed conflicting history or reread plaintext")
			}
			if _, err := next.ListNotebooks(t.Context()); !errors.Is(err, ErrNotReady) {
				t.Fatal("conflict activated repository", err)
			}
		})
	}
}
