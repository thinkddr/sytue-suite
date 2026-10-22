// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package noteshttp

import (
	"context"
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

func TestNoteReadRoutesBatchSmallNotesAndNeverTruncateLegacySilently(t *testing.T) {
	pg := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(pg.Pool)
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Subject: "ada"})
	in := domain.NotebookInput{Title: "Small", Visibility: domain.VisPersonal}
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", in)
	if err != nil {
		t.Fatal(err)
	}
	insert := func(count int) {
		t.Helper()
		if err := pg.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if err := db.MarkWriteTransaction(ctx, tx); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO notes (tenant,notebook_id,owner_sub,title,data) SELECT 'acme',$1,'ada','small','{"n":9007199254740993,"x":"<>&"}'::jsonb FROM generate_series(1,$2)`, book.ID, count)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	insert(150)
	h := &Handlers{store: st}
	router := chi.NewRouter()
	router.Get("/fixture/{id}/notes/page", h.ListNotesPage)
	router.Get("/fixture/{id}/notes", h.ListNotes)
	request := func(suffix string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/fixture/"+book.ID.String()+"/notes"+suffix, http.NoBody).WithContext(ctx)
		router.ServeHTTP(w, r)
		return w
	}
	for _, suffix := range []string{"/page?limit=100&raw_data=1", "?limit=100&raw_data=1"} {
		w := request(suffix)
		var page store.NotePage
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Notes) != 100 || page.NextCursor != page.Notes[99].ID.String() || !strings.Contains(page.Notes[0].DataJSON, "9007199254740993") || strings.Contains(w.Body.String(), `\u003c`) {
			t.Fatalf("page route lost raw data, cursor, or bound: %s %d %.300s", suffix, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing bounded response headers")
		}
	}
	w := request("")
	var page store.NotePage
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Notes) != 150 || page.NextCursor != "" {
		t.Fatal("legacy small list no longer complete")
	}
	insert(851)
	w = request("")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "note_pagination_required") || strings.Contains(w.Body.String(), `"notes"`) {
		t.Fatalf("legacy large list silently truncated: %d %.300s", w.Code, w.Body.String())
	}
}
