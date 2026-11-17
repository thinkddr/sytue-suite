// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type retainedMailSource struct {
	state                      storage.LegacyMigrationState
	rows                       []storage.StreamSourceRow
	bodies                     map[storage.LegacySourceCursor][]byte
	pages, opens               int
	forbidPages                bool
	shared, unavailableIngress bool
	omitLast                   bool
}

func (s *retainedMailSource) PrivateStorageCapabilities(context.Context, storage.Scope) (storage.AppMigrationState, error) {
	return storage.AppMigrationState{Version: 1, LegacyMigrationState: s.state, MaxBodyBytes: 50 << 20, PrivateIngressReady: !s.unavailableIngress, Shared: s.shared}, nil
}

func (s *retainedMailSource) PrivateStorageState(context.Context, storage.Scope) (storage.LegacyMigrationState, error) {
	return s.state, nil
}

func (s *retainedMailSource) PrivateStorageSourcePage(_ context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	s.pages++
	if s.forbidPages {
		return storage.StreamSourcePage{}, errors.New("SQL source pages must not be reimported")
	}
	page := storage.StreamSourcePage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{}}
	rows := s.rows
	if s.omitLast {
		rows = rows[:len(rows)-1]
	}
	for _, row := range rows {
		if row.Kind < after.Kind || row.Kind == after.Kind && row.ID <= after.ID {
			continue
		}
		page.Rows = append(page.Rows, row)
		if len(page.Rows) == 3 {
			break
		}
	}
	if len(page.Rows) > 2 {
		page.Rows = page.Rows[:2]
		last := page.Rows[1]
		page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
	}
	return page, nil
}

