// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package noteshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func TestEncryptedMigrationPassesBothHTTPSizeLimitsAndReturnsCheckedConflicts(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Subject: "ada"})
	bookIn := domain.NotebookInput{Title: "Private", Visibility: domain.VisPersonal}
	if err := bookIn.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", bookIn)
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Title: "legacy", Data: json.RawMessage(`{"attachments":[{"dataUrl":"private-image"}]}`)})
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
		CipherVersion: 2, ExpectedUpdatedAt: note.UpdatedAt, ExpectedCrypto: crypto,
		ContentCipher: "encv1:" + strings.Repeat("A", 2<<20),
	}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{store: st}
	// Register a fixture route on the real router to exercise its outer body
	// middleware as well as DecodeJSONLimit. Authorization is provided explicitly;
	// the store's real tenant/person predicates still execute against PostgreSQL.
	router := NewRouter(Deps{Store: st})
	router.(chi.Router).Post("/_fixture/{id}", h.MigrateEncryptedContent)
	request := func(data []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/_fixture/"+note.ID.String(), bytes.NewReader(data)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if response := request(body); response.Code != http.StatusOK {
		t.Fatalf("sealed attachment could not pass both HTTP limits: %d %.300s", response.Code, response.Body.String())
	}
	if response := request(body); response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "note_changed") {
		t.Fatalf("an ambiguous completed write was not reported as a conflict: %d %.300s", response.Code, response.Body.String())
	}
	if response := request(bytes.Repeat([]byte(" "), int(domain.MaxNoteRequestBody)+1)); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "body_too_large") {
		t.Fatalf("oversized request bypassed bounded decoding: %d %.300s", response.Code, response.Body.String())
	}
}

func TestNotePageRawDataPreservesJSONNumbersAndDoesNotExpandHTML(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Subject: "ada"})
	bookIn := domain.NotebookInput{Title: "Private", Visibility: domain.VisPersonal}
	if err := bookIn.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", bookIn)
	if err != nil {
		t.Fatal(err)
	}
	original := `{"integer":9007199254740993,"decimal":0.1234567890123456789,"text":"<private>"}`
	if _, err := st.CreateNote(ctx, book.ID, "ada", domain.NoteInput{Data: json.RawMessage(original)}); err != nil {
		t.Fatal(err)
	}
	h := &Handlers{store: st}
	router := NewRouter(Deps{Store: st})
	router.(chi.Router).Get("/_fixture/{id}", h.ListNotes)
	request := func(query string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/_fixture/"+book.ID.String()+query, http.NoBody).WithContext(ctx)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	w := request("?limit=1&raw_data=1")
	var page struct {
		Notes []domain.Note `json:"notes"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Notes) != 1 {
		t.Fatalf("raw page: %d %.300s", w.Code, w.Body.String())
	}
	n := page.Notes[0]
	if len(n.Data) != 0 || !strings.Contains(n.DataJSON, "9007199254740993") || !strings.Contains(n.DataJSON, "0.1234567890123456789") || !strings.Contains(w.Body.String(), "<private>") {
		t.Fatalf("raw data was rounded, duplicated or HTML-expanded: %.400s", w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("private JSON response headers missing")
	}
	for _, query := range []string{"?cursor=bad", "?limit=0", "?limit=101", "?limit=1&cursor=bad", "?limit=1&raw_data=yes"} {
		if w := request(query); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid page accepted: %s %d", query, w.Code)
		}
	}
	w = request("")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"data":{`) || strings.Contains(w.Body.String(), `"data_json"`) {
		t.Fatalf("legacy list contract changed: %.300s", w.Body.String())
	}
}

