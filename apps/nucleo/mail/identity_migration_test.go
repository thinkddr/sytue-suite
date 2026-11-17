// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type transferMailCrypto struct {
	*cryptoFixture
	beforeIndex func(context.Context, storage.ObjectCommit) error
}

func (c *transferMailCrypto) FrozenStorageBindings(context.Context) (storage.FrozenBinding, error) {
	return storage.FrozenBinding{Binding: c.binding, Fences: []storage.Reference{{RoomID: c.binding.Target.RoomID, EventID: "$frozen"}}}, nil
}

func (c *transferMailCrypto) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	if c.beforeIndex != nil {
		if err := c.beforeIndex(ctx, commit); err != nil {
			return storage.ObjectVersion{}, err
		}
	}
	return c.cryptoFixture.PutStorageObjectAt(ctx, target, transaction, object, revision, commit)
}

func mailTransferLease(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }

func TestMailCatalogTransferRewritesRealRepositoryDocumentsAndRetainsReceiptUntilVerified(t *testing.T) {
	f := setupImport(t)
	for i, row := range f.source.rows {
		if row.Kind != "message" || row.ID != "live" {
			continue
		}
		key := storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}
		raw, err := patchField(f.source.bodies[key], "future", json.RawMessage(`{"number":9007199254740993,"pad":"`+strings.Repeat("x", 60000)+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		f.source.rows[i].Size = int64(len(raw))
		f.source.rows[i].SHA256 = digest(raw)
		f.source.bodies[key] = raw
	}
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	err := f.repo.run(t.Context(), true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		raw, err := f.repo.document(ctx, s, "migration", testEpoch)
		if err != nil {
			return err
		}
		defer clear(raw)
		value, err := patchField(raw, "future", json.RawMessage(`{"number":9007199254740993,"pad":"`+strings.Repeat("y", 60000)+`"}`))
		if err != nil {
			return err
		}
		defer clear(value)
		return f.repo.writeDocument(ctx, s, "migration", testEpoch, value, "live")
	})
	if err != nil {
		t.Fatal(err)
	}
	var originalEnvelope []byte
	err = f.repo.run(t.Context(), true, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		raw, err := f.repo.document(ctx, s, "message", "live")
		if err != nil {
			return err
		}
		defer clear(raw)
		_, doc, err := decodeMessage(raw, "live")
		originalEnvelope = doc.Envelope
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer clear(originalEnvelope)
	f.repo.Close()
	f.crypto.identity = storage.LegacyIdentity
	old := &transferMailCrypto{cryptoFixture: f.crypto}
	cfg := f.cfg
	cfg.Crypto = old
	cfg.Files = nil
	cfg.WorkLease = mailTransferLease
	if _, err := New(cfg); !errors.Is(err, ErrRecord) {
		t.Fatal("legacy source became an ordinary Mail repository", err)
	}
	source, err := NewHistoricalSource(cfg, f.source)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	routing := storage.IdentityCutoverState{Version: 1, Scope: testScope, Phase: "frozen", Epoch: testEpoch, SourceRooms: old.binding.Rooms}
	putsBefore := old.puts
	if err := source.Prepare(t.Context(), routing); err != nil {
		t.Fatal(err)
	}
	if old.puts != putsBefore {
		t.Fatal("frozen source check changed the catalog")
	}
	next := &transferMailCrypto{cryptoFixture: newCryptoFixture()}
	next.binding.Target = storage.PublicationTarget{RoomID: "!isolated:test", DeviceID: "ISOLATED"}
	next.binding.Rooms = []string{next.binding.Target.RoomID}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	targetCfg := cfg
	targetCfg.Crypto = next
	targetCfg.Journal = journal
	target, err := New(targetCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { target.Close() }()
	copyFile := func(ctx context.Context, destination storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
		var data bytes.Buffer
		request, err := old.DownloadStorageFile(ctx, cfg.Gateway, ref, object, revision, &data)
		if err != nil {
			return storage.Reference{}, err
		}
		defer clear(data.Bytes())
		return next.UploadStorageFileAt(ctx, targetCfg.Gateway, journal, job, destination, request, bytes.NewReader(data.Bytes()))
	}
	receipts := 0
	next.beforeIndex = func(ctx context.Context, commit storage.ObjectCommit) error {
		var h header
		if json.Unmarshal(commit.Metadata, &h) != nil {
			return ErrRecord
		}
		if h.Type != "migration" {
			return nil
		}
		receipts++
		snapshot, err := storage.LoadAtomicCatalog(ctx, next, testScope)
		if err != nil {
			return err
		}
		raw, err := target.document(ctx, snapshot, "message", "live")
		if err != nil {
			t.Fatal("receipt preceded verified message", err)
		}
		defer clear(raw)
		_, doc, err := decodeMessage(raw, "live")
		defer clear(doc.Envelope)
		if err != nil || !bytes.Equal(doc.Envelope, originalEnvelope) || doc.MIME.RoomID != next.binding.Target.RoomID {
			t.Fatal("message rewrite changed original envelope or retained old MIME ref", err)
		}
		entry, ok := snapshot.Entries[doc.MIME]
		if !ok {
			t.Fatal("receipt preceded MIME index")
		}
		version, err := storage.DecodeObject(entry, testScope)
		if err != nil {
			t.Fatal(err)
		}
		var mime bytes.Buffer
		_, err = next.DownloadStorageFile(ctx, targetCfg.Gateway, version.Commit.Content.Reference, version.ObjectID, version.Commit.Content.RevisionID, &mime)
		if err != nil || mime.String() != "original" {
			t.Fatal("receipt preceded exact MIME readback", err)
		}
		return nil
	}
	next.loseIndex = true
	if err := target.MigratePrivateCatalog(t.Context(), source, copyFile); err == nil {
		t.Fatal("lost accepted index ACK was hidden")
	}
	if target.ready || receipts != 0 {
		t.Fatal("incomplete transfer exposed mailbox or receipt")
	}
	pending, err := target.Pending(t.Context())
	if err != nil || pending.Count != 1 {
		t.Fatal("transfer mappings were counted as independent user operations", pending, err)
	}
	target.Close()
	target, err = New(targetCfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.MigratePrivateCatalog(t.Context(), source, copyFile); err != nil {
		t.Fatal(err)
	}
	if !target.ready || receipts == 0 {
		t.Fatal("verified transfer remained unavailable")
	}
	body, err := target.Raw(t.Context(), "live")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(raw) != "original" {
		t.Fatal("ordinary private MIME reader changed after transfer", err)
	}
	snapshot, err := storage.LoadAtomicCatalog(t.Context(), next, testScope)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range snapshot.Heads {
		h, err := decodeHeader(version)
		if err != nil {
			t.Fatal(err)
		}
		if h.Type != "message" && h.Type != "migration" {
			continue
		}
		if version.Commit.Content.Kind != "file" {
			t.Fatal("large domain document was not transferred through the file path", h.Type)
		}
		raw, err := target.readDocument(t.Context(), version)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(raw, []byte(`9007199254740993`)) && h.Type == "migration" {
			t.Fatal("receipt lost unknown exact integer")
		}
		if bytes.Contains(raw, []byte(old.binding.Target.RoomID)) {
			t.Fatal("domain document retained a source SDK reference", h.Type)
		}
		clear(raw)
	}
	if f.source.pages == 0 {
		t.Fatal("fixture never imported retained source")
	}
}

func TestMailHistoricalSourceNeverImportsMissingReceiptAfterIdentityFreeze(t *testing.T) {
	f := setupImport(t)
	f.repo.Close()
	f.crypto.identity = storage.LegacyIdentity
	old := &transferMailCrypto{cryptoFixture: f.crypto}
	cfg := f.cfg
	cfg.Crypto = old
	cfg.Files = nil
	cfg.WorkLease = mailTransferLease
	source, err := NewHistoricalSource(cfg, f.source)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	routing := storage.IdentityCutoverState{Version: 1, Scope: testScope, Phase: "frozen", Epoch: testEpoch, SourceRooms: old.binding.Rooms}
	if err := source.Prepare(t.Context(), routing); !errors.Is(err, ErrNotReady) {
		t.Fatal("missing frozen receipt was not blocked", err)
	}
	if f.source.pages != 0 || f.source.opens != 0 || old.puts != 0 {
		t.Fatal("frozen missing receipt triggered SQL reimport")
	}
}

func TestMailProjectionComparisonPermitsOnlyOuterMessageOrderAndPreservesExactEnvelope(t *testing.T) {
	before, _ := json.Marshal(MessageDocument{Kind: messageKind, Envelope: []byte(`{ "future":9007199254740993 }`), MIME: storage.Reference{RoomID: "!mail:test", EventID: "$mime"}})
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(before, &fields); err != nil {
		t.Fatal(err)
	}
	ordered, _ := json.Marshal(fields)
	if !sameProjectionPayload("message", before, ordered) {
		t.Fatal("equivalent outer field order was rejected")
	}
	if sameProjectionPayload("mailbox", before, ordered) {
		t.Fatal("non-message source archive lost its exact-byte requirement")
	}
	fields["envelope"], _ = json.Marshal([]byte(`{"future":9007199254740993}`))
	changed, _ := json.Marshal(fields)
	if sameProjectionPayload("message", before, changed) {
		t.Fatal("message comparison changed exact envelope bytes")
	}
}

type enrollingMailSource struct {
	*retainedMailSource
	accepted   storage.LegacyMigrationState
	journal    *offline.Store
	beginCalls int
	loseACK    bool
}

func (s *enrollingMailSource) BeginPrivateStorageMigration(_ context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	s.beginCalls++
	var intent struct {
		Epoch string `json:"epoch"`
	}
	found, err := s.journal.Read(scope.Tenant+"\x00"+scope.Subject+"\x00mail", importNamespace, "snapshot", &intent)
	if err != nil || !found || intent.Epoch != epoch {
		return storage.LegacyMigrationState{}, ErrRecord
	}
	s.state = s.accepted
	s.state.Epoch = epoch
	if s.loseACK {
		s.loseACK = false
		return storage.LegacyMigrationState{}, errors.New("accepted source begin reply lost")
	}
	return s.state, nil
}

func TestMailHistoricalEnrollmentRetainsAcceptedSourceEpochAndBlocksUnreconciledCatalog(t *testing.T) {
	for _, fault := range []string{"lost-ack", "unreconciled-catalog", "shared", "ingress-unavailable"} {
		t.Run(fault, func(t *testing.T) {
			f := setupImport(t)
			f.repo.Close()
			f.crypto.identity = storage.LegacyIdentity
			old := &transferMailCrypto{cryptoFixture: f.crypto}
			cfg := f.cfg
			cfg.Crypto = old
			cfg.Files = nil
			cfg.WorkLease = mailTransferLease
			source := &enrollingMailSource{retainedMailSource: f.source, accepted: f.source.state, journal: cfg.Journal, loseACK: fault == "lost-ack"}
			source.state = storage.LegacyMigrationState{Scope: testScope, Mode: "legacy"}
			switch fault {
			case "unreconciled-catalog":
				old.seed(t, "mailbox", "old", map[string]string{"id": "old", "name": "Old"})
			case "shared":
				source.shared = true
			case "ingress-unavailable":
				source.unavailableIngress = true
			}
			historical, err := OpenHistoricalSource(t.Context(), cfg, source)
			if err == nil || historical != nil {
				if historical != nil {
					historical.Close()
				}
				t.Fatal("fault did not block source enrollment", fault, err)
			}
			if fault != "lost-ack" {
				if source.beginCalls != 0 || source.pages != 0 || source.opens != 0 {
					t.Fatal("unsafe source caused enrollment effects")
				}
				return
			}
			if source.beginCalls != 1 {
				t.Fatal("source begin was not attempted exactly once")
			}
			epoch := source.state.Epoch
			historical, err = OpenHistoricalSource(t.Context(), cfg, source)
			if err != nil {
				t.Fatal("lost accepted begin could not reopen exact retained intent", err)
			}
			defer historical.Close()
			if historical.RetainedState().Epoch != epoch || source.beginCalls != 1 {
				t.Fatal("source restart replaced accepted epoch")
			}
			routing := storage.IdentityCutoverState{Version: 1, Scope: testScope, Phase: "legacy", SourceRooms: old.binding.Rooms}
			if err := historical.Prepare(t.Context(), routing); err != nil {
				t.Fatal(err)
			}
			ids, err := cfg.Journal.RecordIDs(testScope.Tenant+"\x00"+testScope.Subject+"\x00mail", importNamespace)
			if err != nil || len(ids) != 0 {
				t.Fatal("verified receipt did not retire exact source intent", ids, err)
			}
		})
	}
}