func (s *retainedMailSource) OpenPrivateStorageSource(_ context.Context, _ storage.Scope, _ string, row storage.StreamSourceRow) (io.ReadCloser, error) {
	s.opens++
	body, ok := s.bodies[storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

type importFixture struct {
	repo    *Repository
	crypto  *cryptoFixture
	source  *retainedMailSource
	cfg     Config
	path    string
	secrets *authtest.Secrets
}

func setupImport(t *testing.T) *importFixture {
	t.Helper()
	started := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	source := &retainedMailSource{state: storage.LegacyMigrationState{Scope: testScope, Mode: "frozen", Epoch: testEpoch, StartedAt: &started}, bodies: map[storage.LegacySourceCursor][]byte{}}
	add := func(kind, id string, raw []byte) {
		source.rows = append(source.rows, storage.StreamSourceRow{Kind: kind, ID: id, Size: int64(len(raw)), SHA256: digest(raw)})
		source.bodies[storage.LegacySourceCursor{Kind: kind, ID: id}] = raw
		if privateBody(storage.StreamSourceRow{Kind: kind}) {
			source.state.Blobs++
		} else {
			source.state.Rows++
		}
	}
	add("account", "account", []byte(`{"id":"account","signature":"Private signature"}`))
	add("mailbox", "inbox", []byte(`{"id":"inbox","name":"Inbox","role":"inbox","parent_id":null,"uid_next":900719925474099512345}`))
	add("label", "label", []byte(`{"id":"label","name":"Private label","color":"blue"}`))
	add("message", "live", []byte(`{"id":"live","mailbox_id":"inbox","thread_id":"thread","rfc822_msg_id":"<original@example.test>","subject":"Private subject","from_addr":"sender@example.test","to_addrs":["alice@example.test"],"size_bytes":8,"flags":[],"received_at":"2026-10-30T00:00:00.123456789Z","sent_at":null,"deleted_at":null,"uid":900719925474099512345}`))
	add("message", "deleted", []byte(`{"id":"deleted","mailbox_id":"inbox","size_bytes":7,"deleted_at":"2026-10-29T00:00:00Z"}`))
	add("mime_message", "live", []byte("original"))
	add("mime_message", "deleted", []byte("deleted"))
	add("outbound", "held", []byte(`{"id":"held","state":"queued","hold":"scheduled","recipients":["outside@example.test"]}`))
	add("mime_outbound", "held", []byte("queued MIME"))
	add("thread", "thread", []byte(`{"id":"thread","subject":"Private thread"}`))
	add("vacation", "account", []byte(`{"account_id":"account","subject":"Away","body":"Private auto-reply"}`))
	add("filter", "large", []byte(`{"id":"large","precise":900719925474099512345,"rule":"`+strings.Repeat("x", 70<<10)+`"}`))
	slices.SortFunc(source.rows, func(a, b storage.StreamSourceRow) int {
		if n := strings.Compare(a.Kind, b.Kind); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	path := t.TempDir()
	secrets := &authtest.Secrets{}
	journal, err := offline.OpenWithSecrets(path, secrets)
	if err != nil {
		t.Fatal(err)
	}
	c := newCryptoFixture()
	cfg := Config{Context: t.Context(), Scope: testScope, Crypto: c, Gateway: api.NewDrive("https://unused.invalid", nil, nil), Journal: journal, MigrationState: source.state}
	repo, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Close)
	return &importFixture{repo: repo, crypto: c, source: source, cfg: cfg, path: path, secrets: secrets}
}

func (f *importFixture) reopen(t *testing.T) *Repository {
	t.Helper()
	f.repo.Close()
	journal, err := offline.OpenWithSecrets(f.path, f.secrets)
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.cfg
	cfg.Journal = journal
	repo, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Close)
	f.repo = repo
	f.cfg = cfg
	return repo
}

func TestPrivateMailImporterArchivesEveryOriginalBeforeActivation(t *testing.T) {
	f := setupImport(t)
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	boxes, err := f.repo.Mailboxes(t.Context())
	if err != nil || len(boxes) != 1 || boxes[0].TotalEmails != 1 {
		t.Fatal("private inbox not active", boxes, err)
	}
	messages, _, err := f.repo.Messages(t.Context(), "inbox", "", 50)
	if err != nil || len(messages) != 1 || messages[0].ID != "live" || messages[0].Size != 8 || messages[0].ReceivedAt.Nanosecond() != 123456789 {
		t.Fatal("lost live envelope or surfaced deleted original", messages, err)
	}
	body, err := f.repo.Raw(t.Context(), "live")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(raw) != "original" {
		t.Fatal("changed MIME", err)
	}
	if signature, err := f.repo.Signature(t.Context()); err != nil || signature != "Private signature" {
		t.Fatal("lost signature", signature, err)
	}
	snapshot, err := storage.LoadAtomicCatalog(t.Context(), f.crypto, testScope)
	if err != nil {
		t.Fatal(err)
	}
	versions := heads(snapshot, objectID("migration", testEpoch))
	if len(versions) != 1 {
		t.Fatal("missing sole completion")
	}
	receiptRaw, err := f.repo.readDocument(t.Context(), versions[0])
	if err != nil {
		t.Fatal(err)
	}
	defer clear(receiptRaw)
	var receipt ImportReceipt
	if json.Unmarshal(receiptRaw, &receipt) != nil || receipt.Kind != migrationKindV2 || len(receipt.Sources) != len(f.source.rows) {
		t.Fatal("incomplete source mapping")
	}
	for _, item := range receipt.Sources {
		archived, err := f.repo.verifyArchive(t.Context(), snapshot, item.Source, item.Archive)
		if err != nil {
			t.Fatal(err)
		}
		expected := f.source.bodies[storage.LegacySourceCursor{Kind: item.Source.Kind, ID: item.Source.ID}]
		if privateBody(item.Source) {
			entry := snapshot.Entries[item.Archive]
			version, err := storage.DecodeObject(entry, testScope)
			if err != nil {
				t.Fatal(err)
			}
			archived = f.crypto.files[version.Commit.Content.Reference].data
		}
		if !bytes.Equal(archived, expected) {
			t.Fatal("changed retained original", item.Source.Kind)
		}
		if !privateBody(item.Source) {
			clear(archived)
		}
	}
	pages := f.source.pages
	puts := f.crypto.puts
	f.source.forbidPages = true
	if err := f.reopen(t).ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	if f.source.pages != pages || f.crypto.puts != puts {
		t.Fatal("completed import re-enumerated SQL or republished data")
	}
}

func TestPrivateMailImporterLostACKResumesWithoutSourcePages(t *testing.T) {
	f := setupImport(t)
	f.crypto.loseIndex = true
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err == nil {
		t.Fatal("lost ACK unexpectedly completed")
	}
	if _, err := f.repo.Mailboxes(t.Context()); !errors.Is(err, ErrNotReady) {
		t.Fatal("partial source became ready", err)
	}
	pages := f.source.pages
	f.source.forbidPages = true
	if err := f.reopen(t).ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	if f.source.pages != pages {
		t.Fatal("resumed import re-enumerated source pages")
	}
}

func TestPrivateMailImporterRefusesSourceAndTargetChanges(t *testing.T) {
	for _, fault := range []string{"legacy", "shared", "ingress-unavailable", "missing-row", "changed-bytes", "foreign-target", "changed-target", "altered-journal"} {
		t.Run(fault, func(t *testing.T) {
			f := setupImport(t)
			switch fault {
			case "legacy":
				f.source.state.Mode = "legacy"
			case "shared":
				f.source.shared = true
			case "ingress-unavailable":
				f.source.unavailableIngress = true
			case "missing-row":
				f.source.omitLast = true
			case "changed-bytes":
				f.source.bodies[storage.LegacySourceCursor{Kind: "account", ID: "account"}] = []byte(`{"id":"account","signature":"changed"}`)
			case "foreign-target":
				f.crypto.seed(t, "mailbox", "foreign", map[string]any{"id": "foreign", "name": "Foreign"})
			case "changed-target", "altered-journal":
				f.crypto.loseIndex = true
				if err := f.repo.ImportRetainedSource(t.Context(), f.source); err == nil {
					t.Fatal("setup ACK did not fail")
				}
				f.source.forbidPages = true
				if fault == "changed-target" {
					f.crypto.binding.Target.DeviceID = "CHANGED"
				} else {
					owner := testScope.Tenant + "\x00" + testScope.Subject + "\x00mail"
					var work importIntent
					found, err := f.cfg.Journal.Read(owner, importNamespace, testEpoch, &work)
					if err != nil || !found {
						t.Fatal("missing import journal", err)
					}
					work.Items[0].Source.SHA256 = strings.Repeat("a", 64)
					if err := f.cfg.Journal.Write(owner, importNamespace, testEpoch, work); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := f.repo.ImportRetainedSource(t.Context(), f.source); err == nil {
				t.Fatal("changed source/target activated")
			}
			if _, err := f.repo.Mailboxes(t.Context()); !errors.Is(err, ErrNotReady) {
				t.Fatal("failed import became ready", err)
			}
		})
	}
}

func TestPrivateMailImporterCorruptArchivePreventsFreshRecovery(t *testing.T) {
	f := setupImport(t)
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal(err)
	}
	for ref, file := range f.crypto.files {
		if string(file.data) == "original" {
			file.data = []byte("tampered")
			f.crypto.files[ref] = file
			break
		}
	}
	if err := f.reopen(t).Initialize(t.Context()); err == nil {
		t.Fatal("corrupt archived body activated on fresh device")
	}
}

func TestPrivateMailImporterStreamsRetainedMIMEAboveCurrentSendLimit(t *testing.T) {
	f := setupImport(t)
	large := bytes.Repeat([]byte("private-retained-mime-block!"), ((64<<20)/28)+1)
	// The exact length need not align to a cipher/staging block.
	large = large[:64<<20]
	for i := range f.source.rows {
		row := &f.source.rows[i]
		if row.Kind == "mime_message" && row.ID == "live" {
			row.Size = int64(len(large))
			row.SHA256 = digest(large)
			f.source.bodies[storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}] = large
		}
		if row.Kind == "message" && row.ID == "live" {
			cursor := storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}
			raw, err := patchField(f.source.bodies[cursor], "size_bytes", len(large))
			if err != nil {
				t.Fatal(err)
			}
			row.Size = int64(len(raw))
			row.SHA256 = digest(raw)
			f.source.bodies[cursor] = raw
		}
	}
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err != nil {
		t.Fatal("retained MIME above the current new-message limit was lost", err)
	}
	staged, err := f.cfg.Journal.List(testScope.Tenant+"\x00"+testScope.Subject+"\x00mail", importStageNamespace)
	if err != nil || len(staged) != 0 {
		t.Fatal("verified import retained staged input", len(staged), err)
	}
	body, err := f.repo.Raw(t.Context(), "live")
	if err != nil {
		t.Fatal(err)
	}
	reader := body.(*bodyReader)
	if reader.stream == nil || len(reader.data) != 0 {
		t.Fatal("large MIME was materialized in the view reader")
	}
	first := make([]byte, 1024)
	if _, err := io.ReadFull(body, first); err != nil || !bytes.Equal(first, large[:1024]) {
		t.Fatal("stream changed retained MIME", err)
	}
	f.repo.Close()
	if _, err := body.Read(first); !errors.Is(err, ErrClosed) {
		t.Fatal("closed account retained large MIME stream", err)
	}
}

type boundedMailSource struct{ size int64 }

func (s boundedMailSource) ReadAt(out []byte, offset int64) (int, error) {
	if offset >= s.size {
		return 0, io.EOF
	}
	n := int(min(int64(len(out)), s.size-offset))
	clear(out[:n])
	if n < len(out) {
		return n, io.EOF
	}
	return n, nil
}

func TestPrivateMailNewMessageSizeUsesTheConfiguredCapability(t *testing.T) {
	f := setup(t, true)
	cfg := f.repo.cfg
	cfg.MaxMessageBytes = 100 << 20
	repo, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	size := int64(64 << 20)
	sum, err := sourceDigest(t.Context(), boundedMailSource{size: size}, size, repo.messageLimit())
	if err != nil || len(sum) != 64 {
		t.Fatal("configured message size was replaced by a fixed50MiB limit", err)
	}
	if _, err := sourceDigest(t.Context(), boundedMailSource{size: size}, size, 50<<20); err == nil {
		t.Fatal("input above the configured message policy was accepted")
	}
}

func TestPrivateMailImporterRefusesExistingTargetBeforePublishing(t *testing.T) {
	f := setupImport(t)
	f.crypto.seed(t, "mailbox", "foreign", map[string]any{"id": "foreign", "name": "Existing private data"})
	puts, files := f.crypto.puts, len(f.crypto.files)
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err == nil {
		t.Fatal("existing target activated")
	}
	if f.crypto.puts != puts || len(f.crypto.files) != files {
		t.Fatal("existing target was modified before rejection")
	}
}

func TestPrivateMailImporterRejectsChangedBytesBeforeAnyCipherUpload(t *testing.T) {
	f := setupImport(t)
	cursor := storage.LegacySourceCursor{Kind: "account", ID: "account"}
	f.source.bodies[cursor] = bytes.Replace(f.source.bodies[cursor], []byte("Private signature"), []byte("Another signature"), 1)
	if err := f.repo.ImportRetainedSource(t.Context(), f.source); err == nil {
		t.Fatal("changed source bytes activated")
	}
	if f.crypto.puts != 0 || len(f.crypto.files) != 0 {
		t.Fatal("unverified source bytes reached a cloud upload")
	}
}
