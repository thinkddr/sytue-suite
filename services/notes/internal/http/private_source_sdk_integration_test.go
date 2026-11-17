// SPDX-License-Identifier: Apache-2.0
//go:build integration

package noteshttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

func TestPrivateNotesPostgresHTTPToSDKAndFreshDevice(t *testing.T) {
	upstream := os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE_SYNAPSE_URL")
	if upstream == "" {
		t.Skip("requires an isolated disposable remote Synapse")
	}
	consumer := os.Getenv("SYTUE_NOTES_SOURCE_FIXTURE_CONSUMER")
	if consumer != "" && consumer != "notepads" {
		t.Fatal("unknown explicitly selected Notes fixture consumer")
	}
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "notes"}
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant})
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	notebook := domain.NotebookInput{Title: "Retained source notebook", Layout: "list", Visibility: domain.VisPrivate}
	if err := notebook.Normalize(); err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNotebook(ctx, scope.Subject, notebook)
	if err != nil {
		t.Fatal(err)
	}
	data := json.RawMessage(`{"integer":900719925474099312345,"scientific":1.234567890123456789e+99,"padding":"` + strings.Repeat("private source sentinel / 漢字 ", 4096) + `"}`)
	note, err := st.CreateNote(ctx, book.ID, scope.Subject, domain.NoteInput{Kind: domain.KindText, Title: "Retained live note", Data: data, Links: []domain.NoteLink{{TargetKind: domain.TargetNote, TargetID: "retained-reference", Label: "exact relation"}}})
	if err != nil {
		t.Fatal(err)
	}
	trash, err := st.CreateNote(ctx, book.ID, scope.Subject, domain.NoteInput{Kind: domain.KindText, Title: "Retained trash"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteNote(ctx, trash.ID, scope.Subject); err != nil {
		t.Fatal(err)
	}
	input := domain.TagInput{Name: "Status", States: []domain.TagStateInput{{Name: "Open", Color: "#123456"}}}
	if err := input.Normalize(); err != nil {
		t.Fatal(err)
	}
	tag, err := st.SaveTag(ctx, scope.Subject, nil, input)
	if err != nil || len(tag.States) != 1 {
		t.Fatal("source tag failed", err)
	}
	if err := st.SetNoteState(ctx, note.ID, tag.ID, &tag.States[0].ID, scope.Subject); err != nil {
		t.Fatal(err)
	}
	reminder := domain.ReminderInput{DueAt: time.Now().Add(-time.Hour), TZID: "UTC"}
	if err := reminder.Normalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetReminder(ctx, note.ID, scope.Subject, reminder); err != nil {
		t.Fatal(err)
	}
	server := notesSDKSourceServer(t, st, scope)
	encoded, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	fixture := pgtest.NativeFixture{
		Module: "../../../../apps/nucleo", Package: "./matrix", Test: "TestPrivateNotesHTTPSourceAgainstSynapseAndFreshDevice", Marker: "NOTES_PG_HTTP_SDK_FRESH_PASS", Timeout: 4 * time.Minute,
		Env: map[string]string{"SYTUE_NOTES_SOURCE_FIXTURE": "1", "SYTUE_NOTES_SOURCE_FIXTURE_URL": server.URL, "SYTUE_NOTES_SOURCE_FIXTURE_SYNAPSE_URL": upstream, "SYTUE_NOTES_SOURCE_FIXTURE_SCOPE": string(encoded), "SYTUE_NOTES_SOURCE_FIXTURE_BOOK": book.ID.String(), "SYTUE_NOTES_SOURCE_FIXTURE_NOTE": note.ID.String()},
	}
	if consumer == "notepads" {
		fixture.Module = "../../../../apps/fyne"
		fixture.Package = "./cmd/notes"
		fixture.Test = "TestPrivateNotepadsApplicationAgainstPostgresAndFreshDevice"
		fixture.Marker = "NOTEPADS_NATIVE_PG_HTTP_SDK_FRESH_PASS"
		fixture.Timeout = 6 * time.Minute
	}
	pgtest.RunNativeFixture(ctx, t, fixture)
	state, err := st.PrivateStorageState(ctx, scope.Subject)
	if err != nil || state.Mode != "frozen" || state.Rows < 8 {
		t.Fatal("source was not completely retained", err)
	}
	if _, err := st.CreateNote(ctx, book.ID, scope.Subject, domain.NoteInput{Title: "old writer"}); !errors.Is(store.PrivateStorageError(err), store.ErrPrivateStorageFrozen) {
		t.Fatal("old writer escaped the actual SQL fence", err)
	}
	page, err := st.PrivateStorageSourcePage(ctx, scope.Subject, uuid.MustParse(state.Epoch), storage.LegacySourceCursor{}, storage.MaxLegacySourceRows)
	if err != nil || len(page.Rows) == 0 {
		t.Fatal("retained source disappeared after fresh recovery", err)
	}
	t.Log("NOTES_REAL_PG_HTTP_SDK_FRESH_PASS original rows, large JSON, trash and auxiliaries retained; old SQL writes rejected")
}

func notesSDKSourceServer(t *testing.T, st *store.Store, scope storage.Scope) *httptest.Server {
	t.Helper()
	handler := &Handlers{migration: st}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer synthetic-notes-source" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ctx := tenancy.Into(r.Context(), tenancy.Tenant{Slug: scope.Tenant})
			ctx = authn.Into(ctx, authn.Claims{Tenant: scope.Tenant, Subject: scope.Subject})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router.Route("/api/v1/private-storage/migration", func(r chi.Router) {
		r.Get("/", handler.PrivateStorageState)
		r.Post("/", handler.BeginPrivateStorage)
		r.Get("/{epoch}/rows", handler.PrivateStoragePage)
		r.Get("/{epoch}/source/{kind}/{source}", handler.PrivateStorageBody)
	})
	s := httptest.NewServer(router)
	t.Cleanup(s.Close)
	return s
}
