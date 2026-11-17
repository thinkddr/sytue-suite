// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	matrixid "maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privatecontacts "git.sytue.eu/sytue/one/apps/nucleo/contacts"
	contactsweb "git.sytue.eu/sytue/one/apps/nucleo/contacts/web"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type contactsPGSource struct {
	Snapshot     records.SnapshotState `json:"state"`
	Rows         []privatecontacts.Row `json:"rows"`
	pages, opens int
	retired      bool
}

func (s *contactsPGSource) State(_ context.Context, scope storage.Scope) (records.SnapshotState, error) {
	if s.Snapshot.Validate(scope) != nil || s.Snapshot.Version != 1 {
		return records.SnapshotState{}, privatecontacts.ErrSnapshot
	}
	return s.Snapshot, nil
}

func (*contactsPGSource) Begin(context.Context, storage.Scope, string) (records.SnapshotState, error) {
	return records.SnapshotState{}, errors.New("fixture source is already frozen")
}

func (s *contactsPGSource) Page(_ context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (records.SnapshotPage, error) {
	s.pages++
	result := records.SnapshotPage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{}}
	if s.retired || scope != s.Snapshot.Scope || epoch != s.Snapshot.Epoch {
		return result, errors.New("unexpected retained source page")
	}
	for _, row := range s.Rows {
		if row.Kind < after.Kind || row.Kind == after.Kind && row.ID <= after.ID {
			continue
		}
		if len(result.Rows) == 2 {
			last := result.Rows[1]
			result.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
			break
		}
		sum := sha256.Sum256(row.JSON)
		result.Rows = append(result.Rows, storage.StreamSourceRow{Kind: row.Kind, ID: row.ID, Size: int64(len(row.JSON)), SHA256: hex.EncodeToString(sum[:])})
	}
	return result, result.Validate(scope, epoch, after)
}

func (s *contactsPGSource) Open(_ context.Context, scope storage.Scope, epoch string, item records.SnapshotRow) (io.ReadCloser, error) {
	s.opens++
	if s.retired || scope != s.Snapshot.Scope || epoch != s.Snapshot.Epoch {
		return nil, errors.New("unexpected retained source body")
	}
	for _, row := range s.Rows {
		if row.Kind == item.Kind && row.ID == item.ID {
			return io.NopCloser(bytes.NewReader(row.JSON)), nil
		}
	}
	return nil, privatecontacts.ErrSnapshot
}

