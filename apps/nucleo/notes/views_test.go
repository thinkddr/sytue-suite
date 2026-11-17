// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestPrivateViewsRecoverAuxiliaryRowsWithoutHTTP(t *testing.T) {
	r, _, source := notesFixture(t)
	source.rows["note/note"] = []byte(`{"id":"note","owner_sub":"alice","notebook_id":"book","kind":"todo","width":260,"pinned":true,"title":"#Reunión","content":"<p>#reunion #plan</p>","updated_at":"2026-10-29T10:00:00Z"}`)
	source.rows["note/other"] = []byte(`{"id":"other","owner_sub":"alice","notebook_id":"book","kind":"text","title":"#planes","content":"#Reunión","updated_at":"2026-10-28T10:00:00Z"}`)
	source.rows["note/hidden"] = []byte(`{"id":"hidden","owner_sub":"alice","notebook_id":"book","kind":"text","indexable":false,"title":"#oculto #plan","updated_at":"2026-10-27T10:00:00Z"}`)
	source.rows["tag/priority"] = []byte(`{"id":"priority","owner_sub":"alice","name":"Prioridad","position":1}`)
	source.rows["tag_state/low"] = []byte(`{"id":"low","tag_id":"priority","name":"Baja","color":"#123456","position":2}`)
	source.rows["tag_state/high"] = []byte(`{"id":"high","tag_id":"priority","name":"Alta","color":"#654321","position":1}`)
	source.rows["note_link/backlink"] = []byte(`{"id":"backlink","note_id":"note","target_kind":"note","target_id":"other","label":"Referencia"}`)
	source.rows["reminder/active"] = []byte(`{"note_id":"note","owner_sub":"alice","due_at":"2026-10-31T12:00:00Z","tzid":"UTC","rrule":"FREQ=DAILY","fired_at":"2026-10-30T12:00:00Z"}`)
	source.rows["reminder/fired"] = []byte(`{"note_id":"other","owner_sub":"alice","due_at":"2026-10-30T11:00:00Z","tzid":"UTC","fired_at":"2026-10-30T12:00:00Z"}`)
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	pages, opens := source.pages, source.opens
	r.Close()
	recovered, err := New(r.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if err := recovered.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	private, err := api.NewNotes(server.URL, nil, server.Client()).WithPrivateData(recovered)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := private.PinnedNotes(t.Context())
	if err != nil || len(pinned) != 1 || pinned[0].ID != "note" {
		t.Fatal("private pinned", pinned, err)
	}
	tags, err := private.ListTags(t.Context())
	if err != nil || len(tags) != 1 || len(tags[0].States) != 2 || tags[0].States[0].ID != "high" {
		t.Fatal("private tag join", tags, err)
	}
	links, err := private.LinkedNotes(t.Context(), "note", "other", 100)
	if err != nil || len(links) != 1 || links[0].ID != "note" {
		t.Fatal("retained inverse link", links, err)
	}
	reminders, err := private.ListReminders(t.Context())
	if err != nil || len(reminders) != 1 || reminders[0].NoteID != "note" || reminders[0].NotebookID != "book" {
		t.Fatal("repeating task reminder", reminders, err)
	}
	hashtags, err := private.Hashtags(t.Context())
	if err != nil || !reflect.DeepEqual(hashtags, []api.Hashtag{{Tag: "Reunión", Count: 2}, {Tag: "plan", Count: 1}, {Tag: "planes", Count: 1}}) {
		t.Fatal("hashtags count visible notes once", hashtags, err)
	}
	hits, err := private.Search(t.Context(), "#plan", 100, false)
	if err != nil || len(hits) != 1 || hits[0].ID != "note" {
		t.Fatal("exact hashtag query", hits, err)
	}
	if hits, err := private.Search(t.Context(), "oculto", 100, false); err != nil || len(hits) != 0 {
		t.Fatal("index optout", hits, err)
	}
	if source.pages != pages || source.opens != opens || calls.Load() != 0 {
		t.Fatal("private views reread a plaintext source", calls.Load())
	}
	// Editing a migrated note can explicitly remove its inherited links. A
	// retained source row must never resurrect that link in the reverse view.
	all, err := private.ListNotes(t.Context(), "book")
	if err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(all, func(n api.Note) bool { return n.ID == "note" })
	if at < 0 || len(all[at].Links) != 1 {
		t.Fatal("source links missing from editor", all)
	}
	input := all[at].Input()
	input.Links = nil
	if _, err := private.UpdateNote(t.Context(), "note", "book", input); err != nil {
		t.Fatal(err)
	}
	if links, err := private.LinkedNotes(t.Context(), "note", "other", 100); err != nil || len(links) != 0 {
		t.Fatal("removed link resurrected", links, err)
	}
	// Returned slices are independent of the authenticated catalog.
	tags[0].States[0].Name = "tampered view"
	again, err := private.ListTags(t.Context())
	if err != nil || again[0].States[0].Name != "Alta" {
		t.Fatal("view mutated retained state", err)
	}
}

func TestPrivateViewsExcludeArchivedDeletedAndCancelledLifetimes(t *testing.T) {
	r, _, source := notesFixture(t)
	source.rows["notebook/book"] = []byte(`{"id":"book","owner_sub":"alice","title":"Archive","indexable":true,"archived_at":"2026-10-30T10:00:00Z"}`)
	source.rows["note/note"] = []byte(`{"id":"note","owner_sub":"alice","notebook_id":"book","kind":"text","pinned":true,"title":"#archive"}`)
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	if v, err := r.PinnedNotes(t.Context()); err != nil || len(v) != 0 {
		t.Fatal(v, err)
	}
	if v, err := r.Hashtags(t.Context()); err != nil || len(v) != 0 {
		t.Fatal(v, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	checks := []func() error{
		func() error { _, e := r.PinnedNotes(ctx); return e },
		func() error { _, e := r.Hashtags(ctx); return e },
		func() error { _, e := r.ListTags(ctx); return e },
		func() error { _, e := r.ListReminders(ctx); return e },
		func() error { _, e := r.LinkedNotes(ctx, "note", "note", 100); return e },
	}
	for _, check := range checks {
		if err := check(); !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled view", err)
		}
	}
}

func TestPrivateTagViewRejectsOrphanedState(t *testing.T) {
	r, _, source := notesFixture(t)
	source.rows["tag_state/orphan"] = []byte(`{"id":"orphan","tag_id":"missing","name":"lost"}`)
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ListTags(t.Context()); !errors.Is(err, ErrSource) {
		t.Fatal("orphan accepted", err)
	}
}

func TestPrivateSearchRespectsNoteIndexOverride(t *testing.T) {
	r, _, source := notesFixture(t)
	var book map[string]json.RawMessage
	if err := json.Unmarshal(source.rows["notebook/book"], &book); err != nil {
		t.Fatal(err)
	}
	book["indexable"] = json.RawMessage("false")
	raw, err := json.Marshal(book)
	if err != nil {
		t.Fatal(err)
	}
	source.rows["notebook/book"] = raw
	source.rows["note/override"] = []byte(`{"id":"override","owner_sub":"alice","notebook_id":"book","kind":"text","indexable":true,"title":"#visible"}`)
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	hits, err := r.Search(t.Context(), "", 100, false)
	if err != nil || len(hits) != 1 || !slices.ContainsFunc(hits, func(h api.NoteHit) bool { return h.ID == "override" }) {
		t.Fatal("explicit index override", hits, err)
	}
}
