// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateNotes "git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var _ privateNotes.Crypto = (*Client)(nil)

type notesRetainedSourceFixture struct {
	state                privateNotes.SnapshotState
	rows                 map[string][]byte
	begins, pages, opens int
}

func (s *notesRetainedSourceFixture) State(context.Context, storage.Scope) (privateNotes.SnapshotState, error) {
	return s.state, nil
}

func (s *notesRetainedSourceFixture) Begin(_ context.Context, scope storage.Scope, epoch string) (privateNotes.SnapshotState, error) {
	s.begins++
	now := time.Now().UTC()
	s.state = privateNotes.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: int64(len(s.rows))}}
	return s.state, nil
}

func (s *notesRetainedSourceFixture) Page(_ context.Context, scope storage.Scope, epoch string, _ storage.LegacySourceCursor) (privateNotes.SnapshotPage, error) {
	s.pages++
	keys := make([]string, 0, len(s.rows))
	for key := range s.rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	page := privateNotes.SnapshotPage{Scope: scope, Epoch: epoch}
	for _, key := range keys {
		kind, sourceID, _ := strings.Cut(key, "/")
		raw := s.rows[key]
		page.Rows = append(page.Rows, privateNotes.SnapshotRow{Kind: kind, ID: sourceID, Size: int64(len(raw)), SHA256: digestBytes(raw)})
	}
	return page, nil
}

func (s *notesRetainedSourceFixture) Open(_ context.Context, _ storage.Scope, _ string, row privateNotes.SnapshotRow) (io.ReadCloser, error) {
	s.opens++
	return io.NopCloser(bytes.NewReader(s.rows[row.Kind+"/"+row.ID])), nil
}