func TestPrivateContactsAgainstSynapseResumesAndRecoversExactPostgresSource(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_CONTACTS") != "1" {
		t.Skip("requires disposable remote Synapse and synthetic PostgreSQL export")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	raw, err := os.ReadFile(os.Getenv("SYTUE_CONTACTS_PRIVATE_VECTOR"))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	var source contactsPGSource
	if json.Unmarshal(raw, &source) != nil || len(source.Rows) != 9 || source.Snapshot.Scope.Product != "contacts" {
		t.Fatal("invalid synthetic retained source")
	}
	slices.SortFunc(source.Rows, func(a, b privatecontacts.Row) int {
		if n := strings.Compare(a.Kind, b.Kind); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	address := os.Getenv("SYTUE_CONTACTS_SYNAPSE_URL")
	if address == "" {
		address = "http://127.0.0.1:18601"
	}
	target, err := url.Parse(address)
	if err != nil || target.Hostname() != "127.0.0.1" {
		t.Fatal("fixture must stay on remote loopback")
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), source.Snapshot.Scope, storage.IsolatedIdentity)
	client := device.Client
	defer func() { _ = client.Close() }()
	files, gateway, _, _ := fileFixture(t)
	gateway.product = "contacts"
	faults := &lostStorageIndexACK{Client: client, drop: true}
	open := func(journal *offline.Store, crypto records.Crypto) *privatecontacts.Repository {
		repository, err := privatecontacts.New(privatecontacts.Config{Context: ctx, Scope: source.Snapshot.Scope, Crypto: crypto, Gateway: files.gateway, Journal: journal, Source: &source})
		if err != nil {
			t.Fatal(err)
		}
		return repository
	}
	repository := open(files.journal, faults)
	if err := repository.Initialize(ctx); !errors.Is(err, records.ErrNotReady) {
		t.Fatal("unimported source became ready", err)
	}
	if source.pages != 0 || source.opens != 0 {
		t.Fatal("initialize imported plaintext automatically")
	}
	if err := repository.ImportLegacySnapshot(ctx); err == nil || faults.accepted == (storage.Reference{}) {
		t.Fatal("actual lost index ACK did not interrupt import", err)
	}
	if snapshot, err := repository.Snapshot(ctx); err == nil || snapshot != nil {
		t.Fatal("partial import became visible")
	}
	repository.Close()
	repository = open(files.journal, faults)
	if err := repository.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal("durable retained import did not resume", err)
	}
	checkContactsPGSnapshot(ctx, t, repository, source.Rows)
	history := artificialCatalogHistory(ctx, t, client, device.Room.String())
	heads, err := storage.CatalogHeads(source.Snapshot.Scope, history)
	if err != nil || len(heads) != 10 {
		t.Fatal("lost ACK duplicated a published source index", len(heads), err)
	}
	for _, entry := range history {
		stored, err := device.Matrix.GetEvent(ctx, device.Room, matrixid.EventID(entry.Reference.EventID))
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(stored)
		if err != nil || stored.Type != event.EventEncrypted || bytes.Contains(wire, []byte("artificial private Contacts source marker")) || bytes.Contains(wire, []byte("ada@sytue.eu")) {
			t.Fatal("private contact data reached Synapse", err)
		}
	}
	checkOpaqueContactsGateway(t, gateway)
	if err := client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	repository.Close()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	login, err := device.Matrix.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: device.Matrix.UserID.String()}, Password: device.Password, DeviceID: "CONTACTS_FRESH_DEVICE"})
	if err != nil {
		t.Fatal(err)
	}
	config := device.Config
	config.StoreDir = t.TempDir()
	config.Secrets = &memorySecrets{values: map[string][]byte{}}
	second := storageSynapseSession(ctx, t, config, credentials{Homeserver: proxy.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: config.Subject, Tenant: config.Tenant})
	defer func() { _ = second.Close() }()
	go func() { _ = second.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := second.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	if count, err := journal.Count("contacts-private-v1"); err != nil || count != 0 {
		t.Fatal("fresh journal was not empty", count, err)
	}
	fresh := open(journal, second)
	defer fresh.Close()
	if err := fresh.Initialize(ctx); err == nil {
		t.Fatal("token-only device opened private contacts")
	}
	if err := second.Recover(ctx, device.Recovery); err != nil {
		t.Fatal("fresh recovery failed", err)
	}
	pages, opens := source.pages, source.opens
	source.retired = true
	if err := fresh.Initialize(ctx); err != nil {
		t.Fatal("empty-journal fresh device did not recover private contacts", err)
	}
	checkContactsPGSnapshot(ctx, t, fresh, source.Rows)
	exercisePrivateContactsCRUD(ctx, t, fresh)
	exerciseContactsWebAgainstRealRepository(ctx, t, fresh, second, files.gateway, &source)
	checkWebContactsCiphertext(ctx, t, second, device.Room, gateway)
	if source.pages != pages || source.opens != opens {
		t.Fatal("fresh device reimported retained plaintext")
	}
	t.Logf("real Contacts: %d retained rows, %d private heads, lost ACK, opaque large content and empty-journal recovered device verified", len(source.Rows), len(heads))
}

func checkContactsPGSnapshot(ctx context.Context, t *testing.T, repository *privatecontacts.Repository, want []privatecontacts.Row) {
	t.Helper()
	snapshot, err := repository.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	actual := snapshot.Rows()
	if len(actual) != len(want) {
		t.Fatal("private source inventory changed")
	}
	originals := map[string][]byte{}
	for _, row := range want {
		originals[row.Kind+"/"+row.ID] = row.JSON
	}
	for _, row := range actual {
		if !bytes.Equal(row.JSON, originals[row.Kind+"/"+row.ID]) {
			t.Fatal("private retained row changed", row.Kind)
		}
	}
	people, err := snapshot.Contacts()
	if err != nil || len(people) != 1 || people[0].DeletedAt == nil || !people[0].Favorite || !people[0].HasPhoto || !strings.Contains(people[0].Note, "artificial private Contacts source marker") {
		t.Fatal("private photo/trash/favorite/note changed", err)
	}
	groups, err := snapshot.Groups()
	if err != nil || len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatal("private group membership changed", err)
	}
}

