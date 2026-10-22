// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// Compartir un álbum con una persona le avisa UNA vez, con su enlace y quién; un `sub`
// que Sytue ID no reconoce en la organización no se guarda; se comparte solo para ver;
// y lo compartido lo abre quien lo recibió y nadie más, también sus fotos.
func TestCompartirUnAlbumConUnaPersona(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	a, err := st.Ingerir(ctx, domain.Activo{
		Duenyo: "ada", DriveID: "d1", Nombre: "cala.jpg", Tipo: "image/jpeg",
		Bytes: 2048, Ancho: 4000, Alto: 3000, Orientacion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	al, _ := st.CrearAlbum(ctx, "ada", "Verano")
	if _, err := st.AnyadirAlAlbum(ctx, al.ID, "ada", []uuid.UUID{a.ID}); err != nil {
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
	id := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			UserID string `json:"user_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q.UserID != "bob" || r.Header.Get("Authorization") != "Bearer token" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"user_id":"bob","email":"bob@acme.test","name":"Bob"}`))
	}))
	defer id.Close()
	h := &Handlers{store: st, hub: hubnotify.New(hub.URL, "s"), avisados: hubnotify.NuevaUnaVez(time.Hour)}
	h.cfg.PublicURL = "https://photos.sytue.eu"
	h.cfg.IDBaseURL = id.URL

	pedir := func(fn http.HandlerFunc, quien, metodo, cuerpo string, params map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(metodo, "/", strings.NewReader(cuerpo))
		req.Header.Set("Authorization", "Bearer token")
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		c := authn.Into(ctx, authn.Claims{Subject: quien, Name: "Ada Lovelace"})
		req = req.WithContext(context.WithValue(c, chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		fn(w, req)
		return w
	}
	compartir := func(sujeto, cuerpo string) int {
		return pedir(h.CompartirConPersona, "ada", http.MethodPut, cuerpo, map[string]string{"album": al.ID.String(), "sub": sujeto}).Code
	}

	if c := compartir("intruso", `{}`); c != http.StatusNotFound {
		t.Fatalf("un sub fuera de la organización: %d", c)
	}
	if c := compartir("bob", `{"permission":"admin"}`); c != http.StatusBadRequest {
		t.Fatalf("un nivel que no existe: %d", c)
	}
	if c := compartir("bob", `{"permission":"read"}`); c != http.StatusOK {
		t.Fatalf("compartir: %d", c)
	}
	select {
	case n := <-avisos:
		if n.Subject != "bob" || n.Tenant != "acme" || n.Source != "photos" || n.Type != "photos.shared" ||
			n.URL != "https://photos.sytue.eu/photos/acme/shared?album="+al.ID.String() ||
			n.Params["actor"] != "Ada Lovelace" || n.Params["name"] != "Verano" {
			t.Fatalf("aviso = %+v", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("compartir no avisa a quien lo recibe")
	}
	if c := compartir("bob", `{}`); c != http.StatusOK {
		t.Fatalf("compartir otra vez: %d", c)
	}
	select {
	case n := <-avisos:
		t.Fatalf("volver a compartir avisa de nuevo: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}

	album := map[string]string{"album": al.ID.String()}
	variante := map[string]string{"album": al.ID.String(), "id": a.ID.String(), "variante": "miniatura"}
	if w := pedir(h.AlbumCompartido, "bob", http.MethodGet, "", album); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), a.ID.String()) || strings.Contains(w.Body.String(), "driveFileId") {
		t.Fatalf("bob abre el álbum: %d %s", w.Code, w.Body)
	}
	for _, quien := range []string{"carla", "ada"} {
		if w := pedir(h.AlbumCompartido, quien, http.MethodGet, "", album); w.Code != http.StatusNotFound {
			t.Errorf("%s abre como compartido un álbum que no le han dado: %d", quien, w.Code)
		}
		if w := pedir(h.VarianteCompartida, quien, http.MethodGet, "", variante); w.Code != http.StatusNotFound {
			t.Errorf("%s pide una foto de un álbum que no le han dado: %d", quien, w.Code)
		}
	}
	if w := pedir(h.PersonasDelAlbum, "bob", http.MethodGet, "", album); w.Code != http.StatusNotFound {
		t.Errorf("bob ve con quién está compartido: %d", w.Code)
	}
	if w := pedir(h.DejarDeCompartirConPersona, "ada", http.MethodDelete, "", map[string]string{"album": al.ID.String(), "sub": "bob"}); w.Code != http.StatusNoContent {
		t.Fatalf("retirar: %d", w.Code)
	}
	if w := pedir(h.AlbumCompartido, "bob", http.MethodGet, "", album); w.Code != http.StatusNotFound {
		t.Errorf("retirado, bob lo sigue abriendo: %d", w.Code)
	}
	// Retirar y volver a compartir no le vuelve a llenar la campana.
	if c := compartir("bob", `{}`); c != http.StatusOK {
		t.Fatalf("volver a compartir: %d", c)
	}
	select {
	case n := <-avisos:
		t.Fatalf("retirar y volver a compartir avisa otra vez: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestAjustesOrganizacion_soloAdminExcluyeDelLote(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	h := &Handlers{store: st}

	pedir := func(fn http.HandlerFunc, quien string, roles []string, metodo, cuerpo string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(metodo, "/api/v1/settings", strings.NewReader(cuerpo))
		req.Header.Set("Content-Type", "application/json")
		c := authn.Into(ctx, authn.Claims{Subject: quien, Roles: roles})
		req = req.WithContext(c)
		w := httptest.NewRecorder()
		fn(w, req)
		return w
	}

	if w := pedir(h.AjustesOrganizacion, "bob", []string{"member"}, http.MethodGet, ""); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"excludeAiBatch":false`) ||
		!strings.Contains(w.Body.String(), `"canManageAiBatch":false`) {
		t.Fatalf("GET inicial miembro: %d %s", w.Code, w.Body)
	}
	if w := pedir(h.GuardarAjustesOrganizacion, "bob", []string{"member"}, http.MethodPut, `{"excludeAiBatch":true}`); w.Code != http.StatusForbidden {
		t.Fatalf("miembro no debe poder excluir la organización: %d %s", w.Code, w.Body)
	}
	if w := pedir(h.GuardarAjustesOrganizacion, "ada", []string{"admin"}, http.MethodPut, `{"excludeAiBatch":true}`); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"excludeAiBatch":true`) ||
		!strings.Contains(w.Body.String(), `"canManageAiBatch":true`) {
		t.Fatalf("admin excluye la organización: %d %s", w.Code, w.Body)
	}
	if w := pedir(h.AjustesOrganizacion, "bob", []string{"member"}, http.MethodGet, ""); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"excludeAiBatch":true`) ||
		!strings.Contains(w.Body.String(), `"canManageAiBatch":false`) {
		t.Fatalf("GET miembro tras excluir: %d %s", w.Code, w.Body)
	}
}
