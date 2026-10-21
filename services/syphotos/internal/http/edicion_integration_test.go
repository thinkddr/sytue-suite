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
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/config"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/drive"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// Bob, con edición en el álbum de Ada: sus fotos se COPIAN al Drive de Ada (por la puerta
// interna, con el secreto) y en Photos pasan a ser de ella, sin su lugar; una foto ajena
// no entra; renombra sin ver el enlace de Ada; quita cualquier foto; a Ada le llega quién
// añadió qué; si no cabe, 507 claro; y al retirarle el permiso, lo copiado se queda.
// Carla, solo con ver, no toca nada.
func TestEditarUnAlbumCompartido(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	lat, lon := 43.3, -1.98
	foto := func(duenya, driveID string, conLugar bool) domain.Activo {
		a := domain.Activo{
			Duenyo: duenya, DriveID: driveID, Nombre: driveID + ".jpg", Tipo: "image/jpeg",
			Bytes: 2048, Ancho: 4000, Alto: 3000, Orientacion: 1,
		}
		if conLugar {
			a.Lat, a.Lon = &lat, &lon
		}
		f, err := st.Ingerir(ctx, a)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	deAda := foto("ada", "d-ada", false)
	deBob := foto("bob", "d-bob", true)
	llena := foto("bob", "d-lleno", false)
	otraDeBob := foto("bob", "d-bob2", false)
	sinOriginal := foto("bob", "d-borrado", false)
	deCarla := foto("carla", "d-carla", false)
	al, _ := st.CrearAlbum(ctx, "ada", "Viaje")
	if _, err := st.AnyadirAlAlbum(ctx, al.ID, "ada", []uuid.UUID{deAda.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CompartirAlbum(ctx, al.ID, "ada"); err != nil { // con enlace
		t.Fatal(err)
	}
	if _, _, err := st.CompartirConPersona(ctx, al.ID, "ada", "bob", true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompartirConPersona(ctx, al.ID, "ada", "carla", false); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var copias []map[string]string
	driveFalso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c map[string]string
		_ = json.NewDecoder(r.Body).Decode(&c)
		if r.URL.Path != "/internal/photos/copies" || r.Header.Get("Authorization") != "Bearer s3creto" || c["editor_sub"] != "bob" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		copias = append(copias, c)
		mu.Unlock()
		switch c["file_id"] {
		case "d-lleno":
			w.WriteHeader(http.StatusInsufficientStorage)
			return
		case "d-borrado":
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"file":{"id":"copia-de-` + c["file_id"] + `"},"reused":false}`))
	}))
	defer driveFalso.Close()
	avisos := make(chan hubnotify.Notification, 4)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		avisos <- n
		w.WriteHeader(http.StatusCreated)
	}))
	defer hub.Close()
	h := &Handlers{
		store: st, hub: hubnotify.New(hub.URL, "s"), drive: drive.Nuevo(driveFalso.URL, 0),
		avisados: hubnotify.NuevaUnaVez(time.Hour), anyadidos: hubnotify.NuevaUnaVez(time.Hour),
		cfg: config.Config{PublicURL: "https://photos.sytue.eu", HubInternalSecret: "s3creto"},
	}
	pedir := func(fn http.HandlerFunc, quien, metodo, cuerpo string, params map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(metodo, "/", strings.NewReader(cuerpo))
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		c := authn.Into(ctx, authn.Claims{Subject: quien, Name: strings.ToUpper(quien[:1]) + quien[1:]})
		req = req.WithContext(context.WithValue(c, chi.RouteCtxKey, rctx))
		w := httptest.NewRecorder()
		fn(w, req)
		return w
	}
	album := map[string]string{"album": al.ID.String()}
	anyadir := func(quien string, ids ...uuid.UUID) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]any{"ids": ids})
		return pedir(h.AnyadirAlAlbum, quien, http.MethodPost, string(b), album)
	}

	// Carla, solo con ver, no toca nada.
	if w := anyadir("carla", deCarla.ID); w.Code != http.StatusNotFound {
		t.Fatalf("Carla añade con solo ver: %d", w.Code)
	}
	if w := pedir(h.RenombrarAlbum, "carla", http.MethodPatch, `{"title":"Mío"}`, album); w.Code != http.StatusNotFound {
		t.Fatalf("Carla renombra: %d", w.Code)
	}

	// Bob añade la suya y, colada, una de Carla: solo entra la suya, copiada.
	if w := anyadir("bob", deBob.ID, deCarla.ID); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"added":1`) {
		t.Fatalf("Bob añade: %d %s", w.Code, w.Body)
	}
	mu.Lock()
	if len(copias) != 1 || copias[0]["tenant"] != "acme" || copias[0]["file_id"] != "d-bob" || copias[0]["owner_sub"] != "ada" {
		t.Fatalf("a Drive llegó %+v", copias)
	}
	mu.Unlock()
	fotos, err := st.FotosDelAlbum(ctx, al.ID, "ada")
	if err != nil || len(fotos) != 2 {
		t.Fatalf("fotos del álbum: %+v %v", fotos, err)
	}
	copia := fotos[1]
	if copia.Duenyo != "ada" || copia.DriveID != "copia-de-d-bob" || copia.Lat != nil || copia.Lon != nil {
		t.Errorf("la copia no es de Ada o lleva el lugar de Bob: %+v", copia)
	}
	select {
	case n := <-avisos:
		if n.Subject != "ada" || n.Type != "photos.album.added" || n.Params["actor"] != "Bob" ||
			n.Params["count"] != "1" || n.Params["name"] != "Viaje" ||
			n.URL != "https://photos.sytue.eu/photos/acme/album?id="+al.ID.String() {
			t.Fatalf("aviso a Ada = %+v", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a Ada no le llega quién añadió fotos")
	}

	// Renombra sin ver el enlace de Ada (y a ella se le dice), y quita una foto que no es suya.
	w := pedir(h.RenombrarAlbum, "bob", http.MethodPatch, `{"title":"Viaje a Donosti"}`, album)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `"shared"`) || !strings.Contains(w.Body.String(), "Donosti") {
		t.Fatalf("Bob renombra: %d %s", w.Code, w.Body)
	}
	select {
	case n := <-avisos:
		if n.Subject != "ada" || n.Type != "photos.album.renamed" || n.Params["name"] != "Viaje a Donosti" || n.Params["actor"] != "Bob" {
			t.Fatalf("aviso del renombrado = %+v", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a Ada no le llega que Bob le ha renombrado el álbum")
	}
	if w := pedir(h.RenombrarAlbum, "ada", http.MethodPatch, `{"title":"Viaje"}`, album); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"shared":true`) {
		t.Fatalf("la dueña renombra y ve su enlace: %d %s", w.Code, w.Body)
	}
	select {
	case n := <-avisos:
		t.Fatalf("la dueña se avisa a sí misma: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}
	if w := pedir(h.QuitarDelAlbum, "bob", http.MethodDelete, "", map[string]string{"album": al.ID.String(), "id": deAda.ID.String()}); w.Code != http.StatusNoContent {
		t.Fatalf("Bob quita la foto de Ada: %d", w.Code)
	}

	// Una foto cuyo original ya no está se salta; lo copiado antes de quedarse sin
	// espacio SÍ entra en el álbum (nada suelto en el Drive de Ada), y se dice con 507.
	w = anyadir("bob", otraDeBob.ID, sinOriginal.ID, llena.ID)
	if w.Code != http.StatusInsufficientStorage || !strings.Contains(w.Body.String(), "quota_exceeded") {
		t.Fatalf("sin espacio: %d %s", w.Code, w.Body)
	}
	if fs, _ := st.FotosDelAlbum(ctx, al.ID, "ada"); len(fs) != 2 || fs[1].DriveID != "copia-de-d-bob2" {
		t.Fatalf("lo copiado antes de quedarse sin espacio no entra: %+v", fs)
	}
	// Una tanda de más de MaxAnyadirEditora no se empieza.
	demasiadas := make([]uuid.UUID, MaxAnyadirEditora+1)
	for i := range demasiadas {
		demasiadas[i] = uuid.New()
	}
	if w := anyadir("bob", demasiadas...); w.Code != http.StatusBadRequest {
		t.Fatalf("una tanda enorme: %d", w.Code)
	}

	// Retirado el permiso, Bob ya no edita, y lo copiado se queda con Ada.
	if err := st.DejarDeCompartirConPersona(ctx, al.ID, "ada", "bob"); err != nil {
		t.Fatal(err)
	}
	if w := anyadir("bob", deBob.ID); w.Code != http.StatusNotFound {
		t.Fatalf("retirado, Bob añade: %d", w.Code)
	}
	if fs, _ := st.FotosDelAlbum(ctx, al.ID, "ada"); len(fs) != 2 || fs[0].ID != copia.ID {
		t.Fatalf("retirado, lo copiado no se queda con Ada: %+v", fs)
	}
}
