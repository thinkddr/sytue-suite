// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

func exportFixture(t *testing.T) (*pgtest.Harness, *Store, context.Context, domain.Notebook, domain.NotebookInput) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := New(h.Pool)
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
	input := domain.NotebookInput{Title: "book", Visibility: domain.VisPersonal}
	if err := input.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", input)
	if err != nil {
		t.Fatal(err)
	}
	note := domain.NoteInput{Kind: domain.KindText, Title: "private before migration"}
	if err := note.Normalize(false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateNote(ctx, book.ID, "ada", note); err != nil {
		t.Fatal(err)
	}
	return h, st, ctx, book, input
}

func TestExportReadersShareSnapshotAndPreserveAuthorization(t *testing.T) {
	_, st, ctx, book, input := exportFixture(t)
	if _, _, err := st.ExportNotebook(ctx, book.ID, "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign person exported: %v", err)
	}
	other := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "other"})
	if _, _, err := st.ExportNotebook(other, book.ID, "ada"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign tenant exported: %v", err)
	}
	if err := st.pool.InTxOpts(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead}, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, "acme"); err != nil {
			return err
		}
		before, err := st.getNotebookTx(ctx, tx, book.ID, "ada")
		if err != nil {
			return err
		}
		input.Encrypted = true
		input.Crypto = &domain.CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Salt: "abcdefghijklmnop", Iterations: 600000, Check: "encv1:test"}
		if err := input.Normalize(); err != nil {
			return err
		}
		if _, err := st.UpdateNotebook(ctx, book.ID, "ada", input); err != nil {
			return err
		}
		notes, err := st.listNotesTx(ctx, tx, book.ID, "ada")
		if err != nil {
			return err
		}
		if before.Encrypted || len(notes) != 1 || notes[0].Title != "private before migration" {
			return errors.New("export readers did not share their snapshot")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.ExportNotebook(ctx, book.ID, "ada"); !errors.Is(err, domain.ErrEncryptedMigrationRequired) {
		t.Fatalf("mixed legacy notebook was exported: %v", err)
	}
}

func TestExportReturnsNoDataIfAuditWriteFails(t *testing.T) {
	h, st, ctx, book, _ := exportFixture(t)
	admin := h.PoolSinRLS(t)
	if _, err := admin.Exec(ctx, `CREATE FUNCTION test_refuse_notes_export_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `CREATE TRIGGER test_refuse_export BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION test_refuse_notes_export_audit()`); err != nil {
		t.Fatal(err)
	}
	got, notes, err := st.ExportNotebook(ctx, book.ID, "ada")
	if err == nil || got.ID != uuid.Nil || notes != nil {
		t.Fatalf("audit failure returned data: %+v %v %v", got, notes, err)
	}
}

func TestEncryptedLegacyLinksNeverLeaveOrDisappearFromBackupPaths(t *testing.T) {
	h, st, ctx, _, _ := exportFixture(t)
	bookInput := domain.NotebookInput{
		Title: "encrypted", Visibility: domain.VisPersonal, Encrypted: true,
		Crypto: &domain.CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Salt: "abcdefghijklmnop", Iterations: 600000, Check: "encv1:test"},
	}
	if err := bookInput.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", bookInput)
	if err != nil {
		t.Fatal(err)
	}
	crypto, _ := json.Marshal(bookInput.Crypto)
	input := domain.NoteInput{Kind: domain.KindText, CipherVersion: domain.EncryptedNoteWriteVersion, ExpectedCrypto: crypto, ContentCipher: "encv1:AAAA", Data: json.RawMessage(`{}`)}
	if err := input.Normalize(true); err != nil {
		t.Fatal(err)
	}
	note, err := st.CreateNote(ctx, book.ID, "ada", input)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := db.MarkWriteTransaction(ctx, tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO note_links(tenant,note_id,target_kind,target_id,label) VALUES('acme',$1,'note','old','secret')`, note.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, format := range []domain.FormatoExport{domain.FormatoHTML, domain.FormatoNativo, domain.FormatoMarkdown, domain.FormatoMarkdownLargo, domain.FormatoTextoPlano, domain.FormatoPDF} {
		if got, notes, err := st.ExportNotebook(ctx, book.ID, "ada", format); !errors.Is(err, domain.ErrEncryptedMigrationRequired) || got.ID != uuid.Nil || notes != nil {
			t.Fatalf("unsafe %s export returned data: %+v %+v %v", format, got, notes, err)
		}
	}
	original, err := st.ListNotes(ctx, book.ID, "ada")
	if err != nil || len(original) != 1 || len(original[0].Links) != 1 || original[0].Links[0].Label != "secret" {
		t.Fatalf("source was lost: %+v %v", original, err)
	}
	audit, err := st.ListAudit(ctx, FiltroAudit{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range audit {
		if row.Action == domain.AuditNotebookExport {
			t.Fatal("a rejected export was audited as successful")
		}
	}
	before, err := st.ListNotebooks(ctx, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := st.ImportNotebook(ctx, "ada", book, original); !errors.Is(err, domain.ErrEncryptedMigrationRequired) {
		t.Fatalf("unsafe backup imported: %v", err)
	}
	after, err := st.ListNotebooks(ctx, "ada")
	if err != nil || len(after) != len(before) {
		t.Fatalf("rejected import created a notebook: %d -> %d, %v", len(before), len(after), err)
	}
}

func TestCleanEncryptedBackupFormatsDoNotReportIncompleteExports(t *testing.T) {
	_, st, ctx, _, _ := exportFixture(t)
	input := domain.NotebookInput{
		Title: "encrypted", Visibility: domain.VisPersonal, Encrypted: true,
		Crypto: &domain.CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Salt: "abcdefghijklmnop", Iterations: 600000, Check: "encv1:test"},
	}
	if err := input.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", input)
	if err != nil {
		t.Fatal(err)
	}
	crypto, _ := json.Marshal(input.Crypto)
	noteInput := domain.NoteInput{Kind: domain.KindText, CipherVersion: domain.EncryptedNoteWriteVersion, ExpectedCrypto: crypto, ContentCipher: "encv1:AAAA", Data: json.RawMessage(`{}`)}
	if err := noteInput.Normalize(true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateNote(ctx, book.ID, "ada", noteInput); err != nil {
		t.Fatal(err)
	}
	for _, format := range []domain.FormatoExport{domain.FormatoMarkdown, domain.FormatoMarkdownLargo, domain.FormatoTextoPlano, domain.FormatoPDF} {
		if got, notes, err := st.ExportNotebook(ctx, book.ID, "ada", format); !errors.Is(err, domain.ErrEncryptedMigrationRequired) || got.ID != uuid.Nil || notes != nil {
			t.Fatalf("incomplete %s returned: %+v %+v %v", format, got, notes, err)
		}
	}
	for _, format := range []domain.FormatoExport{domain.FormatoHTML, domain.FormatoNativo} {
		got, notes, err := st.ExportNotebook(ctx, book.ID, "ada", format)
		if err != nil || got.ID != book.ID || len(notes) != 1 || notes[0].ContentCipher != "encv1:AAAA" {
			t.Fatalf("clean %s export failed: %+v %+v %v", format, got, notes, err)
		}
	}
}
