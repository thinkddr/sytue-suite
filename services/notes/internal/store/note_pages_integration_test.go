// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

type notePageQueryGate struct {
	entered, release chan struct{}
	once             sync.Once
}

func (g *notePageQueryGate) TraceQueryStart(ctx context.Context, _ *pgx.Conn, query pgx.TraceQueryStartData) context.Context {
	if strings.Contains(query.SQL, "WHERE n.tenant=$1 AND n.id=ANY($2)") {
		g.once.Do(func() {
			close(g.entered)
			select {
			case <-g.release:
			case <-ctx.Done():
			}
		})
	}
	return ctx
}

func (*notePageQueryGate) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestNotePageBudgetAndMetadataUseTheSameSnapshotDuringConcurrentWrites(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx, cancel := context.WithTimeout(tenantCtx("acme"), 10*time.Second)
	defer cancel()
	st := store.New(h.Pool)
	book, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Snapshot", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Title: "original"})
	if err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(h.AppDSN)
	if err != nil {
		t.Fatal(err)
	}
	gate := &notePageQueryGate{entered: make(chan struct{}), release: make(chan struct{})}
	config.ConnConfig.Tracer = gate
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	reader := store.New(&pg.Pool{Pool: pool})
	type result struct {
		page store.NotePage
		err  error
	}
	done := make(chan result, 1)
	go func() { page, err := reader.ListNotesPage(ctx, book.ID, "ada", 100, nil); done <- result{page, err} }()
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("page did not reach the payload query after candidate selection")
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO note_links (tenant,note_id,target_kind,target_id,label) VALUES ('acme',$1,'note','new-reference','arrived after budget')`, note.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	release.Do(func() { close(gate.release) })
	select {
	case got := <-done:
		if got.err != nil || len(got.page.Notes) != 1 || len(got.page.Notes[0].Links) != 0 {
			t.Fatalf("metadata changed between budget and materialization: notes=%d err=%v", len(got.page.Notes), got.err)
		}
	case <-ctx.Done():
		t.Fatal("snapshot page did not finish")
	}
	fresh, err := reader.ListNotesPage(ctx, book.ID, "ada", 100, nil)
	if err != nil || len(fresh.Notes) != 1 || len(fresh.Notes[0].Links) != 1 {
		t.Fatal("next page read did not observe the committed reference")
	}
}

func TestNoteBytePagesSelectBodiesBeforeMaterializingAndDoNotSkipChangedNotes(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st, ctx := store.New(h.Pool), tenantCtx("acme")
	in := entradaCuaderno("Large", domain.VisPersonal)
	in.Encrypted = true
	in.Crypto = &domain.CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Salt: strings.Repeat("a", 24), Iterations: 600000, Check: "encv1:fixture"}
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", in)
	if err != nil {
		t.Fatal(err)
	}
	crypto, _ := json.Marshal(in.Crypto)
	input := domain.NoteInput{CipherVersion: 2, ExpectedCrypto: crypto, ContentCipher: "encv1:" + strings.Repeat("A", 8<<20), Data: json.RawMessage(`{}`)}
	for range 3 {
		if _, err := st.CreateNote(ctx, book.ID, "ada", input); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[uuid.UUID]bool{}
	var after *uuid.UUID
	for {
		page, err := st.ListNotesPage(ctx, book.ID, "ada", 100, after)
		if err != nil || len(page.Notes) != 1 {
			t.Fatalf("byte budget loaded a batch of large bodies: count=%d err=%v", len(page.Notes), err)
		}
		note := page.Notes[0]
		if seen[note.ID] || note.ContentCipher != input.ContentCipher {
			t.Fatal("large page repeated or damaged a note")
		}
		seen[note.ID] = true
		if _, err := st.UpdateNote(ctx, note.ID, "ada", input); err != nil {
			t.Fatal(err)
		}
		if page.NextCursor == "" {
			break
		}
		if page.NextCursor != note.ID.String() {
			t.Fatal("cursor skipped an undelivered note")
		}
		id := note.ID
		after = &id
	}
	if len(seen) != 3 {
		t.Fatal("byte pages lost a large note")
	}
	for _, who := range []struct {
		ctx context.Context
		sub string
	}{{ctx, "bob"}, {tenantCtx("other"), "ada"}} {
		page, err := st.ListNotesPage(who.ctx, book.ID, who.sub, 100, nil)
		if err != nil || len(page.Notes) != 0 || page.NextCursor != "" {
			t.Fatal("bounded candidates leaked another person or tenant")
		}
	}
}

func TestSmallNotesBatchAndLegacyRemainCompleteWithoutSilentTruncation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st, ctx := store.New(h.Pool), tenantCtx("acme")
	book, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Small", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	insert := func(count int) {
		t.Helper()
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO notes (tenant,notebook_id,owner_sub,title,data) SELECT 'acme',$1,'ada','small','{}'::jsonb FROM generate_series(1,$2)`, book.ID, count)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	insert(150)
	page, err := st.ListNotesPage(ctx, book.ID, "ada", 100, nil)
	if err != nil || len(page.Notes) != 100 || page.NextCursor != page.Notes[99].ID.String() {
		t.Fatalf("small batch: %d %v", len(page.Notes), err)
	}
	legacy, err := st.ListNotesLegacyBounded(ctx, book.ID, "ada")
	if err != nil || len(legacy.Notes) != 150 || legacy.NextCursor != "" {
		t.Fatalf("small legacy list truncated: %d %v", len(legacy.Notes), err)
	}
	insert(851)
	legacy, err = st.ListNotesLegacyBounded(ctx, book.ID, "ada")
	if !errors.Is(err, domain.ErrNotePaginationRequired) || len(legacy.Notes) != 0 {
		t.Fatalf("large legacy silently truncated: %d %v", len(legacy.Notes), err)
	}
}

func TestNotePageAccountsForLinkedMetadataBeforeFetchingBody(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st, ctx := store.New(h.Pool), tenantCtx("acme")
	book, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Metadata", domain.VisPersonal))
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Title: "original"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO note_links (tenant,note_id,target_kind,target_id,label,url) SELECT 'acme',$1,'note',x::text,repeat('l',400),repeat('u',2048) FROM generate_series(1,15000) x`, note.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	page, err := st.ListNotesPage(ctx, book.ID, "ada", 100, nil)
	if !errors.Is(err, domain.ErrNotePageTooLarge) || len(page.Notes) != 0 {
		t.Fatalf("oversize linked metadata was materialized: %d %v", len(page.Notes), err)
	}
}
