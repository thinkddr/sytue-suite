// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateNotes "git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type notesSourceSession struct{}

func (notesSourceSession) AccessToken(context.Context) (string, error) {
	return "synthetic-notes-source", nil
}
func (notesSourceSession) Invalidate(string) {}

type countedNotesHTTPSource struct {
	privateNotes.Source
	pages, opens int
}

func (s *countedNotesHTTPSource) Page(ctx context.Context, scope storage.Scope, epoch string, cursor storage.LegacySourceCursor) (privateNotes.SnapshotPage, error) {
	s.pages++
	return s.Source.Page(ctx, scope, epoch, cursor)
}

func (s *countedNotesHTTPSource) Open(ctx context.Context, scope storage.Scope, epoch string, row privateNotes.SnapshotRow) (io.ReadCloser, error) {
	s.opens++
	return s.Source.Open(ctx, scope, epoch, row)
}

func notesSourceFixtureURL(t *testing.T, key string) *url.URL {
	t.Helper()
	u, err := url.Parse(os.Getenv(key))
	if err != nil || u.Scheme != "http" || u.User != nil || u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" || u.Port() == "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		t.Fatal("disposable loopback source endpoint required")
	}
	return u
}

func notesHTTPOriginals(ctx context.Context, t *testing.T, source privateNotes.Source, scope storage.Scope, epoch string) map[string][]byte {
	t.Helper()
	originals := map[string][]byte{}
	cursor := storage.LegacySourceCursor{}
	for {
		page, err := source.Page(ctx, scope, epoch, cursor)
		if err != nil {
			t.Fatal("read actual source descriptors", err)
		}
		for _, row := range page.Rows {
			body, err := source.Open(ctx, scope, epoch, row)
			if err != nil {
				t.Fatal("read actual source body", err)
			}
			raw, readErr := io.ReadAll(io.LimitReader(body, privateNotes.MaxEntityJSON+1))
			closeErr := body.Close()
			if readErr != nil || closeErr != nil || int64(len(raw)) != row.Size || digestBytes(raw) != row.SHA256 {
				t.Fatal("actual source commitment mismatch", readErr, closeErr)
			}
			key := row.Kind + "/" + row.ID
			if _, duplicate := originals[key]; duplicate {
				t.Fatal("actual source repeated a descriptor")
			}
			originals[key] = raw
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	return originals
}

func verifyNotesHTTPOriginals(ctx context.Context, t *testing.T, repository *privateNotes.Repository, originals map[string][]byte) {
	t.Helper()
	for key, want := range originals {
		kind, id, ok := strings.Cut(key, "/")
		if !ok {
			t.Fatal("invalid source key")
		}
		got, err := repository.ReadRaw(ctx, kind, id)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("retained SQL bytes changed in encrypted storage", err)
		}
		clear(got)
	}
}

func TestPrivateNotesHTTPSourceAgainstSynapseAndFreshDevice(t *testing.T) {
	if os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE") != "1" {
		t.Skip("requires real PostgreSQL source supplied by its parent service fixture")
	}
	base := notesSourceFixtureURL(t, "SYTUE_NOTES_SOURCE_FIXTURE_URL")
	upstream := notesSourceFixtureURL(t, "SYTUE_NOTES_SOURCE_FIXTURE_SYNAPSE_URL")
	var scope storage.Scope
	if json.Unmarshal([]byte(os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE_SCOPE")), &scope) != nil || !scope.Valid() || scope.Product != "notes" {
		t.Fatal("synthetic source scope required")
	}
	bookID, noteID := os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE_BOOK"), os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE_NOTE")
	if _, err := uuid.Parse(bookID); err != nil {
		t.Fatal("synthetic book required")
	}
	if _, err := uuid.Parse(noteID); err != nil {
		t.Fatal("synthetic note required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	t.Cleanup(proxy.Close)
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	client := api.NewNotes(base.String(), notesSourceSession{}, nil)
	port, err := privateNotes.NewHTTPSource(client)
	if err != nil {
		t.Fatal(err)
	}
	source := &countedNotesHTTPSource{Source: port}
	state, err := source.State(ctx, scope)
	if err != nil || state.Mode != "legacy" {
		t.Fatal("expected real unfrozen SQL source", err)
	}
	state, err = source.Begin(ctx, scope, uuid.NewString())
	if err != nil || state.Mode != "frozen" || state.Rows < 8 || state.Shared {
		t.Fatal("actual source capture failed", err)
	}
	originals := notesHTTPOriginals(ctx, t, source, scope, state.Epoch)
	defer func() {
		for _, raw := range originals {
			clear(raw)
		}
	}()
	if int64(len(originals)) != state.Rows {
		t.Fatal("actual source graph was incomplete")
	}
	gateway := &fileGatewayFixture{product: "notes"}
	blobServer := httptest.NewServer(gateway)
	t.Cleanup(blobServer.Close)
	blobs := api.NewDrive(blobServer.URL, nil, blobServer.Client())
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	open := func(crypto privateNotes.Crypto, work *offline.Store) *privateNotes.Repository {
		t.Helper()
		r, err := privateNotes.New(privateNotes.Config{Context: ctx, Scope: scope, Crypto: crypto, Gateway: blobs, Journal: work, Source: source})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	lost := &lostStorageIndexACK{Client: device.Client, drop: true}
	repository := open(lost, journal)
	if err := repository.ImportLegacySnapshot(ctx); err == nil || lost.drop || !lost.accepted.Valid() {
		t.Fatal("lost accepted import ACK was not exercised", err)
	}
	repository.Close()
	if err := device.Client.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(ctx, device.Config)
	if err != nil {
		t.Fatal("restart actual SDK", err)
	}
	defer func() { _ = restarted.Close() }()
	go func() { _ = restarted.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := restarted.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	repository = open(restarted, journal)
	defer repository.Close()
	if err := repository.ImportLegacySnapshot(ctx); err != nil {
		t.Fatal("resume actual retained SQL import", err)
	}
	verifyNotesHTTPOriginals(ctx, t, repository, originals)
	rows, err := repository.ListNotes(ctx, bookID)
	if err != nil || len(rows) != 1 || rows[0].ID != noteID {
		t.Fatal("private SQL projection lost live/trash separation", err)
	}
	in := rows[0].Input()
	in.Title = "Device-only updated title"
	if _, err := repository.UpdateNote(ctx, noteID, bookID, in); err != nil {
		t.Fatal("private post-import edit", err)
	}
	if err := restarted.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	fresh := freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), device)
	emptyJournal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	recovered := open(fresh, emptyJournal)
	defer recovered.Close()
	pages, opens := source.pages, source.opens
	if err := recovered.Initialize(ctx); err != nil {
		t.Fatal("independent SDK recovery", err)
	}
	rows, err = recovered.ListNotes(ctx, bookID)
	if err != nil || len(rows) != 1 || rows[0].Title != in.Title || source.pages != pages || source.opens != opens {
		t.Fatal("fresh recovery depended on SQL pages or old local WAL", err)
	}
	for key, want := range originals {
		if key == "note/"+noteID {
			continue
		}
		verifyNotesHTTPOriginals(ctx, t, recovered, map[string][]byte{key: want})
	}
	raw, err := recovered.ReadRaw(ctx, "note", noteID)
	if err != nil || !bytes.Contains(raw, []byte("900719925474099312345")) {
		t.Fatal("private edit lost precise retained JSON", err)
	}
	clear(raw)
	if _, err := recovered.UpdateNote(ctx, noteID, bookID, in); !errors.Is(err, privateNotes.ErrConflict) {
		t.Fatal("stale private edit accepted", err)
	}
	for _, ciphertext := range gateway.data {
		if bytes.Contains(ciphertext, []byte("private source sentinel")) || bytes.Contains(ciphertext, []byte("900719925474099312345")) {
			t.Fatal("binary provider received actual SQL content in clear")
		}
	}
	t.Log("NOTES_PG_HTTP_SDK_FRESH_PASS real retained source, lost import ACK, exact full rows, private edit, backup and fresh journal; no SQL Page/Open after recovery")
}
