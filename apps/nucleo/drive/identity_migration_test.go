// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type catalogTransferCrypto struct {
	Crypto
	binding      storage.Binding
	records      map[storage.Reference]storage.Record
	transactions map[string]storage.Reference
	prefix       string
	fence        string
	fenceError   error
	loseIndex    bool
	writes       int
	afterWrite   func()
}

func newCatalogTransferCrypto(scope storage.Scope, prefix string) *catalogTransferCrypto {
	room := "!" + prefix + ":matrix.test"
	return &catalogTransferCrypto{binding: storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: room, DeviceID: prefix + "-device"}, Rooms: []string{room}}, records: map[storage.Reference]storage.Record{}, transactions: map[string]storage.Reference{}, prefix: prefix, fence: "$frozen"}
}

func (c *catalogTransferCrypto) PrepareStorage(_ context.Context, scope storage.Scope) (storage.Binding, error) {
	if scope != c.binding.Scope {
		return storage.Binding{}, ErrSource
	}
	return c.binding, nil
}

func (c *catalogTransferCrypto) StorageBindings(context.Context) (storage.Binding, error) {
	return c.binding, nil
}

func (c *catalogTransferCrypto) FrozenStorageBindings(context.Context) (storage.FrozenBinding, error) {
	if c.fenceError != nil {
		return storage.FrozenBinding{}, c.fenceError
	}
	return storage.FrozenBinding{Binding: c.binding, Fences: []storage.Reference{{RoomID: c.binding.Target.RoomID, EventID: c.fence}}}, nil
}

func (c *catalogTransferCrypto) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if target != c.binding.Target {
		return ErrSource
	}
	return nil
}

func (c *catalogTransferCrypto) StorageObjectHistory(_ context.Context, room, cursor string, _ int) (storage.ObjectPage, error) {
	if room != c.binding.Target.RoomID || cursor != "" {
		return storage.ObjectPage{}, ErrSource
	}
	page := storage.ObjectPage{}
	for ref, record := range c.records {
		if record.Role == "index" {
			page.Entries = append(page.Entries, storage.RecordEntry{Reference: ref, Record: cloneTransferRecord(record)})
		}
	}
	return page, nil
}

func cloneTransferRecord(record storage.Record) storage.Record {
	record.Payload = bytes.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	return record
}

func (c *catalogTransferCrypto) ReadStorageRecord(_ context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	record, found := c.records[ref]
	if !found || record.Validate(c.binding.Scope) != nil || record.ObjectID != object || record.Role != role {
		return storage.Record{}, ErrSource
	}
	return cloneTransferRecord(record), nil
}

func (c *catalogTransferCrypto) PutStorageRecordAt(_ context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	if err := c.CheckStoragePublicationTarget(target); err != nil {
		return storage.Reference{}, err
	}
	if record.Validate(c.binding.Scope) != nil {
		return storage.Reference{}, ErrSource
	}
	c.writes++
	if ref, found := c.transactions[transaction]; found {
		before, _ := json.Marshal(c.records[ref])
		after, _ := json.Marshal(record)
		if !bytes.Equal(before, after) {
			return storage.Reference{}, ErrSource
		}
		return ref, nil
	}
	ref := storage.Reference{RoomID: target.RoomID, EventID: "$" + c.prefix + strconv.Itoa(len(c.records)+1)}
	c.transactions[transaction] = ref
	c.records[ref] = cloneTransferRecord(record)
	if c.afterWrite != nil {
		c.afterWrite()
	}
	return ref, nil
}

func (c *catalogTransferCrypto) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	payload, err := json.Marshal(commit)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	previous := make([]string, len(commit.Parents))
	for i, parent := range commit.Parents {
		previous[i] = parent.EventID
	}
	record := storage.Record{Version: 1, Scope: c.binding.Scope, ObjectID: object, RevisionID: revision, Role: "index", Payload: payload, Previous: previous}
	ref, err := c.PutStorageRecordAt(ctx, target, transaction, record)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	version, err := storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: record}, c.binding.Scope)
	if err == nil && c.loseIndex {
		c.loseIndex = false
		return version, errors.New("synthetic accepted index lost its acknowledgment")
	}
	return version, err
}