func checkOpaqueContactsGateway(t *testing.T, gateway *fileGatewayFixture) {
	t.Helper()
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.data) == 0 {
		t.Fatal("large vCard/contact rows did not use encrypted file publication")
	}
	for _, ciphertext := range gateway.data {
		if bytes.Contains(ciphertext, []byte("artificial private Contacts source marker")) || bytes.Contains(ciphertext, []byte("PHOTO:data:")) {
			t.Fatal("object provider received contact plaintext")
		}
	}
}

type contactsNoLegacyTransport struct{ calls atomic.Int32 }

func (t *contactsNoLegacyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return nil, errors.New("private Contacts attempted legacy HTTP")
}

func exercisePrivateContactsCRUD(ctx context.Context, t *testing.T, repository *privatecontacts.Repository) {
	t.Helper()
	transport := &contactsNoLegacyTransport{}
	original := api.NewContacts("https://legacy.invalid", nil, &http.Client{Transport: transport})
	client, err := original.WithPrivateData(repository)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := snapshot.Contacts()
	if err != nil || len(imported) != 1 {
		t.Fatal(err)
	}
	id := imported[0].ID
	if err := client.Restore(ctx, id); err != nil {
		t.Fatal("restore private source", err)
	}
	if _, err := client.Contact(ctx, id); err != nil {
		t.Fatal(err)
	}
	books, err := client.AddressBooks(ctx)
	if err != nil || len(books) == 0 {
		t.Fatal(err)
	}
	work, err := client.CreateAddressBook(ctx, "Private work")
	if err != nil {
		t.Fatal(err)
	}
	exact, err := client.CreateAddressBookAt(ctx, "private-exact", "Exact", "#112233")
	if err != nil {
		t.Fatal(err)
	}
	work, err = client.RenameAddressBook(ctx, work.ID, "Renamed work")
	if err != nil || work.DisplayName != "Renamed work" {
		t.Fatal(err)
	}
	person, err := client.Create(ctx, work.ID, api.ContactInput{FormattedName: "Private created person", Birthday: "2000-02-29", Emails: []api.ContactEmail{{Email: "created@example.invalid", Type: "work", IsPrimary: true}}, Note: "Private Contacts CRUD marker"})
	if err != nil {
		t.Fatal(err)
	}
	input := person.Input()
	input.Note = "Private edited person"
	person, err = client.Update(ctx, person.ID, input, person.ETag)
	if err != nil || person.Note != "Private edited person" {
		t.Fatal(err)
	}
	raw, err := client.VCard(ctx, person.ID)
	if err != nil || !bytes.Contains(raw, []byte("Private edited person")) {
		t.Fatal("private vCard changed", err)
	}
	other, err := client.ImportCard(ctx, work.ID, "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:external-person\r\nFN:External person\r\nEMAIL:external@example.invalid\r\nEND:VCARD\r\n")
	if err != nil {
		t.Fatal(err)
	}
	results, err := client.Search(ctx, "private created", 50)
	if err != nil || len(results) != 1 {
		t.Fatal("private search", err)
	}
	all, _, err := client.All(ctx, 100, 0)
	if err != nil || len(all) != 3 {
		t.Fatal("private all", len(all), err)
	}
	if rows, err := client.InBook(ctx, work.ID); err != nil || len(rows) != 2 {
		t.Fatal("private complete book", len(rows), err)
	}
	if err := client.SetPhoto(ctx, person.UID, "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg=="); err != nil {
		t.Fatal(err)
	}
	if photo, err := client.Photo(ctx, person.UID); err != nil || len(photo) == 0 {
		t.Fatal("private photo", err)
	}
	if err := client.RemovePhoto(ctx, person.UID); err != nil {
		t.Fatal(err)
	}
	if err := client.SetFavorite(ctx, person.UID, true); err != nil {
		t.Fatal(err)
	}
	if err := client.BatchFavorite(ctx, []string{person.UID, other.UID}, true); err != nil {
		t.Fatal(err)
	}
	if favorites, err := client.Favorites(ctx); err != nil || len(favorites) < 2 {
		t.Fatal("private favorites", err)
	}
	group, err := client.CreateGroup(ctx, work.ID, "Private group")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.BatchGroupMembers(ctx, group.ID, []string{person.UID, other.UID, "unresolved"}, true); err != nil {
		t.Fatal(err)
	}
	groups, err := client.Groups(ctx, work.ID)
	if err != nil || len(groups) != 1 || len(groups[0].Members) != 3 {
		t.Fatal("private groups", err)
	}
	group, err = client.RenameGroup(ctx, group.ID, "Renamed group", groups[0].Members)
	if err != nil || group.Name != "Renamed group" {
		t.Fatal(err)
	}
	if err := client.BatchGroupMembers(ctx, group.ID, []string{other.UID}, false); err != nil {
		t.Fatal(err)
	}
	count, skipped, err := client.ImportCSV(ctx, exact.ID, []byte("Name,E-mail 1 - Value\nCSV private,csv@example.invalid\nCSV duplicate,csv@example.invalid\n"))
	if err != nil || count != 1 || skipped != 1 {
		t.Fatal("private CSV import", count, skipped, err)
	}
	if name, raw, err := client.ExportCSV(ctx); err != nil || name == "" || !bytes.Contains(raw, []byte("csv@example.invalid")) {
		t.Fatal("private CSV export", err)
	}
	if n, err := client.BatchTrash(ctx, []string{person.UID, other.UID}); err != nil || n != 2 {
		t.Fatal(err)
	}
	trash, err := client.Trash(ctx, 200)
	if err != nil || len(trash) != 2 {
		t.Fatal("private batch trash", len(trash), err)
	}
	if err := client.Restore(ctx, person.ID); err != nil {
		t.Fatal(err)
	}
	if err := client.Delete(ctx, person.ID); err != nil {
		t.Fatal(err)
	}
	if err := client.Purge(ctx, person.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Contact(ctx, person.ID); err == nil {
		t.Fatal("purged private contact is visible")
	}
	groups, err = client.Groups(ctx, work.ID)
	if err != nil || len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatal("purge lost unresolved membership", err)
	}
	if err := client.DeleteGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteAddressBook(ctx, exact.ID); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteAddressBook(ctx, work.ID); err != nil {
		t.Fatal(err)
	}
	if transport.calls.Load() != 0 {
		t.Fatal("private operation fell back to legacy HTTP")
	}
}

