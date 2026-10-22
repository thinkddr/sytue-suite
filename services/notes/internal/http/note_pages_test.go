// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func TestNotePageExactBoundUsesLastDeliveredCursorAndPreservesRawData(t *testing.T) {
	page := store.NotePage{}
	for range 3 {
		page.Notes = append(page.Notes, domain.Note{ID: uuid.New(), DataJSON: `{"n":9007199254740993,"text":"` + strings.Repeat("<>&\"", 40) + `"}`})
	}
	want := store.NotePage{Notes: page.Notes[:2], NextCursor: page.Notes[1].ID.String()}
	limit, err := httpx.EncodeJSONLimit(want, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	body, err := encodeNotePage(page, true, len(limit))
	if err != nil || len(body) > len(limit) {
		t.Fatalf("page escaped exact bound: %d %v", len(body), err)
	}
	var got store.NotePage
	if json.Unmarshal(body, &got) != nil || len(got.Notes) != 2 || got.NextCursor != want.NextCursor || got.Notes[0].DataJSON != page.Notes[0].DataJSON {
		t.Fatalf("cursor skipped undelivered data or raw data changed: %s", body)
	}
	if _, err := encodeNotePage(page, false, len(limit)); !errors.Is(err, domain.ErrNotePageTooLarge) {
		t.Fatal("legacy reader received a silently truncated list")
	}
}

func TestIndivisibleNoteReturnsErrorBeforeAnySuccessHeaders(t *testing.T) {
	page := store.NotePage{Notes: []domain.Note{{ID: uuid.New(), ContentCipher: "encv1:" + strings.Repeat("A", domain.MaxNotePageBytes)}}}
	h := &Handlers{}
	w := httptest.NewRecorder()
	h.writeNotePage(w, httptest.NewRequest(http.MethodGet, "/fixture", http.NoBody), page, true)
	if w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), "note_page_too_large") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("oversized note emitted a partial success response: %d %.300s", w.Code, w.Body.String())
	}
}

func TestBoundedNotePageHasDistinctCapabilityRoute(t *testing.T) {
	router := NewRouter(Deps{Store: &store.Store{}}).(chi.Router)
	if !router.Match(chi.NewRouteContext(), http.MethodGet, "/api/v1/notebooks/"+uuid.NewString()+"/notes/page") {
		t.Fatal("missing bounded page capability route")
	}
}
