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
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
)

// Only synthetic historical data from the actual PostgreSQL test is served.
// This fixture deliberately does not claim a real continuing SMTP/IMAP bridge.
type mailPGSource struct {
	storagetest.MailSnapshot
	privateSettingsEdited             bool
	authorization                     string
	pages, opens, requests            atomic.Int32
	forbidPages, retired, unavailable atomic.Bool
}

func (s *mailPGSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	if r.Header.Get("Authorization") != s.authorization {
		http.Error(w, "fixture authorization required", http.StatusUnauthorized)
		return
	}
	if s.unavailable.Load() {
		http.NotFound(w, r)
		return
	}
	if s.retired.Load() || r.Method != http.MethodGet || storage.ValidateRequestScope(r.Header, s.State.Scope) != nil {
		http.Error(w, "source unavailable", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	if path == "/private-storage/migration" {
		_ = json.NewEncoder(w).Encode(s.State)
		return
	}
	base := "/private-storage/migration/" + s.State.Epoch
	if path == base+"/rows" {
		s.page(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, base+"/source/"), "/")
	if len(parts) == 2 && strings.HasPrefix(path, base+"/source/") {
		for _, row := range s.Rows {
			if row.Descriptor.Kind != parts[0] || row.Descriptor.ID != parts[1] {
				continue
			}
			s.opens.Add(1)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", strconv.Itoa(len(row.Body)))
			w.Header().Set("X-Sytue-Source-SHA256", row.Descriptor.SHA256)
			_, _ = w.Write(row.Body)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *mailPGSource) page(w http.ResponseWriter, r *http.Request) {
	s.pages.Add(1)
	if s.forbidPages.Load() {
		http.Error(w, "SQL pages must not be repeated", http.StatusConflict)
		return
	}
	after, err := storage.DecodeSourceCursor(r.URL.Query().Get("after"))
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}
	page, err := s.Page(after, 2)
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(page)
}

func loadMailPGSource(t *testing.T) *mailPGSource {
	t.Helper()
	raw, err := os.ReadFile(os.Getenv("SYTUE_MAIL_PRIVATE_VECTOR"))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(raw)
	source, err := storagetest.DecodeMailSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	return &mailPGSource{MailSnapshot: source}
}

func TestPrivateMailImportsActualPostgresSourceAndRecoversAgainstSynapse(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_MAIL_SOURCE") != "1" {
		t.Skip("requires synthetic actual PostgreSQL export and disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	source := loadMailPGSource(t)
	target, err := url.Parse(os.Getenv("SYTUE_MAIL_SYNAPSE_URL"))
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" {
		t.Fatal("Synapse fixture must stay on remote loopback")
	}
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), source.State.Scope, storage.IsolatedIdentity)
	client := device.Client
	defer func() { _ = client.Close() }()
	source.authorization = "Bearer synthetic-mail-source-fixture"
	server := httptest.NewTLSServer(source)
	defer server.Close()
	session := auth.NewSession(auth.Tokens{Issuer: "https://synthetic-source.example.test", Cliente: "synthetic-mail", Subject: source.State.Scope.Subject, Organizacion: source.State.Scope.Tenant, Access: "synthetic-mail-source-fixture", Expiry: time.Now().Add(time.Hour)}, nil, nil)
	legacy := api.NewMail(server.URL, session, server.Client())
	gateway := &fileGatewayFixture{product: "mail"}
	blobServer := httptest.NewServer(gateway)
	defer blobServer.Close()
	blobs := api.NewDrive(blobServer.URL, nil, blobServer.Client())
	journalDir := t.TempDir()
	secrets := &authtest.Secrets{}
	journal, err := offline.OpenWithSecrets(journalDir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	faults := &lostStorageIndexACK{Client: client, drop: true}
	open := func(crypto privateMail.Crypto, wal *offline.Store) *privateMail.Repository {
		r, err := privateMail.New(privateMail.Config{Context: ctx, Scope: source.State.Scope, Crypto: crypto, Gateway: blobs, Journal: wal, MigrationState: source.State.LegacyMigrationState, MaxMessageBytes: source.State.MaxBodyBytes})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	repository := open(faults, journal)
	defer func() { repository.Close() }()
	if err := repository.Initialize(ctx); !errors.Is(err, privateMail.ErrNotReady) {
		t.Fatal("empty account became ready", err)
	}
	if source.pages.Load() != 0 || source.opens.Load() != 0 {
		t.Fatal("Initialize silently imported legacy SQL")
	}
	source.unavailable.Store(true)
	if err := repository.ImportRetainedSource(ctx, legacy); err == nil {
		t.Fatal("unavailable HTTP source became ready")
	}
	if gateway.puts != 0 || faults.accepted != (storage.Reference{}) || source.pages.Load() != 0 || source.opens.Load() != 0 {
		t.Fatal("HTTP404 source admitted effects or legacy fallback")
	}
	if _, err := repository.Mailboxes(ctx); !errors.Is(err, privateMail.ErrNotReady) {
		t.Fatal("HTTP404 source became visible", err)
	}
	source.unavailable.Store(false)
	if err := repository.ImportRetainedSource(ctx, legacy); err == nil || faults.accepted == (storage.Reference{}) {
		t.Fatal("actual accepted index ACK was not lost", err)
	}
	if _, err := repository.Mailboxes(ctx); !errors.Is(err, privateMail.ErrNotReady) {
		t.Fatal("partial source became visible", err)
	}
	pages := source.pages.Load()
	source.forbidPages.Store(true)
	repository.Close()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	client, err = Open(ctx, device.Config)
	if err != nil {
		t.Fatal("native SDK did not reopen its retained keys", err)
	}
	go func() { _ = client.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	journal, err = offline.OpenWithSecrets(journalDir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	faults.Client = client
	repository = open(faults, journal)
	if err := repository.ImportRetainedSource(ctx, legacy); err != nil {
		t.Fatal("actual SDK/WAL import did not resume", err)
	}
	if source.pages.Load() != pages {
		t.Fatal("retry reimported SQL pages")
	}
	verifyPrivateMailPGContents(ctx, t, repository, client, blobs, source)
	puts := gateway.puts
	if err := repository.ImportRetainedSource(ctx, legacy); err != nil || gateway.puts != puts {
		t.Fatal("completed import uploaded again", err)
	}
	exercisePrivateMailSettings(ctx, t, repository)
	source.privateSettingsEdited = true
	verifyPrivateMailPGContents(ctx, t, repository, client, blobs, source)
	if err := client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	source.retired.Store(true)
	requests := source.requests.Load()
	fresh := freshMailStorageDevice(ctx, t, proxy.URL, proxy.Client(), device)
	defer func() { _ = fresh.Close() }()
	freshJournal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	recovered := open(fresh, freshJournal)
	defer recovered.Close()
	if err := recovered.Initialize(ctx); err != nil {
		t.Fatal("fresh owner could not authenticate full historical receipt", err)
	}
	verifyPrivateMailPGContents(ctx, t, recovered, fresh, blobs, source)
	if source.requests.Load() != requests {
		t.Fatal("fresh recovery requested a retained plaintext source")
	}
	for _, raw := range gateway.data {
		if bytes.Contains(raw, []byte("SYNTHETIC_PRIVATE")) {
			t.Fatal("opaque blob gateway received plaintext")
		}
	}
	snapshot, err := storage.LoadCatalog(ctx, fresh, source.State.Scope)
	if err != nil {
		t.Fatal(err)
	}
	for ref := range snapshot.Entries {
		stored, err := device.Matrix.GetEvent(ctx, id.RoomID(ref.RoomID), id.EventID(ref.EventID))
		if err != nil || stored.Type != event.EventEncrypted {
			t.Fatal("Synapse stored a readable application event", err)
		}
	}
	t.Log("actual PostgreSQL/RLS/fence → authenticated HTTP Source → real maintained SDK/Synapse → lost accepted ACK + complete SDK/WAL restart → fresh recovery with empty journal; exact JSON and all live/deleted/queued MIME retained; continuing SMTP/IMAP ingress and original retirement are separate")
}

func verifyPrivateMailPGContents(ctx context.Context, t *testing.T, repo *privateMail.Repository, client *Client, blobs *api.Drive, source *mailPGSource) {
	t.Helper()
	facade, err := api.NewMail("https://closed-legacy.invalid", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	verifyPrivateMailSettings(ctx, t, facade, source)
	messages, _, err := facade.Messages(ctx, source.InboxID, "", 50)
	if err != nil || len(messages) != 1 || messages[0].ID != source.LiveID || messages[0].Subject != "SYNTHETIC_PRIVATE_LIVE" {
		t.Fatal("live/trash projection changed", messages, err)
	}
	if signature, err := facade.Signature(ctx); err != nil || signature != source.Signature {
		t.Fatal("large private signature changed", err)
	}
	labels, err := facade.Labels(ctx)
	if err != nil || len(labels) != 1 || labels[0].ID != source.LabelID {
		t.Fatal("private label metadata changed", labels, err)
	}
	reader, err := facade.Raw(ctx, source.LiveID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range source.Rows {
		if row.Descriptor.Kind == "mime_message" && row.Descriptor.ID == source.LiveID && !bytes.Equal(body, row.Body) {
			t.Fatal("private MIME projection changed")
		}
	}
	clear(body)
	snapshot, err := storage.LoadCatalog(ctx, client, source.State.Scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range source.Rows {
		typ, key := "source", digestBytes([]byte(row.Descriptor.Kind+"\x00"+row.Descriptor.ID))
		if row.Descriptor.Kind == "mime_message" {
			typ, key = "mime", row.Descriptor.ID
		}
		object := mailFixtureObject(typ, key)
		var version storage.ObjectVersion
		for _, head := range snapshot.Heads {
			if head.ObjectID == object {
				version = head
				break
			}
		}
		if !version.Reference.Valid() || version.Commit.Content == nil || version.Commit.Content.Kind != "file" {
			t.Fatal("retained source omitted", row.Descriptor.Kind, row.Descriptor.ID)
		}
		pointer := version.Commit.Content
		var exact bytes.Buffer
		info, err := client.DownloadStorageFile(ctx, blobs, pointer.Reference, object, pointer.RevisionID, &exact)
		if err != nil || info.SizeBytes != row.Descriptor.Size || !bytes.Equal(exact.Bytes(), row.Body) {
			t.Fatal("exact PostgreSQL archive changed", row.Descriptor.Kind, err)
		}
		clear(exact.Bytes())
	}
}

// The source rows remain the untouched PostgreSQL originals. Only the expected
// operational projection changes before backup; fresh recovery reads those
// changes through the same API facade with the legacy source unavailable.
func verifyPrivateMailSettings(ctx context.Context, t *testing.T, facade *api.Mail, source *mailPGSource) {
	t.Helper()
	var expectedRules []api.MailFilter
	var expectedVacation api.MailVacation
	for _, row := range source.Rows {
		switch row.Descriptor.Kind {
		case "filter":
			var value api.MailFilter
			if err := json.Unmarshal(row.Body, &value); err != nil {
				t.Fatal(err)
			}
			if source.privateSettingsEdited {
				value.Name += " (private edit)"
			}
			expectedRules = append(expectedRules, value)
		case "vacation":
			if err := json.Unmarshal(row.Body, &expectedVacation); err != nil {
				t.Fatal(err)
			}
			if source.privateSettingsEdited {
				expectedVacation.Body += " (private edit)"
			}
		}
	}
	rules, err := facade.Filters(ctx)
	if err != nil || len(rules) != len(expectedRules) {
		t.Fatal("private PostgreSQL rules missing", rules, err)
	}
	for i, rule := range rules {
		if rule.PrivateRevision == "" {
			t.Fatal("private rule omitted concurrency revision")
		}
		rule.PrivateRevision = ""
		actual, _ := json.Marshal(rule)
		expected, _ := json.Marshal(expectedRules[i])
		if !bytes.Equal(actual, expected) {
			t.Fatal("full private filter DTO changed", string(actual), string(expected))
		}
	}
	vacation, err := facade.Vacation(ctx)
	if err != nil || vacation.PrivateRevision == "" || vacation.UpdatedAt.IsZero() {
		t.Fatal("full private vacation unavailable", err)
	}
	vacation.PrivateRevision = ""
	if source.privateSettingsEdited {
		expectedVacation.UpdatedAt = vacation.UpdatedAt
	}
	actual, _ := json.Marshal(vacation)
	expected, _ := json.Marshal(expectedVacation)
	if !bytes.Equal(actual, expected) {
		t.Fatal("full private vacation DTO changed", string(actual), string(expected))
	}
}

func exercisePrivateMailSettings(ctx context.Context, t *testing.T, repository *privateMail.Repository) {
	t.Helper()
	facade, err := api.NewMail("https://closed-legacy.invalid", nil, nil).WithPrivateData(repository)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := facade.Filters(ctx)
	if err != nil || len(rules) != 1 {
		t.Fatal(rules, err)
	}
	old := rules[0]
	edited := old
	edited.Name += " (private edit)"
	if err := facade.UpdateFilter(ctx, edited); err != nil {
		t.Fatal(err)
	}
	if err := facade.UpdateFilter(ctx, old); !errors.Is(err, privateMail.ErrConflict) {
		t.Fatal("real SDK accepted stale rule", err)
	}
	created, err := facade.CreateFilter(ctx, api.MailFilter{Name: "Temporary private rule", MatchSubject: "Temporary", MarkFlagged: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.CreateFilter(ctx, created); err != nil {
		t.Fatal("real SDK create retry changed operation", err)
	}
	if err := facade.DeleteFilter(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	vacation, err := facade.Vacation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	oldVacation := vacation
	vacation.Body += " (private edit)"
	if err := facade.SetVacation(ctx, vacation); err != nil {
		t.Fatal(err)
	}
	if err := facade.SetVacation(ctx, oldVacation); !errors.Is(err, privateMail.ErrConflict) {
		t.Fatal("real SDK accepted stale vacation", err)
	}
}
