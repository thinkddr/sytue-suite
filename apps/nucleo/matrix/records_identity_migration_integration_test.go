// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

const recordsFixtureEntity = "org.sytue.contacts.fixture.entity.v1"

type recordsHistoryACK struct {
	*Client
	lost     bool
	accepted map[string]storage.Reference
}

func (c *recordsHistoryACK) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	version, err := c.Client.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
	if err != nil {
		return version, err
	}
	if previous, found := c.accepted[txn]; found && previous != version.Reference {
		return version, errors.New("accepted record index duplicated on retry")
	}
	c.accepted[txn] = version.Reference
	var header struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(commit.Metadata, &header) == nil && header.Kind == recordsFixtureEntity && !c.lost {
		c.lost = true
		return version, errors.New("synthetic accepted record index acknowledgement lost")
	}
	return version, nil
}

// Observe the real transfer boundary through its SDK source. An internal
// destination verification must not publish ready before the final source pin.
type recordsFrozenSource struct {
	*Client
	target  *records.Repository
	t       *testing.T
	enforce bool
	checks  int
}

func (c *recordsFrozenSource) FrozenStorageBindings(ctx context.Context) (storage.FrozenBinding, error) {
	if c.enforce && c.target != nil {
		c.checks++
		if _, err := c.target.ReadRaw(ctx, "contact", "small"); !errors.Is(err, records.ErrNotReady) {
			c.t.Error("target ready before final transfer boundary", err)
		}
	}
	return c.Client.FrozenStorageBindings(ctx)
}

