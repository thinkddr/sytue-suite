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
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// Compartir un cuaderno o una nota avisa UNA vez a quien lo recibe, con su enlace y
// quién; subir el nivel no vuelve a avisar; y un `sub` que Sytue ID no reconoce en la
// organización no se guarda.
func TestCompartirConUnaPersonaAvisaYNoAdmiteDesconocidos(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	in := domain.NotebookInput{Title: "Viaje", Visibility: domain.VisPersonal}
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	nb, err := st.CreateNotebook(ctx, "ada", in)
	if err != nil {
		t.Fatal(err)
	}
	nota, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "Billetes"})
	if err != nil {
		t.Fatal(err)
	}

	avisos := make(chan hubnotify.Notification, 4)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		avisos <- n
		w.WriteHeader(http.StatusCreated)
	}))
	defer hub.Close()
	// Sytue ID: Bob es de la organización; nadie más.
	id := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			UserID string `json:"user_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q.UserID != "bob" || r.Header.Get("Authorization") != "Bearer token-de-ada" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"user_id":"bob","email":"bob@acme.test","name":"Bob"}`))
	}))
	defer id.Close()
	h := &Handlers{store: st, hub: hubnotify.New(hub.URL, "s")}
	h.cfg.PublicURL = "https://notes.sytue.eu"
	h.cfg.IDBaseURL = id.URL

	put := func(fn http.HandlerFunc, recurso, sujeto, permiso string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"permission":"`+permiso+`"}`))
		req.Header.Set("Authorization", "Bearer token-de-ada")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", recurso)
		rctx.URLParams.Add("sub", sujeto)
		c := authn.Into(ctx, authn.Claims{Subject: "ada", Name: "Ada Lovelace"})
		req = req.WithContext(context.WithValue(c, chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		fn(w, req)
		return w
	}
	espera := func(tipo, enlace string) {
		t.Helper()
		select {
		case n := <-avisos:
			if n.Subject != "bob" || n.Tenant != "acme" || n.Source != "notes" || n.Type != tipo || n.URL != enlace {
				t.Fatalf("aviso = %+v", n)
			}
			if n.Params["actor"] != "Ada Lovelace" || n.Params["name"] == "" {
				t.Fatalf("variables del aviso: %+v", n.Params)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("compartir no avisa a quien lo recibe")
		}
	}
	nada := func() {
		t.Helper()
		select {
		case n := <-avisos:
			t.Fatalf("aviso de más: %+v", n)
		case <-time.After(300 * time.Millisecond):
		}
	}

	if w := put(h.CompartirCuaderno, nb.ID.String(), "intruso", "read"); w.Code != http.StatusNotFound {
		t.Fatalf("un sub fuera de la organización: %d %s", w.Code, w.Body.String())
	}
	if w := put(h.CompartirCuaderno, nb.ID.String(), "bob", "admin"); w.Code != http.StatusBadRequest {
		t.Fatalf("un nivel que no existe: %d %s", w.Code, w.Body.String())
	}
	nada()
	if w := put(h.CompartirCuaderno, nb.ID.String(), "bob", "read"); w.Code != http.StatusOK {
		t.Fatalf("compartir el cuaderno: %d %s", w.Code, w.Body.String())
	}
	espera("notes.shared", "https://notes.sytue.eu/notes/acme/?notebook="+nb.ID.String())
	if w := put(h.CompartirCuaderno, nb.ID.String(), "bob", "write"); w.Code != http.StatusOK {
		t.Fatalf("dar edición: %d %s", w.Code, w.Body.String())
	}
	nada()
	if w := put(h.CompartirNota, nota.ID.String(), "bob", "write"); w.Code != http.StatusOK {
		t.Fatalf("compartir la nota: %d %s", w.Code, w.Body.String())
	}
	espera("notes.shared", "https://notes.sytue.eu/notes/acme/?shared="+nota.ID.String())
	if ps, _ := st.PersonasDeCuaderno(ctx, nb.ID, "ada"); len(ps) != 1 || ps[0].Sujeto != "bob" {
		t.Fatalf("el intruso se guardó: %+v", ps)
	}
	// Sin el cuaderno, quien tiene la nota por compartida con edición la guarda por la
	// ruta de siempre —aunque no pueda leer su cuaderno— y solo cambia su contenido
	// (no la fija); quien no la tiene, no la toca.
	if err := st.DejarDeCompartirCuaderno(ctx, nb.ID, "ada", "bob"); err != nil {
		t.Fatal(err)
	}
	editar := func(quien string) int {
		req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(
			`{"title":"Billetes de tren","content":"<p>AVE</p>","notebook_id":"`+nb.ID.String()+`","pinned":true}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", nota.ID.String())
		c := authn.Into(ctx, authn.Claims{Subject: quien})
		req = req.WithContext(context.WithValue(c, chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		h.UpdateNote(w, req)
		return w.Code
	}
	if c := editar("carla"); c != http.StatusNotFound {
		t.Fatalf("Carla edita una nota que no le han compartido: %d", c)
	}
	if c := editar("bob"); c != http.StatusOK {
		t.Fatalf("Bob no guarda la nota compartida con edición: %d", c)
	}
	if ns, _ := st.NotasCompartidas(ctx, "bob"); len(ns) != 1 || ns[0].Title != "Billetes de tren" || ns[0].Pinned {
		t.Fatalf("la edición de Bob: %+v", ns)
	}

	// Y el tipo no se cambia para saltarse el saneado: con `link` el HTML no se sanea, y
	// acabaría pintado como texto en el navegador de Ada.
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(
		`{"kind":"link","content":"<p>hola</p><img src=x onerror=alert(1)>","notebook_id":"`+nb.ID.String()+`"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", nota.ID.String())
	req = req.WithContext(context.WithValue(authn.Into(ctx, authn.Claims{Subject: "bob"}), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.UpdateNote(w, req)
	ns, _ := st.NotasCompartidas(ctx, "bob")
	if w.Code != http.StatusOK || len(ns) != 1 || ns[0].Kind != domain.KindText ||
		strings.Contains(ns[0].Content, "onerror") || !strings.Contains(ns[0].Content, "hola") {
		t.Fatalf("cambiar el tipo cuela HTML sin sanear: %d %+v", w.Code, ns)
	}
}
