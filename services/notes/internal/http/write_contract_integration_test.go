// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package noteshttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func TestUnsupportedBackendWriteContractReturns503WithoutChangingContent(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := authn.Into(tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Subject: "ada"})
	bookIn := domain.NotebookInput{Title: "Original", Visibility: domain.VisPersonal}
	if err := bookIn.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", bookIn)
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Title: "preserved", Content: "original"})
	if err != nil {
		t.Fatal(err)
	}
	// A future schema may require a newer binary. Never adopt its requested
	// capability from the database; the current binary must remain fixed at 2.
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config($1,'3',true)`, db.WriteVersionSetting); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE notes SET min_write_version=3 WHERE id=$1`, note.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{store: st}
	router := NewRouter(Deps{Store: st})
	router.(chi.Router).Put("/_fixture/{id}", handlers.UpdateNote)
	body, _ := json.Marshal(domain.NoteInput{Title: "new", Content: "overwrite"})
	req := httptest.NewRequest(http.MethodPut, "/_fixture/"+note.ID.String(), bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "notes_backend_upgrade_required") {
		t.Fatalf("unsupported writer did not return an actionable rejection: %d %s", w.Code, w.Body.String())
	}
	for _, private := range []string{"min_write_version", "notes_write_contract", "SQLSTATE", "notes_require_write_contract"} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatalf("HTTP error exposed database internals: %s", w.Body.String())
		}
	}
	got, err := st.ListNotes(ctx, book.ID, "ada")
	if err != nil || len(got) != 1 || got[0].Title != "preserved" || got[0].Content != "original" {
		t.Fatalf("rejected HTTP write changed the original: %+v %v", got, err)
	}
}
