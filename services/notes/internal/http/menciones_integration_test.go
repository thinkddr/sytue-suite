// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package noteshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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

// Mencionar a alguien en una nota (`@bob@acme.test`) le avisa UNA vez —al aparecer la
// mención, no en cada guardado— y solo si puede leer la nota: ni a quien no ve el
// cuaderno, ni de una nota privada, ni a una misma.
func TestMencionarAvisaUnaVezYSoloAQuienLeeLaNota(t *testing.T) {
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
	// Bob lee el cuaderno; Carol es de la organización y no lo ve.
	if _, _, err := st.CompartirCuaderno(ctx, nb.ID, "ada", "bob", false); err != nil {
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
	// Sytue ID resuelve por correo a los tres de la organización, y a nadie más.
	id := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Email string `json:"email"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		quien, _, _ := strings.Cut(q.Email, "@")
		if !strings.HasSuffix(q.Email, "@acme.test") || !slices.Contains([]string{"ada", "bob", "carol"}, quien) ||
			r.Header.Get("Authorization") != "Bearer token-de-ada" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"user_id":"` + quien + `","email":"` + q.Email + `"}`))
	}))
	defer id.Close()
	h := &Handlers{store: st, hub: hubnotify.New(hub.URL, "s")}
	h.cfg.PublicURL = "https://notes.sytue.eu"
	h.cfg.IDBaseURL = id.URL

	pide := func(fn http.HandlerFunc, metodo, recurso, cuerpo string) {
		t.Helper()
		req := httptest.NewRequest(metodo, "/", strings.NewReader(cuerpo))
		req.Header.Set("Authorization", "Bearer token-de-ada")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", recurso)
		c := authn.Into(ctx, authn.Claims{Subject: "ada", Name: "Ada Lovelace"})
		w := httptest.NewRecorder()
		fn(w, req.WithContext(context.WithValue(c, chi.RouteCtxKey, rctx)))
		if w.Code >= 300 {
			t.Fatalf("%s %s: %d %s", metodo, cuerpo, w.Code, w.Body.String())
		}
	}
	guarda := func(contenido string) {
		t.Helper()
		pide(h.UpdateNote, http.MethodPut, nota.ID.String(), `{"title":"Billetes","content":"<p>`+contenido+`</p>"}`)
	}
	espera := func(sujeto, enlace string) {
		t.Helper()
		select {
		case n := <-avisos:
			if n.Subject != sujeto || n.Tenant != "acme" || n.Source != "notes" || n.Type != "notes.mention" || n.URL != enlace {
				t.Fatalf("aviso = %+v, se esperaba a %s con %s", n, sujeto, enlace)
			}
			if n.Params["actor"] != "Ada Lovelace" || n.Params["name"] != "Billetes" {
				t.Fatalf("variables del aviso: %+v", n.Params)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("mencionar a %s no le avisa", sujeto)
		}
	}
	nada := func(motivo string) {
		t.Helper()
		select {
		case n := <-avisos:
			t.Fatalf("aviso de más (%s): %+v", motivo, n)
		case <-time.After(300 * time.Millisecond):
		}
	}
	enCuaderno := hubnotify.EnlaceNota("https://notes.sytue.eu", "acme", nb.ID.String(), nota.ID.String())

	// A Bob sí; a Carol (no ve el cuaderno), a una misma y a quien no existe, no.
	guarda("Míralo @bob@acme.test, @carol@acme.test, @ada@acme.test y @nadie@otra.test")
	espera("bob", enCuaderno)
	nada("solo Bob lee la nota")
	// El guardado siguiente lleva la misma mención: no es nueva.
	guarda("Míralo @bob@acme.test, @carol@acme.test, @ada@acme.test y @nadie@otra.test. Gracias")
	nada("la mención ya estaba")

	// Carol recibe la nota suelta: al volver a mencionarla, su aviso abre «Compartido conmigo».
	guarda("Míralo @bob@acme.test")
	if _, _, err := st.CompartirNota(ctx, nota.ID, "ada", "carol", false); err != nil {
		t.Fatal(err)
	}
	guarda("Míralo @bob@acme.test y @carol@acme.test")
	espera("carol", hubnotify.EnlaceNotaCompartida("https://notes.sytue.eu", "acme", nota.ID.String()))
	nada("solo la mención nueva")

	// Una nota PRIVADA de Ada no se le nombra a Bob aunque lea el cuaderno; una nota
	// nueva corriente, sí.
	pide(h.CreateNote, http.MethodPost, nb.ID.String(), `{"kind":"text","title":"Billetes","content":"<p>@bob@acme.test</p>","visibility":"private"}`)
	nada("la nota es privada")
	pide(h.CreateNote, http.MethodPost, nb.ID.String(), `{"kind":"text","title":"Billetes","content":"<p>@bob@acme.test</p>"}`)
	select {
	case n := <-avisos:
		if n.Subject != "bob" || n.Type != "notes.mention" || !strings.Contains(n.URL, "notebook="+nb.ID.String()) {
			t.Fatalf("aviso de la nota nueva = %+v", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("mencionar en una nota nueva no avisa")
	}
}