func TestVersionedEncryptedWritesEnforceNotebookAndFullCrypto(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	tenant := tenancy.Tenant{Slug: "acme"}
	ctx := authn.Into(tenancy.Into(context.Background(), tenant), authn.Claims{Subject: "ada"})
	in := domain.NotebookInput{
		Title: "Private", Visibility: domain.VisPersonal, Encrypted: true,
		Crypto: &domain.CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Salt: strings.Repeat("a", 24), Iterations: 600000, Check: "encv1:fixture"},
	}
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, "ada", in)
	if err != nil {
		t.Fatal(err)
	}
	plainInput := domain.NotebookInput{Title: "Plain", Visibility: domain.VisPersonal}
	if err := plainInput.Normalize(); err != nil {
		t.Fatal(err)
	}
	plain, err := st.CreateNotebook(ctx, "ada", plainInput)
	if err != nil {
		t.Fatal(err)
	}
	crypto, err := json.Marshal(in.Crypto)
	if err != nil {
		t.Fatal(err)
	}
	body := domain.NoteInput{CipherVersion: 2, ExpectedCrypto: crypto, ContentCipher: "encv1:fixture-note", Data: json.RawMessage(`{}`)}
	h := &Handlers{store: st}
	router := NewRouter(Deps{Store: st})
	router.(chi.Router).Post("/_new/{id}", h.CreateEncryptedNote)
	router.(chi.Router).Put("/_new/{id}", h.UpdateEncryptedNote)
	request := func(method, id string, scope context.Context, input domain.NoteInput) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, "/_new/"+id, bytes.NewReader(raw)).WithContext(scope)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	response := request(http.MethodPost, book.ID.String(), ctx, body)
	if response.Code != http.StatusCreated {
		t.Fatalf("new encrypted create failed: %d %s", response.Code, response.Body.String())
	}
	var note domain.Note
	if err := json.Unmarshal(response.Body.Bytes(), &note); err != nil {
		t.Fatal(err)
	}
	body.ContentCipher = "encv1:updated-note"
	if response := request(http.MethodPut, note.ID.String(), ctx, body); response.Code != http.StatusOK {
		t.Fatalf("new encrypted update failed: %d %s", response.Code, response.Body.String())
	}
	stale := body
	stale.ExpectedCrypto = json.RawMessage(strings.Replace(string(crypto), strings.Repeat("a", 24), strings.Repeat("b", 24), 1))
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		id := book.ID.String()
		if method == http.MethodPut {
			id = note.ID.String()
		}
		response := request(method, id, ctx, stale)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "notebook_crypto_changed") {
			t.Fatalf("new %s ignored full crypto CAS: %d %s", method, response.Code, response.Body.String())
		}
		otherPerson := authn.Into(ctx, authn.Claims{Subject: "mallory"})
		otherTenant := tenancy.Into(ctx, tenancy.Tenant{Slug: "beta"})
		for _, scope := range []context.Context{otherPerson, otherTenant} {
			response := request(method, id, scope, body)
			if response.Code != http.StatusNotFound {
				t.Fatalf("new %s crossed person/tenant: %d %s", method, response.Code, response.Body.String())
			}
		}
	}
	// The new paths cannot act as ordinary plaintext writers, including an empty
	// notebook whose public crypto changed before this request arrived.
	plaintext := domain.NoteInput{CipherVersion: 2, Title: "must not be created", Data: json.RawMessage(`{}`)}
	response = request(http.MethodPost, plain.ID.String(), ctx, plaintext)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "notebook_crypto_changed") {
		t.Fatalf("versioned route accepted a plaintext notebook: %d %s", response.Code, response.Body.String())
	}
	plainNote, err := st.CreateNote(ctx, plain.ID, "ada", domain.NoteInput{Title: "original", Data: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	response = request(http.MethodPut, plainNote.ID.String(), ctx, plaintext)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "notebook_crypto_changed") {
		t.Fatalf("versioned route edited a plaintext note: %d %s", response.Code, response.Body.String())
	}
	for _, test := range []struct {
		book      domain.Notebook
		encrypted bool
	}{{book, true}, {plain, false}} {
		saved, err := st.ListNotes(ctx, test.book.ID, "ada")
		if err != nil {
			t.Fatal(err)
		}
		if len(saved) != 1 || (test.encrypted && saved[0].ContentCipher != "encv1:updated-note") || (!test.encrypted && saved[0].Title != "original") {
			t.Fatalf("rejected writes changed original or created another note: %+v", saved)
		}
	}
}