func TestPrivateNotesAgainstSynapseImportsExactJSONResumesAndRecovers(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_NOTES") != "1" {
		t.Skip("requires isolated disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	upstreamURL := privateNotesFixtureTarget(t)
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstreamURL))
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "notes"}
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	gateway := &fileGatewayFixture{product: "notes"}
	blobServer := httptest.NewServer(gateway)
	t.Cleanup(blobServer.Close)
	blobs := api.NewDrive(blobServer.URL, nil, blobServer.Client())
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	notebook, _ := json.Marshal(map[string]any{"id": "book", "owner_sub": scope.Subject, "title": "private notebook sentinel", "layout": "list", "visibility": "private", "indexable": true, "updated_at": "2026-10-30T10:00:00.123456789Z"})
	note := []byte(`{"id":"note","owner_sub":"` + scope.Subject + `","notebook_id":"book","kind":"text","width":260,"pinned":true,"title":"private note sentinel","content":"#private ` + strings.Repeat("exact private content ", 4096) + `","data":{"n":900719925474099312345,"scientific":1.234567890123456789e+99},"updated_at":"2026-10-30T10:00:00.123456789Z","future":{"nested":9007199254740993}}`)
	source := &notesRetainedSourceFixture{state: privateNotes.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, rows: map[string][]byte{"notebook/book": notebook, "note/note": note}}
	source.rows["tag/priority"], err = json.Marshal(map[string]any{"id": "priority", "owner_sub": scope.Subject, "name": "Private priority"})
	if err != nil {
		t.Fatal(err)
	}
	source.rows["tag_state/high"] = []byte(`{"id":"high","tag_id":"priority","name":"High","position":1}`)
	source.rows["note_link/link"] = []byte(`{"id":"link","note_id":"note","target_kind":"note","target_id":"another","label":"Private backlink"}`)
	source.rows["reminder/pending"], err = json.Marshal(map[string]any{"note_id": "note", "owner_sub": scope.Subject, "due_at": "2026-10-31T12:00:00Z", "tzid": "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	open := func(crypto privateNotes.Crypto) *privateNotes.Repository {
		t.Helper()
		r, err := privateNotes.New(privateNotes.Config{Context: ctx, Scope: scope, Crypto: crypto, Gateway: blobs, Journal: journal, Source: source})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	lost := &lostStorageIndexACK{Client: device.Client}
	repository := open(lost)
	defer func() { repository.Close() }()
	if err := repository.Initialize(ctx); !errors.Is(err, privateNotes.ErrNotReady) || source.begins != 0 || source.pages != 0 || source.opens != 0 {
		t.Fatal("normal open reimported SQL", err)
	}
	if err := repository.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal("explicit retained import", err)
	}
	raw, err := repository.ReadRaw(ctx, "note", "note")
	if err != nil || !bytes.Equal(raw, note) {
		t.Fatal("source JSON bytes changed", err)
	}
	clear(raw)
	facade, err := api.NewNotes("https://unused.invalid", nil, nil).WithPrivateData(repository)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := facade.ListNotes(ctx, "book")
	if err != nil || len(notes) != 1 {
		t.Fatal("private Notes facade", err)
	}
	input := notes[0].Input()
	input.Title = "updated private sentinel"
	lost.drop = true
	if _, err = facade.UpdateNote(ctx, "note", "book", input); err == nil || lost.drop || !lost.accepted.Valid() {
		t.Fatal("accepted index ACK was not lost", err)
	}
	repository.Close()
	if err = device.Client.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(ctx, device.Config)
	if err != nil {
		t.Fatal("SDK restart", err)
	}
	defer func() { _ = restarted.Close() }()
	go func() { _ = restarted.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = restarted.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	repository = open(restarted)
	if err = repository.Initialize(ctx); err != nil {
		t.Fatal("completed catalog reopen after lost ACK", err)
	}
	updated, err := repository.UpdateNote(ctx, "note", "book", input)
	if err != nil || updated.Title != input.Title {
		t.Fatal("durable mutation did not resume", err)
	}
	snapshot, err := storage.LoadCatalog(ctx, restarted, scope)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, head := range snapshot.Heads {
		if head.Reference == lost.accepted {
			matches++
		}
	}
	if matches != 1 {
		t.Fatal("resume duplicated accepted index", matches)
	}
	before := gateway.puts
	if _, err = repository.UpdateNote(ctx, "note", "book", input); !errors.Is(err, privateNotes.ErrConflict) || gateway.puts != before {
		t.Fatal("completed old CAS retried as new write", err)
	}
	if err = restarted.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	fresh := freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), device)
	journal, err = offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	recovered := open(fresh)
	defer recovered.Close()
	reads := []int{source.begins, source.pages, source.opens}
	if err = recovered.Initialize(ctx); err != nil {
		t.Fatal("fresh recovered Notes catalog", err)
	}
	list, err := recovered.ListNotes(ctx, "book")
	if err != nil || len(list) != 1 || list[0].Title != input.Title {
		t.Fatal("fresh recovered Notes content", err)
	}
	if !slices.Equal(reads, []int{source.begins, source.pages, source.opens}) {
		t.Fatal("fresh recovery reimported plaintext source")
	}
	recoveredFacade, err := api.NewNotes("https://unused.invalid", nil, nil).WithPrivateData(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if pinned, e := recoveredFacade.PinnedNotes(ctx); e != nil || len(pinned) != 1 || pinned[0].ID != "note" {
		t.Fatal("fresh pinned view", e)
	}
	if tags, e := recoveredFacade.ListTags(ctx); e != nil || len(tags) != 1 || len(tags[0].States) != 1 || tags[0].States[0].ID != "high" {
		t.Fatal("fresh tag join", e)
	}
	if links, e := recoveredFacade.LinkedNotes(ctx, "note", "another", 100); e != nil || len(links) != 1 || links[0].ID != "note" {
		t.Fatal("fresh inverse links", e)
	}
	if reminders, e := recoveredFacade.ListReminders(ctx); e != nil || len(reminders) != 1 || reminders[0].NoteID != "note" {
		t.Fatal("fresh private reminder", e)
	}
	if hashtags, e := recoveredFacade.Hashtags(ctx); e != nil || len(hashtags) != 1 || hashtags[0].Tag != "private" || hashtags[0].Count != 1 {
		t.Fatal("fresh local hashtag index", e)
	}
	raw, err = recovered.ReadRaw(ctx, "note", "note")
	if err != nil || !bytes.Contains(raw, []byte(`900719925474099312345`)) || !bytes.Contains(raw, []byte(`"future":{"nested":9007199254740993}`)) {
		t.Fatal("private edit lost exact unknown fields", err)
	}
	clear(raw)
	for _, data := range gateway.data {
		if bytes.Contains(data, []byte("private")) || bytes.Contains(data, []byte("900719925474099312345")) {
			t.Fatal("opaque gateway received plaintext")
		}
	}
	for ref := range snapshot.Entries {
		stored, err := device.Matrix.GetEvent(ctx, id.RoomID(ref.RoomID), id.EventID(ref.EventID))
		if err != nil || stored.Type != event.EventEncrypted {
			t.Fatal("Synapse stored plaintext application record", err)
		}
	}
	t.Log("real SDK/Synapse: explicit exact-byte source import and final receipt, opaque large JSON blobs, lost accepted index ACK, full SDK restart and durable retry, fresh isolated-device recovery without source reimport; SQL source and UI activation remain separate")
}

func freshNotesStorageDevice(ctx context.Context, t *testing.T, base string, httpClient *http.Client, original storageSynapseDevice) *Client {
	t.Helper()
	anonymous, err := mautrix.NewClient(base, "", "")
	if err != nil {
		t.Fatal(err)
	}
	anonymous.Client = httpClient
	login, err := anonymous.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: original.Matrix.UserID.String()}, Password: original.Password, DeviceID: id.DeviceID("FRESH_" + uuid.NewString())})
	if err != nil {
		t.Fatal(err)
	}
	cfg := original.Config
	cfg.StoreDir = t.TempDir()
	cfg.Secrets = &memorySecrets{values: map[string][]byte{}}
	fresh := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: base, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: cfg.Subject, Tenant: cfg.Tenant})
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err = fresh.Recover(ctx, original.Recovery); err != nil {
		t.Fatal("fresh isolated Notes recovery", err)
	}
	return fresh
}