func (c *catalogTransferCrypto) StorageObject(ctx context.Context, ref storage.Reference, object string) (storage.ObjectVersion, error) {
	record, err := c.ReadStorageRecord(ctx, ref, object, "index")
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: record}, c.binding.Scope)
}

type catalogTransferSource struct {
	state                storage.LegacyMigrationState
	pages, opens, begins int
}

func (s *catalogTransferSource) PrivateStorageState(_ context.Context, scope storage.Scope) (storage.LegacyMigrationState, error) {
	if s.state.Validate(scope) != nil {
		return storage.LegacyMigrationState{}, ErrSource
	}
	return s.state, nil
}

func (s *catalogTransferSource) BeginPrivateStorageMigration(context.Context, storage.Scope, string) (storage.LegacyMigrationState, error) {
	s.begins++
	return storage.LegacyMigrationState{}, errors.New("encrypted migration must not restart SQL import")
}

func (s *catalogTransferSource) PrivateStorageSourcePage(context.Context, storage.Scope, string, storage.LegacySourceCursor) (storage.LegacySourcePage, error) {
	s.pages++
	return storage.LegacySourcePage{}, errors.New("encrypted migration must not read SQL pages")
}

func (s *catalogTransferSource) OpenPrivateStorageSource(context.Context, storage.Scope, string, string, string, int64) (io.ReadCloser, error) {
	s.opens++
	return nil, errors.New("encrypted migration must not read SQL blobs")
}

type catalogTransferFixture struct {
	source, target *Repository
	old, next      *catalogTransferCrypto
	metadata       *catalogTransferSource
	journal        *offline.Store
	root, child    string
	versions       []storage.ObjectVersion
}

func newCatalogTransferFixture(t *testing.T) *catalogTransferFixture {
	t.Helper()
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 10, 29, 12, 0, 0, 123456789, time.UTC)
	f := &catalogTransferFixture{old: newCatalogTransferCrypto(readerScope, "source"), next: newCatalogTransferCrypto(readerScope, "isolated"), journal: journal, root: "root", child: "child"}
	f.metadata = &catalogTransferSource{state: storage.LegacyMigrationState{Scope: readerScope, Mode: "frozen", Epoch: readerEpoch, StartedAt: &started, Rows: 2, Blobs: 0}}
	config := func(crypto Crypto) Config {
		return Config{Context: t.Context(), Scope: readerScope, Crypto: crypto, Gateway: api.NewDrive("https://unused.invalid", nil, nil), Source: f.metadata, Journal: journal}
	}
	f.source, err = New(config(f.old))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.source.Close)
	f.target, err = New(config(f.next))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.target.Close)
	receipt := completion{Kind: migrationKind, Epoch: readerEpoch, Rows: 2, Blobs: 0, InventorySHA256: digest([]byte("retained SQL inventory"))}
	payload, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	object := "drive_migration_" + digest([]byte(readerEpoch))
	f.addRecordVersion(t, object, "receipt", payload, payload, "live", nil)
	f.addFolder(t, f.root, "root-base", "Mi unidad", true, "", nil)
	base := f.addFolder(t, f.child, "child-base", "before", false, "", nil)
	left := f.addFolder(t, f.child, "child-left", "left", false, "", []storage.Reference{base.Reference})
	right := f.addFolder(t, f.child, "child-right", "right", false, "", []storage.Reference{base.Reference})
	f.addFolder(t, f.child, "child-trash", "right", false, "2026-10-29T21:01:02.123456789Z", []storage.Reference{right.Reference})
	item, err := decodeSummary(left)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	f.addRecordVersion(t, objectID("folder", f.child), "child-delete", metadata, nil, "deleted", []storage.Reference{left.Reference})
	if err := f.source.refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.source.ready = true
	return f
}