func TestSharedRecordHistoryCutoverAgainstSynapse(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_RECORD_CUTOVER") != "1" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	upstream, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_MATRIX_RECORD_CUTOVER_URL"))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "contacts"}
	old, controller := controlledMigrationSource(ctx, t, proxy.URL, proxy.Client(), scope)
	next := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	gateway := &fileGatewayFixture{product: scope.Product, policyScope: scope}
	server := httptest.NewServer(gateway)
	t.Cleanup(server.Close)
	blobs := api.NewDrive(server.URL, nil, server.Client())
	small := []byte(`{"id":"small","name":"private old contact","future":{"n":9007199254740993}}`)
	large := []byte(`{"id":"large","name":"private large contact","notes":"` + strings.Repeat("exact private contact ", 4096) + `","future":{"n":900719925474099312345,"e":1.234567890123456789e+99}}`)
	source := &notesRetainedSourceFixture{state: records.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, rows: map[string][]byte{"contact/small": small, "contact/large": large}}
	secrets := &authtest.Secrets{}
	directory := t.TempDir()
	journal, err := offline.OpenWithSecrets(directory, secrets)
	if err != nil {
		t.Fatal(err)
	}
	cfg := func(crypto records.Crypto) records.Config {
		return records.Config{
			Context: ctx, Scope: scope, Product: scope.Product, EntityKind: recordsFixtureEntity, ReceiptKind: "org.sytue.contacts.fixture.receipt.v1", Crypto: crypto, Gateway: blobs, Source: source, Journal: journal,
			WorkLease: func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() },
			ValidateRow: func(_ context.Context, kind, id string, raw []byte) error {
				var row struct {
					ID string `json:"id"`
				}
				if kind != "contact" || json.Unmarshal(raw, &row) != nil || row.ID != id {
					return errors.New("changed private contact identity")
				}
				return nil
			},
			ValidateCatalog: func(ctx context.Context, catalog records.Catalog) error {
				entries, err := catalog.Entries()
				if err != nil {
					return err
				}
				for _, entry := range entries {
					raw, err := catalog.ReadJSON(ctx, entry)
					clear(raw)
					if err != nil {
						return err
					}
				}
				return nil
			},
		}
	}
	if _, err := records.New(cfg(old.Client)); !errors.Is(err, records.ErrSource) {
		t.Fatal("ordinary repository admitted legacy identity", err)
	}
	frozenSource := &recordsFrozenSource{Client: old.Client, t: t, enforce: true}
	historical, err := records.NewHistoricalSource(cfg(frozenSource))
	if err != nil {
		t.Fatal(err)
	}
	defer historical.Close()
	routing := &driveCutoverRouting{state: storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: "legacy", SourceRooms: []string{old.Room.String()}}}
	if err := historical.Prepare(ctx, routing.state); err != nil {
		t.Fatal("explicit historical source preparation", err)
	}
	baseline, err := storage.LoadCatalog(ctx, old.Client, scope)
	if err != nil {
		t.Fatal(err)
	}
	var previous storage.ObjectVersion
	for _, version := range baseline.Heads {
		var row struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(version.Commit.Metadata, &row) == nil && row.ID == "small" {
			previous = version
		}
	}
	if !previous.Reference.Valid() {
		t.Fatal("baseline contact missing")
	}
	changed := []byte(`{"id":"small","name":"private edit after SQL snapshot","future":{"n":900719925474099312345}}`)
	payload, _ := json.Marshal(map[string]any{"kind": recordsFixtureEntity, "json": changed})
	body := storage.Record{Version: 1, Scope: scope, ObjectID: previous.ObjectID, RevisionID: "edited_body", Role: "content", Payload: payload}
	binding, err := old.Client.StorageBindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := old.Client.PutStorageRecordAt(ctx, binding.Target, "records_private_edit_body", body)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]json.RawMessage
	if json.Unmarshal(previous.Commit.Metadata, &metadata) != nil {
		t.Fatal("invalid baseline metadata")
	}
	metadata["sha256"], _ = json.Marshal(digestBytes(changed))
	metadata["seed"] = json.RawMessage("false")
	encoded, _ := json.Marshal(metadata)
	for i, state := range []string{"live", "deleted", "live"} {
		commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: state, Metadata: encoded, Parents: []storage.Reference{previous.Reference}, Content: &storage.ContentPointer{Kind: "record", RevisionID: body.RevisionID, Reference: ref}}
		if state == "deleted" {
			commit.Content = nil
		}
		binding, err := old.Client.StorageBindings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		previous, err = old.Client.PutStorageObjectAt(ctx, binding.Target, "records_private_edit_"+string(rune('a'+i)), previous.ObjectID, "edited_index_"+string(rune('a'+i)), commit)
		if err != nil {
			t.Fatal(err)
		}
	}
	before, err := storage.LoadCatalog(ctx, old.Client, scope)
	if err != nil {
		t.Fatal(err)
	}
	pages, opens := source.pages, source.opens
	if _, err := controller.SendStateEvent(ctx, old.Room, event.StatePowerLevels, "", migrationFixturePowers(true)); err != nil {
		t.Fatal(err)
	}
	routing.state.Phase = "frozen"
	routing.state.Epoch = uuid.NewString()
	faults := &recordsHistoryACK{Client: next.Client, accepted: map[string]storage.Reference{}}
	copyFile := func(ctx context.Context, target storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
		return old.Client.MigrateStorageFileContentAt(ctx, next.Client, blobs, journal, job, target, ref, object, revision)
	}
	var target *records.Repository
	for attempt := range 2 {
		target, err = records.New(cfg(faults))
		if err != nil {
			t.Fatal(err)
		}
		frozenSource.target = target
		cutover, err := records.NewIdentityCutover(records.IdentityCutoverConfig{Scope: scope, Service: routing, Legacy: historical, Isolated: target, CopyFile: copyFile, Authorize: func(ctx context.Context) error { return ctx.Err() }})
		if err != nil {
			t.Fatal(err)
		}
		err = cutover.Prepare(ctx)
		if err == nil {
			frozenSource.enforce = false
			break
		}
		if attempt != 0 || !faults.lost {
			t.Fatal("unexpected cutover failure", err)
		}
		if _, err := target.ReadRaw(ctx, "contact", "small"); !errors.Is(err, records.ErrNotReady) {
			t.Fatal("partial target exposed", err)
		}
		target.Close()
		journal, err = offline.OpenWithSecrets(directory, secrets)
		if err != nil {
			t.Fatal(err)
		}
	}
	defer target.Close()
	if err != nil || routing.state.Phase != "isolated" || !faults.lost || frozenSource.checks == 0 {
		t.Fatal("record cutover not complete", err)
	}
	after, err := storage.LoadCatalog(ctx, next.Client, scope)
	if err != nil || len(after.Entries) != len(before.Entries) {
		t.Fatal("history count changed", len(before.Entries), len(after.Entries), err)
	}
	for id, want := range map[string][]byte{"small": changed, "large": large} {
		raw, err := target.ReadRaw(ctx, "contact", id)
		if err != nil || !bytes.Equal(raw, want) {
			t.Fatal("exact private record changed", id, err)
		}
		clear(raw)
	}
	if source.pages != pages || source.opens != opens {
		t.Fatal("cutover reimported retained SQL")
	}
	pending, err := target.Pending(ctx)
	if err != nil || pending.Count != 0 {
		t.Fatal("acknowledged transfer remained active", pending, err)
	}
	ids, err := journal.RecordIDs(scope.Tenant+"\x00"+scope.Subject+"\x00"+scope.Product, storage.IdentityHistoryNamespace(scope.Product))
	if err != nil || len(ids) == 0 {
		t.Fatal("acknowledged evidence was not retained", err)
	}
	anonymous, err := mautrix.NewClient(proxy.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	anonymous.Client = proxy.Client()
	login, err := anonymous.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: next.Matrix.UserID.String()}, Password: next.Password, DeviceID: "RECORDS_FRESH"})
	if err != nil {
		t.Fatal(err)
	}
	freshCfg := next.Config
	freshCfg.StoreDir = t.TempDir()
	freshCfg.Secrets = &memorySecrets{values: map[string][]byte{}}
	fresh := storageSynapseSession(ctx, t, freshCfg, credentials{Homeserver: proxy.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: scope.Subject, Tenant: scope.Tenant})
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Recover(ctx, next.Recovery); err != nil {
		t.Fatal("independent SDK recovery", err)
	}
	journal, err = offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := records.New(cfg(fresh))
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if err := recovered.Initialize(ctx); err != nil {
		t.Fatal("fresh SDK and empty journal", err)
	}
	raw, err := recovered.ReadRaw(ctx, "contact", "small")
	if err != nil || !bytes.Equal(raw, changed) {
		t.Fatal("fresh recovery lost private edit", err)
	}
	clear(raw)
	if source.pages != pages || source.opens != opens {
		t.Fatal("fresh SDK read SQL bodies")
	}
	savedState := source.state
	begins := source.begins
	source.state = records.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}
	err = historical.Prepare(ctx, storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: "legacy", SourceRooms: []string{old.Room.String()}})
	source.state = savedState
	if !errors.Is(err, records.ErrSource) || source.begins != begins || source.pages != pages || source.opens != opens {
		t.Fatal("unreconciled private history was treated as a new SQL baseline", err)
	}
}