func privateNotesFixtureTarget(t *testing.T) *url.URL {
	t.Helper()
	fixtureURL := os.Getenv("SYTUE_MATRIX_PRIVATE_NOTES_URL")
	if fixtureURL == "" {
		fixtureURL = "http://127.0.0.1:18595"
	}
	upstreamURL, err := url.Parse(fixtureURL)
	if err != nil {
		t.Fatal("invalid disposable Notes fixture URL", err)
	}
	return upstreamURL
}

func TestPrivateNotesImportHandoffAgainstSynapseWithFreshDeviceAndEmptyJournal(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_NOTES") != "1" {
		t.Skip("requires isolated disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(privateNotesFixtureTarget(t)))
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "notes"}
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	gateway := &fileGatewayFixture{product: "notes"}
	blobServer := httptest.NewServer(gateway)
	t.Cleanup(blobServer.Close)
	blobs := api.NewDrive(blobServer.URL, nil, blobServer.Client())
	firstJournal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	book := []byte(`{"id":"book","owner_sub":"` + scope.Subject + `","title":"handoff book","visibility":"private"}`)
	body := []byte(`{"id":"note","owner_sub":"` + scope.Subject + `","notebook_id":"book","kind":"text","width":260,"title":"handoff note","padding":"` + strings.Repeat("private handoff source ", 2048) + `","future":{"integer":900719925474099312345}}`)
	source := &notesRetainedSourceFixture{state: privateNotes.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, rows: map[string][]byte{"notebook/book": book, "note/note": body}}
	open := func(crypto privateNotes.Crypto, journal *offline.Store) *privateNotes.Repository {
		t.Helper()
		r, err := privateNotes.New(privateNotes.Config{Context: ctx, Scope: scope, Crypto: crypto, Gateway: blobs, Journal: journal, Source: source})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	lost := &lostStorageIndexACK{Client: device.Client, drop: true}
	first := open(lost, firstJournal)
	if err := first.ImportLegacySnapshot(ctx); err == nil || lost.drop || !lost.accepted.Valid() {
		t.Fatal("partial accepted index ACK was not lost", err)
	}
	first.Close()
	if source.begins != 1 || source.opens != 1 || gateway.puts != 1 {
		t.Fatal("partial import fixture did not stop after one body", source.begins, source.opens, gateway.puts)
	}
	if err := device.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	fresh := freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), device)
	emptyJournal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	if count, err := emptyJournal.Count("notes-private-v1"); err != nil || count != 0 {
		t.Fatal("fresh journal was not empty", count, err)
	}
	second := open(fresh, emptyJournal)
	defer second.Close()
	if err := second.Initialize(ctx); !errors.Is(err, privateNotes.ErrNotReady) {
		t.Fatal("partial catalog activated", err)
	}
	if source.opens != 1 {
		t.Fatal("ordinary fresh initialization read SQL")
	}
	if err := second.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal("explicit fresh-device partial resume failed", err)
	}
	if source.begins != 1 || source.opens != 2 || gateway.puts != 1 {
		t.Fatal("handoff reimported or uploaded accepted seed", source.begins, source.opens, gateway.puts)
	}
	recovered, err := second.ReadRaw(ctx, "note", "note")
	if err != nil || !bytes.Equal(recovered, body) {
		t.Fatal("handoff changed exact seed body", err)
	}
	clear(recovered)
	catalog, err := storage.LoadCatalog(ctx, fresh, scope)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, entry := range catalog.Entries {
		if entry.Record.ObjectID == "notes_"+digestBytes([]byte("note\x00note")) {
			found++
			if entry.Reference != lost.accepted {
				t.Fatal("accepted seed was replaced")
			}
		}
	}
	if found != 1 {
		t.Fatal("handoff duplicated seed branches", found)
	}
	if _, err := second.ListNotes(ctx, "book"); err != nil {
		t.Fatal("completed graph unavailable", err)
	}
	// Reopen the first device's encrypted WAL using the recovered SDK. Its old
	// publication target cannot be reused for new writes; completion authenticates
	// only the exact accepted index and retires that file receipt without SQL.
	original := open(fresh, firstJournal)
	defer original.Close()
	beforeOpen, beforePut := source.opens, gateway.puts
	if err := original.Initialize(ctx); err != nil {
		t.Fatal("original WAL reconciliation after handoff", err)
	}
	pending, err := original.Pending(ctx)
	if err != nil || pending.Count != 0 {
		t.Fatal("confirmed original WAL remained pending", pending, err)
	}
	if source.opens != beforeOpen || gateway.puts != beforePut {
		t.Fatal("original WAL reconciliation republished or reimported")
	}
	t.Log("real SDK/Synapse: fresh-device handoff preserved the accepted seed; the original encrypted WAL then authenticated completion and its exact file/index, retired its SDK upload receipt, and reached zero pending without SQL reads or new ciphertext")
}