// The adapter is exercised against the real recovered repository and SDK, not
// a ContactsData stub. This deliberately does not implement the future web
// session, its WorkGate, decoder, delivery acknowledgment, or browser UI.
func exerciseContactsWebAgainstRealRepository(ctx context.Context, t *testing.T, repository *privatecontacts.Repository, client *Client, gateway *api.Drive, source *contactsPGSource) {
	t.Helper()
	dispatcher, err := contactsweb.New(repository)
	if err != nil {
		t.Fatal(err)
	}
	call := func(command contactsweb.Command) contactsweb.Result {
		t.Helper()
		var result contactsweb.Result
		var borrowed []byte
		err := dispatcher.Perform(ctx, command, func(value contactsweb.Result) error {
			result.Value = value.Value
			borrowed = value.Body
			result.Body = bytes.Clone(value.Body)
			return nil
		})
		if err != nil {
			t.Fatalf("private Contacts web action %T: %v", command, err)
		}
		if !bytes.Equal(borrowed, make([]byte, len(borrowed))) {
			t.Fatalf("private Contacts web action %T retained plaintext buffer", command)
		}
		return result
	}
	created := call(contactsweb.CreateBookAt{Name: "web-integration", Display: "Web integration", Color: "#112233"})
	book, ok := created.Value.(api.AddressBook)
	if !ok || book.ID == "" {
		t.Fatal("typed book result missing")
	}
	created = call(contactsweb.CreateContact{Book: book.ID, Input: api.ContactInput{FormattedName: "Web private person", Note: "web-private-contact-marker", Emails: []api.ContactEmail{{Email: "web-private@example.invalid", IsPrimary: true}}}})
	person, ok := created.Value.(api.Contact)
	if !ok || person.ID == "" || person.ETag == "" {
		t.Fatal("typed contact result missing identity or revision")
	}
	input := person.Input()
	input.Note = "web-private-edited-marker"
	edited, ok := call(contactsweb.UpdateContact{ID: person.ID, Input: input, ETag: person.ETag}).Value.(api.Contact)
	if !ok || edited.Note != input.Note || edited.ETag == person.ETag {
		t.Fatal("web update did not retain private input and revision")
	}
	photoURI := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg=="
	call(contactsweb.SetPhoto{UID: edited.UID, DataURI: photoURI})
	photo := call(contactsweb.GetPhoto{UID: edited.UID}).Body
	if len(photo) < 8 || !bytes.Equal(photo[:8], []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("private photo differs")
	}
	clear(photo)
	card := call(contactsweb.ExportCard{ID: edited.ID}).Body
	if !bytes.Contains(card, []byte("web-private-edited-marker")) || !bytes.Contains(card, []byte("PHOTO")) {
		t.Fatal("private vCard lost edit or photo")
	}
	clear(card)
	imported := call(contactsweb.ImportCard{Book: book.ID, Card: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:external-web\r\nFN:Imported web person\r\nX-KEEP:private-web-property\r\nEND:VCARD\r\n"})
	other, ok := imported.Value.(api.Contact)
	if !ok || other.ID == "" {
		t.Fatal("private vCard import missing")
	}
	card = call(contactsweb.ExportCard{ID: other.ID}).Body
	if !bytes.Contains(card, []byte("X-KEEP:private-web-property")) {
		t.Fatal("private vCard import dropped unknown property")
	}
	clear(card)
	csvInput := []byte("Name,E-mail 1 - Value\nWeb CSV person,web-csv@example.invalid\n")
	count, ok := call(contactsweb.ImportCSV{Book: book.ID, Data: csvInput}).Value.(contactsweb.ImportCount)
	clear(csvInput) // host regains and clears its exclusive input after Perform returns
	if !ok || count.Imported != 1 || count.Skipped != 0 {
		t.Fatal("private CSV import changed")
	}
	csv := call(contactsweb.ExportCSV{}).Body
	if !bytes.Contains(csv, []byte("web-csv@example.invalid")) {
		t.Fatal("private CSV export lost contact")
	}
	clear(csv)
	call(contactsweb.DeleteContact{ID: edited.ID})
	if _, err := repository.Contact(ctx, edited.ID); err == nil {
		t.Fatal("private delete did not hide contact")
	}
	call(contactsweb.Restore{ID: edited.ID})
	if value, err := repository.Contact(ctx, edited.ID); err != nil || value.ID != edited.ID {
		t.Fatal("private restore lost contact", err)
	}

	// The write has committed before serialization/delivery can fail. Do not
	// retry it: there is no durable caller intent receipt yet.
	lost := errors.New("synthetic host response loss")
	var acceptedID string
	err = dispatcher.Perform(ctx, contactsweb.CreateContact{Book: book.ID, Input: api.ContactInput{FormattedName: "Lost web response"}}, func(value contactsweb.Result) error {
		accepted, ok := value.Value.(api.Contact)
		if !ok || accepted.ID == "" {
			t.Fatal("missing accepted contact")
		}
		acceptedID = accepted.ID // test witness only; the failed host receives no acknowledgement
		return lost
	})
	if !errors.Is(err, lost) || acceptedID == "" {
		t.Fatal("synthetic lost response was not surfaced", err)
	}
	if value, err := repository.Contact(ctx, acceptedID); err != nil || value.FormattedName != "Lost web response" {
		t.Fatal("failed delivery did not leave the accepted write visible", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	called := false
	err = dispatcher.Perform(cancelled, contactsweb.ListBooks{}, func(contactsweb.Result) error { called = true; return nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatal("canceled session exposed a result", err)
	}

	// Cancel after the real SDK has received an index receipt, before the
	// repository can deliver its result. This does not authorize a retry.
	inflightJournal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	intercept := &cancelContactsWebAfterIndex{Crypto: client}
	inflightRepo, err := privatecontacts.New(privatecontacts.Config{Context: ctx, Scope: source.Snapshot.Scope, Crypto: intercept, Gateway: gateway, Journal: inflightJournal, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	defer inflightRepo.Close()
	if err := inflightRepo.Initialize(ctx); err != nil {
		t.Fatal("real recovered repository for canceled write", err)
	}
	inflightDispatcher, err := contactsweb.New(inflightRepo)
	if err != nil {
		t.Fatal(err)
	}
	inflight, stop := context.WithCancel(ctx)
	intercept.stop = stop
	defer stop()
	called = false
	err = inflightDispatcher.Perform(inflight, contactsweb.CreateContact{Book: book.ID, Input: api.ContactInput{FormattedName: "Canceled after index accepted"}}, func(contactsweb.Result) error { called = true; return nil })
	if !errors.Is(err, context.Canceled) || called || !intercept.accepted.Valid() {
		t.Fatal("accepted index cancellation exposed a result or did not reach Synapse", err)
	}
	stored, err := client.cli.GetEvent(ctx, matrixid.RoomID(intercept.accepted.RoomID), matrixid.EventID(intercept.accepted.EventID))
	if err != nil || stored.Type != event.EventEncrypted {
		t.Fatal("canceled action did not leave its accepted index encrypted", err)
	}

	// Rebinding the real SDK to a different owner or tenant must not initialize
	// a repository or expose any of the previous scope through the dispatcher.
	for _, change := range []string{"subject", "tenant"} {
		scope := source.Snapshot.Scope
		if change == "subject" {
			scope.Subject += "-other"
		} else {
			scope.Tenant += "-other"
		}
		journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
		if err != nil {
			t.Fatal(err)
		}
		foreign, err := privatecontacts.New(privatecontacts.Config{Context: ctx, Scope: scope, Crypto: client, Gateway: gateway, Journal: journal, Source: source})
		if err != nil {
			t.Fatal(err)
		}
		if err := foreign.Initialize(ctx); err == nil {
			foreign.Close()
			t.Fatalf("SDK accepted a foreign %s", change)
		}
		foreignDispatcher, err := contactsweb.New(foreign)
		if err != nil {
			foreign.Close()
			t.Fatal(err)
		}
		called = false
		err = foreignDispatcher.Perform(ctx, contactsweb.ListBooks{}, func(contactsweb.Result) error { called = true; return nil })
		foreign.Close()
		if err == nil || called {
			t.Fatalf("foreign %s yielded Contacts data", change)
		}
	}
}

// Verify new dispatcher writes are encrypted at the homeserver and file gateway.
func checkWebContactsCiphertext(ctx context.Context, t *testing.T, client *Client, room matrixid.RoomID, gateway *fileGatewayFixture) {
	t.Helper()
	markers := [][]byte{[]byte("web-private-edited-marker"), []byte("web-csv@example.invalid"), []byte("private-web-property")}
	history := artificialCatalogHistory(ctx, t, client, room.String())
	if len(history) == 0 {
		t.Fatal("missing Contacts catalog events")
	}
	seen := map[storage.Reference]bool{}
	inspect := func(ref storage.Reference) {
		t.Helper()
		if seen[ref] {
			return
		}
		seen[ref] = true
		stored, err := client.cli.GetEvent(ctx, matrixid.RoomID(ref.RoomID), matrixid.EventID(ref.EventID))
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(stored)
		if err != nil || stored.Type != event.EventEncrypted {
			t.Fatal("Contacts record was not encrypted", err)
		}
		for _, marker := range markers {
			if bytes.Contains(wire, marker) {
				t.Fatal("Contacts web marker reached Synapse")
			}
		}
	}
	for _, entry := range history {
		inspect(entry.Reference)
		version, err := storage.DecodeObject(entry, client.storageScope())
		if err != nil {
			t.Fatal("invalid Contacts catalog entry", err)
		}
		if version.Commit.Content != nil {
			inspect(version.Commit.Content.Reference)
		}
	}
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	for _, encrypted := range gateway.data {
		for _, marker := range markers {
			if bytes.Contains(encrypted, marker) {
				t.Fatal("Contacts web marker reached object gateway")
			}
		}
	}
}

// Delegates to the maintained SDK; only the test context is canceled after the
// actual remote index receipt. No verification flags or crypto are simulated.
type cancelContactsWebAfterIndex struct {
	records.Crypto
	stop     context.CancelFunc
	accepted storage.Reference
}

func (c *cancelContactsWebAfterIndex) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, txn, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	version, err := c.Crypto.PutStorageObjectAt(ctx, target, txn, object, revision, commit)
	if err == nil && c.stop != nil {
		c.accepted = version.Reference
		c.stop()
		c.stop = nil
	}
	return version, err
}
