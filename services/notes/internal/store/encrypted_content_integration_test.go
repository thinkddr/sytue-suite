// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func TestEncryptedContentMigrationChecksBothRevisionsAndRetainsTheOriginalUntilCommitted(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	bookIn := entradaCuaderno("Private", domain.VisPersonal)
	book, err := st.CreateNotebook(ctx, "ada", bookIn)
	if err != nil {
		t.Fatal(err)
	}
	x := 42
	legacy, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{
		Title: "private title", Content: "private body", X: &x,
		Data: json.RawMessage(`{"client_id":"capture-1","attachments":[{"dataUrl":"private-image"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	bookIn.Encrypted = true
	bookIn.Crypto = &domain.CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Salt: strings.Repeat("a", 24), Iterations: 600000, Check: "encv1:fixture"}
	if err := bookIn.Normalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateNotebook(ctx, book.ID, "ada", bookIn); err != nil {
		t.Fatal(err)
	}
	crypto, _ := json.Marshal(bookIn.Crypto)
	in := domain.EncryptedContentInput{
		CipherVersion: 2, ExpectedUpdatedAt: legacy.UpdatedAt, ExpectedCrypto: crypto,
		ContentCipher: "encv1:client-ciphertext", Data: json.RawMessage(`{"client_id":"capture-1"}`),
	}
	if _, err := st.UpdateNote(ctx, legacy.ID, "ada", domain.NoteInput{CipherVersion: 2, ExpectedCrypto: crypto, ContentCipher: in.ContentCipher}); !errors.Is(err, domain.ErrEncryptedMigrationRequired) {
		t.Fatalf("ordinary update could discard legacy Data: %v", err)
	}
	if _, err := st.MigrateEncryptedContent(ctx, legacy.ID, "bob", in); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another person could migrate a private note: %v", err)
	}
	if _, err := st.MigrateEncryptedContent(tenantCtx("other"), legacy.ID, "ada", in); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("migration escaped tenant RLS: %v", err)
	}
	stale := in
	stale.ExpectedUpdatedAt = stale.ExpectedUpdatedAt.Add(-time.Second)
	if _, err := st.MigrateEncryptedContent(ctx, legacy.ID, "ada", stale); !errors.Is(err, domain.ErrNoteChanged) {
		t.Fatalf("stale note revision was accepted: %v", err)
	}
	// Change salt while keeping Check: a check-only CAS would miss this rotation.
	changed := *bookIn.Crypto
	changed.Salt = strings.Repeat("b", 24)
	bookIn.Crypto = &changed
	if _, err := st.UpdateNotebook(ctx, book.ID, "ada", bookIn); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MigrateEncryptedContent(ctx, legacy.ID, "ada", in); !errors.Is(err, domain.ErrNotebookCryptoChanged) {
		t.Fatalf("changed public KDF parameters were accepted: %v", err)
	}
	current, err := st.ListNotes(ctx, book.ID, "ada")
	if err != nil || len(current) != 1 || current[0].ContentCipher != "" || !strings.Contains(string(current[0].Data), "private-image") {
		t.Fatalf("a failed migration changed the original: %#v %v", current, err)
	}
	in.ExpectedCrypto, _ = json.Marshal(bookIn.Crypto)
	sealed, err := st.MigrateEncryptedContent(ctx, legacy.ID, "ada", in)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Title != "" || sealed.Content != "" || sealed.ContentCipher != in.ContentCipher || strings.Contains(string(sealed.Data), "private") || sealed.X != x || sealed.ID != legacy.ID {
		t.Fatalf("migration retained plaintext or changed layout/identity: %#v", sealed)
	}
	if sealed.Indexable == nil || *sealed.Indexable {
		t.Fatal("sealed note remained indexable")
	}
	if _, err := st.UpdateNote(ctx, legacy.ID, "ada", domain.NoteInput{ContentCipher: in.ContentCipher}); !errors.Is(err, domain.ErrEncryptedClientUpgrade) {
		t.Fatalf("an old client could overwrite sealed Data: %v", err)
	}
	if _, err := st.UpdateNote(ctx, legacy.ID, "ada", domain.NoteInput{CipherVersion: 2, ExpectedCrypto: in.ExpectedCrypto, ContentCipher: in.ContentCipher, Data: json.RawMessage(`{"url":"https://example.org/private"}`)}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("a modern client could persist private Data beside ciphertext: %v", err)
	}
	if _, err := st.MigrateEncryptedContent(ctx, legacy.ID, "ada", in); !errors.Is(err, domain.ErrNoteChanged) {
		t.Fatalf("an ambiguous completed migration was blindly replayed: %v", err)
	}
}

func TestEncryptedArchiveCannotSkipPrivateDataOrCreateAnEmptyDestination(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	book := domain.Notebook{Title: "Backup", Encrypted: true, Crypto: &domain.CryptoParams{
		V: 1, KDF: "PBKDF2-SHA256", Salt: strings.Repeat("a", 24), Iterations: 600000, Check: "encv1:fixture",
	}}
	legacy := domain.Note{ContentCipher: "encv1:client-ciphertext", Data: json.RawMessage(`{"attachments":[{"dataUrl":"private-image"}]}`)}
	if _, _, _, err := st.ImportNotebook(ctx, "ada", book, []domain.Note{legacy}); !errors.Is(err, domain.ErrEncryptedMigrationRequired) {
		t.Fatalf("legacy private Data was skipped or imported in clear: %v", err)
	}
	books, err := st.ListNotebooks(ctx, "ada")
	if err != nil || len(books) != 0 || !strings.Contains(string(legacy.Data), "private-image") {
		t.Fatalf("rejected import changed source or left an empty notebook: %v %#v", err, books)
	}
	sealed := legacy
	sealed.Data = json.RawMessage(`{"client_id":"archive-1","repeated_into":"old-object-id"}`)
	created, imported, skipped, err := st.ImportNotebook(ctx, "ada", book, []domain.Note{sealed})
	if err != nil || imported != 1 || skipped != 0 {
		t.Fatalf("sealed archive failed: %d/%d %v", imported, skipped, err)
	}
	notes, err := st.ListNotes(ctx, created.ID, "ada")
	if err != nil || len(notes) != 1 || notes[0].ContentCipher != sealed.ContentCipher || strings.Contains(string(notes[0].Data), "old-object-id") {
		t.Fatalf("archive changed ciphertext or trusted task references from another notebook: %#v %v", notes, err)
	}
}

func TestEncryptedWritesRejectStaleKeysAndMalformedExpectedCrypto(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	bookIn := entradaCuaderno("Private", domain.VisPersonal)
	bookIn.Encrypted = true
	bookIn.Crypto = &domain.CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Salt: strings.Repeat("a", 24), Iterations: 600000, Check: "encv1:fixture"}
	if err := bookIn.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", bookIn)
	if err != nil {
		t.Fatal(err)
	}
	crypto, _ := json.Marshal(bookIn.Crypto)
	in := domain.NoteInput{CipherVersion: 2, ExpectedCrypto: crypto, ContentCipher: "encv1:sealed"}
	note, err := st.CreateNote(ctx, book.ID, "ada", in)
	if err != nil {
		t.Fatal(err)
	}
	changed := *bookIn.Crypto
	changed.Salt = strings.Repeat("b", 24)
	bookIn.Crypto = &changed
	if _, err := st.UpdateNotebook(ctx, book.ID, "ada", bookIn); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []json.RawMessage{crypto, nil, json.RawMessage(`{`)} {
		in.ExpectedCrypto = expected
		if _, err := st.CreateNote(ctx, book.ID, "ada", in); !errors.Is(err, domain.ErrNotebookCryptoChanged) {
			t.Fatalf("stale or malformed create key was accepted: %v", err)
		}
		if _, err := st.UpdateNote(ctx, note.ID, "ada", in); !errors.Is(err, domain.ErrNotebookCryptoChanged) {
			t.Fatalf("stale or malformed update key was accepted: %v", err)
		}
	}
	in.ExpectedCrypto, _ = json.Marshal(bookIn.Crypto)
	in.ContentCipher = "encv1:current-key"
	if _, err := st.UpdateNote(ctx, note.ID, "ada", in); err != nil {
		t.Fatal(err)
	}
	all, err := st.ListNotes(ctx, book.ID, "ada")
	if err != nil || len(all) != 1 || all[0].ContentCipher != in.ContentCipher {
		t.Fatalf("failed writes changed or duplicated the original: %#v %v", all, err)
	}
}

func TestNotePagesAreStableAcrossWritesAndKeepTenantAndPersonBoundaries(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	book, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Private", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Title: "note"}); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	var after *uuid.UUID
	for {
		page, err := st.ListNotesPage(ctx, book.ID, "ada", 1, after)
		if err != nil || len(page.Notes) != 1 {
			t.Fatalf("page: %#v %v", page, err)
		}
		n := page.Notes[0]
		if seen[n.ID.String()] {
			t.Fatal("a changed record moved to another page")
		}
		seen[n.ID.String()] = true
		if _, err := st.UpdateNote(ctx, n.ID, "ada", domain.NoteInput{Title: "changed"}); err != nil {
			t.Fatal(err)
		}
		if page.NextCursor == "" {
			break
		}
		cursor, err := uuid.Parse(page.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
		after = &cursor
	}
	if len(seen) != 3 {
		t.Fatalf("lost records: %d", len(seen))
	}
	for _, who := range []struct {
		ctx context.Context
		sub string
	}{{ctx, "bob"}, {tenantCtx("other"), "ada"}} {
		page, err := st.ListNotesPage(who.ctx, book.ID, who.sub, 1, nil)
		if err != nil || len(page.Notes) != 0 {
			t.Fatalf("page leaked data: %#v %v", page, err)
		}
	}
}