func (f *catalogTransferFixture) addRecordVersion(t *testing.T, object, revision string, metadata, payload []byte, state string, parents []storage.Reference) storage.ObjectVersion {
	t.Helper()
	var content *storage.ContentPointer
	if payload != nil {
		record := storage.Record{Version: 1, Scope: readerScope, ObjectID: object, RevisionID: revision + "-content", Role: "content", Payload: payload}
		ref, err := f.old.PutStorageRecordAt(t.Context(), f.old.binding.Target, revision+"-content", record)
		if err != nil {
			t.Fatal(err)
		}
		content = &storage.ContentPointer{Kind: "record", RevisionID: record.RevisionID, Reference: ref}
	}
	version, err := f.old.PutStorageObjectAt(t.Context(), f.old.binding.Target, revision+"-index", object, revision+"-index", storage.ObjectCommit{Kind: storage.ObjectKind, State: state, Metadata: metadata, Content: content, Parents: parents})
	if err != nil {
		t.Fatal(err)
	}
	f.versions = append(f.versions, version)
	return version
}

func (f *catalogTransferFixture) addFolder(t *testing.T, id, revision, name string, root bool, deleted string, parents []storage.Reference) storage.ObjectVersion {
	t.Helper()
	fields := map[string]json.RawMessage{"id": mustTransferJSON(t, id), "name": mustTransferJSON(t, name), "is_root": mustTransferJSON(t, root), "updated_at": json.RawMessage(`"2026-10-29T21:00:00.123456789Z"`), "extension": json.RawMessage(`{"integer":900719925474099512345,"scientific":1.234567890123456789e+99}`)}
	if !root {
		fields["parent_id"] = mustTransferJSON(t, f.root)
	}
	if deleted != "" {
		fields["deleted_at"] = mustTransferJSON(t, deleted)
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	item, err := summarize(storage.LegacySourceRow{Kind: "folder", ID: id, Payload: raw}, readerEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if len(parents) > 0 {
		var original summary
		parent, err := f.old.StorageObject(t.Context(), parents[0], objectID("folder", id))
		if err != nil {
			t.Fatal(err)
		}
		if json.Unmarshal(parent.Commit.Metadata, &original) != nil {
			t.Fatal("invalid fixture parent")
		}
		item.BaselineSHA256, item.Seed = original.BaselineSHA256, false
	}
	metadata, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	envelope := map[string]json.RawMessage{"kind": mustTransferJSON(t, entryKind), "source_kind": mustTransferJSON(t, "folder"), "source_id": mustTransferJSON(t, id), "raw": mustTransferJSON(t, raw), "unrecognized_extension": json.RawMessage(`{"precise":900719925474099512345}`)}
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	state := "live"
	if deleted != "" {
		state = "trash"
	}
	return f.addRecordVersion(t, objectID("folder", id), revision, metadata, payload, state, parents)
}

func mustTransferJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (f *catalogTransferFixture) migrate(t *testing.T) error {
	t.Helper()
	return f.target.MigratePrivateCatalog(t.Context(), f.source, func(context.Context, storage.PublicationTarget, string, storage.Reference, string, string) (storage.Reference, error) {
		t.Error("record-only fixture requested a file or plaintext source")
		return storage.Reference{}, ErrSource
	})
}

func (f *catalogTransferFixture) assertNoPlaintextSource(t *testing.T) {
	t.Helper()
	if f.metadata.pages != 0 || f.metadata.opens != 0 || f.metadata.begins != 0 {
		t.Fatal("private identity migration reimported retained SQL source")
	}
}

func TestPrivateCatalogMigrationPreservesEveryRevisionBranchTrashAndExactJSON(t *testing.T) {
	f := newCatalogTransferFixture(t)
	if err := f.migrate(t); err != nil {
		t.Fatal(err)
	}
	if !f.target.ready {
		t.Fatal("fully verified migration did not activate target")
	}
	oldSnapshot, _, err := f.source.load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	newSnapshot, _, err := f.target.load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(oldSnapshot.Entries) != len(newSnapshot.Entries) || len(oldSnapshot.Entries) != len(f.versions) {
		t.Fatal("migration copied only visible heads")
	}
	byRevision := map[string]storage.ObjectVersion{}
	for _, entry := range newSnapshot.Entries {
		version, err := storage.DecodeObject(entry, readerScope)
		if err != nil {
			t.Fatal(err)
		}
		byRevision[version.RevisionID] = version
	}
	for _, before := range f.versions {
		after, found := byRevision[before.RevisionID]
		if !found || after.ObjectID != before.ObjectID || after.Commit.State != before.Commit.State || !bytes.Equal(after.Commit.Metadata, before.Commit.Metadata) || len(after.Commit.Parents) != len(before.Commit.Parents) {
			t.Fatal("historical metadata, branch or tombstone changed")
		}
		for i, parent := range before.Commit.Parents {
			original, err := f.old.StorageObject(t.Context(), parent, before.ObjectID)
			if err != nil {
				t.Fatal(err)
			}
			if byRevision[original.RevisionID].Reference != after.Commit.Parents[i] {
				t.Fatal("parent relationship still points at legacy identity")
			}
		}
		if before.Commit.Content == nil {
			continue
		}
		old, err := f.old.ReadStorageRecord(t.Context(), before.Commit.Content.Reference, before.ObjectID, "content")
		if err != nil {
			t.Fatal(err)
		}
		next, err := f.next.ReadStorageRecord(t.Context(), after.Commit.Content.Reference, after.ObjectID, "content")
		if err != nil {
			t.Fatal(err)
		}
		var previous, newValue map[string]json.RawMessage
		if json.Unmarshal(old.Payload, &previous) != nil || json.Unmarshal(next.Payload, &newValue) != nil {
			t.Fatal("invalid copied content")
		}
		if !bytes.Equal(previous["raw"], newValue["raw"]) || !bytes.Equal(previous["unrecognized_extension"], newValue["unrecognized_extension"]) {
			t.Fatal("migration rounded or dropped exact domain JSON")
		}
	}
	if len(f.target.versions(objectID("folder", f.child))) != 2 {
		t.Fatal("concurrent trash/delete branches were silently resolved")
	}
	if _, err := f.target.ReadRaw(t.Context(), "folder", f.child); !errors.Is(err, ErrConflict) {
		t.Fatal("conflict no longer requires resolution", err)
	}
	f.assertNoPlaintextSource(t)
}

func TestPrivateCatalogMigrationRejectsSourceObservationBehindTransferScan(t *testing.T) {
	f := newCatalogTransferFixture(t)
	// Another device writes after the repository observation was captured but
	// before the frozen transfer scan. Until transfer supplies its exact source
	// observation to domain validation, refusing the migration is safer than
	// combining the two observations.
	f.addFolder(t, "late", "late-base", "Late", false, "", nil)
	before := f.next.writes
	if err := f.migrate(t); !errors.Is(err, ErrSource) {
		t.Fatalf("stale source observation was not rejected: %v", err)
	}
	if f.next.writes != before || f.target.ready {
		t.Fatal("stale source observation caused a partial transfer")
	}
}

func TestPrivateCatalogMigrationLostIndexACKReopensAndResumesExactGraph(t *testing.T) {
	f := newCatalogTransferFixture(t)
	f.next.loseIndex = true
	if err := f.migrate(t); err == nil || f.target.ready {
		t.Fatal("lost ACK exposed partially copied target", err)
	}
	first := len(f.next.records)
	config := f.target.cfg
	f.target.Close()
	replacement, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(replacement.Close)
	f.target = replacement
	if err := f.migrate(t); err != nil {
		t.Fatal("same intent could not resume", err)
	}
	if !f.target.ready || len(f.next.records) != len(f.old.records) || first == 0 {
		t.Fatal("recovery duplicated or omitted retained content")
	}
	f.assertNoPlaintextSource(t)
}

func TestPrivateCatalogMigrationRejectsChangingFencesInventoryAndForeignDestination(t *testing.T) {
	for _, fault := range []string{"not-frozen", "fence-changed", "inventory-changed", "destination-extra", "overlap", "destination-scope"} {
		t.Run(fault, func(t *testing.T) {
			f := newCatalogTransferFixture(t)
			switch fault {
			case "not-frozen":
				f.old.fenceError = errors.New("synthetic source still writable")
			case "fence-changed":
				f.next.afterWrite = func() { f.old.fence = "$refrozen-after-write" }
			case "inventory-changed":
				f.next.afterWrite = func() { f.next.afterWrite = nil; f.addFolder(t, "late", "late-arrival", "late", false, "", nil) }
			case "destination-extra":
				record := storage.Record{Version: 1, Scope: readerScope, ObjectID: "unexpected", RevisionID: "unexpected", Role: "index", Payload: mustTransferJSON(t, storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{}`)})}
				ref := storage.Reference{RoomID: f.next.binding.Target.RoomID, EventID: "$unexpected"}
				f.next.records[ref] = record
			case "overlap":
				f.next.binding = f.old.binding
			case "destination-scope":
				f.next.binding.Scope.Tenant = "other"
			}
			if err := f.migrate(t); err == nil || f.target.ready {
				t.Fatal("unstable source or destination became ready", fault, err)
			}
			f.assertNoPlaintextSource(t)
		})
	}
}

func TestPrivateCatalogMigrationRejectsAlteredDurablePlansAndMappingsBeforeRetryWrites(t *testing.T) {
	for _, fault := range []string{"inventory", "scope", "target", "salt", "mapping-input", "mapping-result"} {
		t.Run(fault, func(t *testing.T) {
			f := newCatalogTransferFixture(t)
			f.next.loseIndex = true
			if err := f.migrate(t); err == nil {
				t.Fatal("fixture failed to interrupt")
			}
			var plan identityMigration
			found, err := f.journal.Read(f.target.owner(), identityMigrationNamespace, "plan", &plan)
			if err != nil || !found {
				t.Fatal(err)
			}
			switch fault {
			case "inventory":
				plan.Inventory = digest([]byte("other"))
			case "scope":
				plan.Scope.Tenant = "other"
			case "target":
				plan.Destination.Target.DeviceID = "replacement-device"
			case "salt":
				plan.Salt = "invalid salt"
			case "mapping-input", "mapping-result":
				transfer := identityTransfer{target: f.target, state: plan}
				key := ""
				var mapping identityMapping
				for _, version := range f.versions {
					candidate := transfer.transaction(version.Reference, "index")
					found, err = f.journal.Read(f.target.owner(), identityMigrationNamespace, candidate, &mapping)
					if err != nil {
						t.Fatal(err)
					}
					if found {
						key = candidate
						break
					}
				}
				if key == "" {
					t.Fatal("missing partial index mapping")
				}
				if fault == "mapping-input" {
					mapping.Input = digest([]byte("other"))
				} else {
					mapping.Result = &storage.Reference{RoomID: "!foreign:test", EventID: "$foreign"}
				}
				if err := f.journal.Write(f.target.owner(), identityMigrationNamespace, key, mapping); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.journal.Write(f.target.owner(), identityMigrationNamespace, "plan", plan); err != nil {
				t.Fatal(err)
			}
			writes := f.next.writes
			if err := f.migrate(t); err == nil || f.target.ready {
				t.Fatal("altered durable intent became ready", fault, err)
			}
			if (fault != "mapping-input" && fault != "mapping-result") && writes != f.next.writes {
				t.Fatal("invalid plan rejected after a retry write")
			}
			f.assertNoPlaintextSource(t)
		})
	}
}

func TestPrivateCatalogMigrationRejectsAnAlreadyVisibleDestinationBeforePublication(t *testing.T) {
	f := newCatalogTransferFixture(t)
	f.target.ready = true
	if err := f.migrate(t); !errors.Is(err, ErrSource) {
		t.Fatal("migration replaced an already visible private repository", err)
	}
	if f.next.writes != 0 || len(f.next.records) != 0 {
		t.Fatal("visible target was rejected after publication")
	}
	f.assertNoPlaintextSource(t)
}

func TestPrivateCatalogMigrationPreservesUnpublishedSourceWork(t *testing.T) {
	for _, fault := range []string{"mutation", "upload", "malformed", "late", "completed-begin"} {
		t.Run(fault, func(t *testing.T) {
			f := newCatalogTransferFixture(t)
			namespace := migrationNamespace
			key := "pending-mutation"
			var value any = map[string]any{"target": f.old.binding.Target}
			if fault == "upload" {
				namespace = storage.PendingUploadsNamespace
			}
			if fault == "malformed" {
				value = map[string]any{"unknown": "retained work"}
			}
			if fault == "completed-begin" {
				key = "begin_retained"
				value = beginWork{Scope: readerScope, Epoch: readerEpoch, Target: f.old.binding.Target}
			}
			write := func() {
				if err := f.journal.Write(f.source.owner(), namespace, key, value); err != nil {
					t.Fatal(err)
				}
			}
			if fault == "late" {
				f.next.afterWrite = write
			} else {
				write()
			}
			err := f.migrate(t)
			if fault == "completed-begin" {
				if err != nil {
					t.Fatal("completed original import intent blocked migration", err)
				}
				return
			}
			if err == nil || f.target.ready {
				t.Fatal("unpublished source work was omitted from cutover", fault, err)
			}
			var retained map[string]any
			if found, err := f.journal.Read(f.source.owner(), namespace, key, &retained); err != nil || !found {
				t.Fatal("migration deleted pending source work", err)
			}
			f.assertNoPlaintextSource(t)
		})
	}
}

func (c *catalogTransferCrypto) StorageIdentityVersion() storage.IdentityVersion {
	if c.prefix == "isolated" {
		return storage.IsolatedIdentity
	}
	return storage.LegacyIdentity
}

func TestIsolatedInitializationCannotReimportSQLBeforeFinalReceipt(t *testing.T) {
	f := newCatalogTransferFixture(t)
	if err := f.target.Initialize(t.Context()); !errors.Is(err, ErrNotReady) || f.target.ready {
		t.Fatal("isolated namespace fell back to SQL import", err)
	}
	f.assertNoPlaintextSource(t)
	// A separate device has a different encrypted journal and sees remote writes
	// immediately; the writer's local migration lock is not a readiness barrier.
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	observations := 0
	f.next.afterWrite = func() {
		readerConfig := f.target.cfg
		readerConfig.Journal = journal
		reader, err := New(readerConfig)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		err = reader.Initialize(t.Context())
		if err == nil {
			for _, version := range f.versions {
				if version.ObjectID == "drive_migration_"+digest([]byte(readerEpoch)) {
					continue
				}
				found := false
				for _, record := range f.next.records {
					if record.Role == "index" && record.ObjectID == version.ObjectID && record.RevisionID == version.RevisionID {
						found = true
						break
					}
				}
				if !found {
					t.Fatal("reader became ready before retained history was copied", version.RevisionID)
				}
			}
		} else if !errors.Is(err, ErrNotReady) {
			t.Fatal("partial reader failed outside readiness gate", err)
		}
		observations++
	}
	if err := f.migrate(t); err != nil {
		t.Fatal(err)
	}
	if observations == 0 {
		t.Fatal("test did not observe partial remote writes")
	}
	f.assertNoPlaintextSource(t)
}

func TestPrivateCatalogMigrationRechecksEveryCommitAfterCompletionPublication(t *testing.T) {
	f := newCatalogTransferFixture(t)
	changed := false
	f.next.afterWrite = func() {
		complete := false
		for _, record := range f.next.records {
			if record.Role == "index" && record.ObjectID == "drive_migration_"+digest([]byte(readerEpoch)) {
				complete = true
			}
		}
		if !complete || changed {
			return
		}
		for ref, record := range f.next.records {
			if record.Role != "index" || record.ObjectID != objectID("folder", f.root) {
				continue
			}
			var commit storage.ObjectCommit
			var fields map[string]json.RawMessage
			if json.Unmarshal(record.Payload, &commit) != nil || json.Unmarshal(commit.Metadata, &fields) != nil {
				t.Fatal("invalid fixture commit")
			}
			fields["name"] = json.RawMessage(`"tampered after data verification"`)
			commit.Metadata = mustTransferJSON(t, fields)
			record.Payload = mustTransferJSON(t, commit)
			f.next.records[ref] = record
			changed = true
			return
		}
	}
	if err := f.migrate(t); err == nil || f.target.ready || !changed {
		t.Fatal("late metadata substitution became a ready catalog", err, changed)
	}
	f.assertNoPlaintextSource(t)
}
