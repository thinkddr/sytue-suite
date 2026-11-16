// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func TestPrivateNotesSQLCapturesCompleteExactSourceAndFencesEveryLegacyWriter(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	book, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Private", domain.VisPrivate))
	if err != nil {
		t.Fatal(err)
	}
	data := json.RawMessage(`{"integer":900719925474099312345,"scientific":1.234567890123456789e+99,"padding":"` + strings.Repeat("private payload ", 8192) + `"}`)
	note, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Kind: domain.KindText, Title: "Live", Data: data, Links: []domain.NoteLink{{TargetKind: domain.TargetNote, TargetID: "local-reference", Label: "retained link"}}})
	if err != nil {
		t.Fatal(err)
	}
	trash, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Title: "Trash"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteNote(ctx, trash.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	enPapeleraDesde(t, h, trash.ID.String(), 40*24*time.Hour)
	tagInput := domain.TagInput{Name: "State", States: []domain.TagStateInput{{Name: "Open", Color: "#123456"}}}
	if err := tagInput.Normalize(); err != nil {
		t.Fatal(err)
	}
	tag, err := st.SaveTag(ctx, "ada", nil, tagInput)
	if err != nil || len(tag.States) != 1 {
		t.Fatal(err)
	}
	if err := st.SetNoteState(ctx, note.ID, tag.ID, &tag.States[0].ID, "ada"); err != nil {
		t.Fatal(err)
	}
	reminder := domain.ReminderInput{DueAt: time.Now().Add(-time.Hour), TZID: "UTC"}
	if err := reminder.Normalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetReminder(ctx, note.ID, "ada", reminder); err != nil {
		t.Fatal(err)
	}
	var original []byte
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT convert_to((to_jsonb(n)-'tenant')::text,'UTF8') FROM notes n WHERE id=$1`, note.ID).Scan(&original); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO audit_log(tenant,actor,action,target,detail) VALUES('acme','ada','notebook.export',$1,'{"future":900719925474099312345}')`, book.ID.String())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state, err := st.PrivateStorageState(ctx, "ada")
	if err != nil || state.Mode != "legacy" || state.Shared {
		t.Fatal("unexpected initial source", state, err)
	}
	epoch := uuid.New()
	state, err = st.BeginPrivateStorageMigration(ctx, "ada", epoch)
	if err != nil || state.Mode != "frozen" || state.Rows != 9 || state.Blobs != 0 {
		t.Fatal("incomplete frozen source", state, err)
	}
	counts := map[string]int{}
	cursor := storage.LegacySourceCursor{}
	seen := 0
	for {
		page, err := st.PrivateStorageSourcePage(ctx, "ada", epoch, cursor, 2)
		if err != nil || page.Validate(state.Scope, epoch.String(), cursor) != nil {
			t.Fatal("invalid source page", err)
		}
		for _, row := range page.Rows {
			body, err := st.PrivateStorageSourceBody(ctx, "ada", epoch, row.Kind, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(body.Data)
			if row.SHA256 != hex.EncodeToString(sum[:]) || row.Size != int64(len(body.Data)) || body.SHA256 != row.SHA256 {
				t.Fatal("descriptor does not match exact source")
			}
			if row.Kind == "note" && row.ID == note.ID.String() && !bytes.Equal(body.Data, original) {
				t.Fatal("large source JSON changed or was truncated")
			}
			clear(body.Data)
			counts[row.Kind]++
			seen++
			cursor = storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}
		}
		if page.Next == nil {
			break
		}
	}
	if seen != 9 || counts["note"] != 2 || len(counts) != 8 {
		t.Fatal("retained graph was incomplete", counts, seen)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, "ada", epoch); err != nil {
		t.Fatal("same epoch did not resume", err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, "ada", uuid.New()); !errors.Is(err, store.ErrPrivateStorageEpoch) {
		t.Fatal("source epoch changed", err)
	}
	if _, err := st.PrivateStorageSourceBody(ctx, "bob", epoch, "note", note.ID.String()); err == nil {
		t.Fatal("another owner read frozen source")
	}
	if _, err := st.PrivateStorageSourceBody(tenantCtx("other"), "ada", epoch, "note", note.ID.String()); err == nil {
		t.Fatal("another tenant read frozen source")
	}
	writers := []func() error{
		func() error {
			_, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("new", domain.VisPrivate))
			return err
		},
		func() error { _, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Title: "new"}); return err },
		func() error { return st.DeleteNote(ctx, note.ID, "ada") },
		func() error { return st.DeleteTag(ctx, tag.ID, "ada") },
		func() error { return st.DeleteReminder(ctx, note.ID, "ada") },
		func() error { _, _, err := st.CompartirCuaderno(ctx, book.ID, "ada", "bob", false); return err },
		func() error {
			return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE notes SET title='legacy forgery' WHERE id=$1`, note.ID)
				return err
			})
		},
		func() error {
			return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE note_links SET label='changed' WHERE note_id=$1`, note.ID)
				return err
			})
		},
	}
	for index, write := range writers {
		if err := write(); !errors.Is(store.PrivateStorageError(err), store.ErrPrivateStorageFrozen) {
			t.Fatalf("legacy writer %d escaped: %v", index, err)
		}
	}
	if err := h.Pool.InMergeTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE notes_private_storage_snapshot SET payload=convert_to('{}','UTF8') WHERE epoch=$1`, epoch)
		return err
	}); !errors.Is(store.PrivateStorageError(err), store.ErrPrivateStorageFrozen) {
		t.Fatal("snapshot was mutable during merge", err)
	}
	if err := h.Pool.InMergeTx(t.Context(), func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE notes_private_storage_states SET tenant='other' WHERE tenant='acme'`)
		return err
	}); !errors.Is(store.PrivateStorageError(err), store.ErrPrivateStorageFrozen) {
		t.Fatal("frozen scope was reassignable", err)
	}
	// A migrated owner's preserved trash and reminder must not abort maintenance
	// for an unrelated, still-legacy account in the same organisation.
	otherBook, err := st.CreateNotebook(ctx, "bob", entradaCuaderno("Other", domain.VisPrivate))
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateNote(ctx, otherBook.ID, "bob", domain.NoteInput{Title: "Other trash"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteNote(ctx, other.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	enPapeleraDesde(t, h, other.ID.String(), 40*24*time.Hour)
	if count, err := st.PurgeExpiredTrash(t.Context(), 30*24*time.Hour); err != nil || count != 1 {
		t.Fatal("frozen account poisoned unrelated purging", count, err)
	}
	active, err := st.CreateNote(ctx, otherBook.ID, "bob", domain.NoteInput{Title: "Other reminder"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetReminder(ctx, active.ID, "bob", reminder); err != nil {
		t.Fatal(err)
	}
	notified := []string{}
	if count, err := st.ReclamarRecordatorios(t.Context(), time.Now(), 20, func(value store.RecordatorioVencido) { notified = append(notified, value.OwnerSub) }); err != nil || count != 1 || len(notified) != 1 || notified[0] != "bob" {
		t.Fatal("frozen account poisoned unrelated reminders", count, notified, err)
	}
	body, err := st.PrivateStorageSourceBody(ctx, "ada", epoch, "note", note.ID.String())
	if err != nil || !bytes.Equal(body.Data, original) {
		t.Fatal("maintenance changed preserved source", err)
	}
	clear(body.Data)
}

func TestPrivateNotesSQLPreflightRejectsUnsupportedDataWithoutFreezingOldApps(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	for _, kind := range []string{"shared", "attachment", "file-link", "incoming"} {
		t.Run(kind, func(t *testing.T) {
			ctx := tenantCtx("preflight-" + kind)
			book, err := st.CreateNotebook(ctx, "ada", entradaCuaderno("Old app", domain.VisPrivate))
			if err != nil {
				t.Fatal(err)
			}
			input := domain.NoteInput{Title: "Existing"}
			if kind == "attachment" {
				input.Kind = domain.KindImage
				input.Data = json.RawMessage(`{"url":"https://legacy.example.test/plain"}`)
			}
			if kind == "file-link" {
				input.Links = []domain.NoteLink{{TargetKind: domain.TargetDriveFile, TargetID: "legacy-file"}}
			}
			if _, err := st.CreateNote(ctx, book.ID, "ada", input); err != nil {
				t.Fatal(err)
			}
			if kind == "shared" {
				if _, _, err := st.CompartirCuaderno(ctx, book.ID, "ada", "bob", false); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "incoming" {
				if _, err := st.CreateNotebook(ctx, "bob", entradaCuaderno("Team", domain.VisOrganization)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := st.BeginPrivateStorageMigration(ctx, "ada", uuid.New()); err == nil {
				t.Fatal("unsupported source froze")
			}
			state, err := st.PrivateStorageState(ctx, "ada")
			if err != nil || state.Mode != "legacy" || state.Epoch != "" {
				t.Fatal("preflight left legacy app frozen", state, err)
			}
			if _, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Title: "Old app still works"}); err != nil {
				t.Fatal("preflight blocked old app", err)
			}
		})
	}
}
