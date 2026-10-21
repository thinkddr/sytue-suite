// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/ingressroutes"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// El álbum compartido se abre también por `/api/v1/public/…`, que es lo que publica el
// ingress: las apps nativas no tienen el BFF de la web, que pregunta por `/public/…`
// dentro del clúster. Y SIN sesión, aunque cuelgue de `/api/v1`, cuyo resto la exige:
// se comprueba contra `/timeline` en la misma petición sin token.
func TestAlbumPublico_tambienPorElIngressYSinSesion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "t1"})
	a, err := st.Ingerir(ctx, domain.Activo{
		Duenyo: "u1", DriveID: "d1", Nombre: "cala.jpg", Tipo: "image/jpeg",
		Bytes: 2048, Ancho: 4000, Alto: 3000, Orientacion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	al, _ := st.CrearAlbum(ctx, "u1", "Verano")
	if _, err := st.AnyadirAlAlbum(ctx, al.ID, "u1", []uuid.UUID{a.ID}); err != nil {
		t.Fatal(err)
	}
	token, err := st.CompartirAlbum(ctx, al.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouter(Deps{Store: st})
	pedir := func(ruta string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, ruta, nil))
		return rec
	}

	for _, base := range []string{"/public", "/api/v1/public"} {
		rec := pedir(base + "/albums/" + token)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", base, rec.Code, rec.Body)
		}
		var cuerpo struct {
			Album struct {
				Title string `json:"title"`
				Count int    `json:"count"`
			} `json:"album"`
			Items []struct {
				ID     string `json:"id"`
				Nombre string `json:"name"`
				Ancho  int    `json:"width"`
			} `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
			t.Fatal(err)
		}
		if cuerpo.Album.Title != "Verano" || cuerpo.Album.Count != 1 || len(cuerpo.Items) != 1 ||
			cuerpo.Items[0].ID != a.ID.String() || cuerpo.Items[0].Ancho != 4000 {
			t.Errorf("%s devolvió %s", base, rec.Body)
		}
		if rec.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
			t.Errorf("%s se deja indexar", base)
		}
		if rec := pedir(base + "/albums/no-existe"); rec.Code != http.StatusNotFound {
			t.Errorf("%s con un token que no existe: %d", base, rec.Code)
		}
	}
	if rec := pedir("/api/v1/timeline"); rec.Code != http.StatusUnauthorized {
		t.Errorf("el resto de /api/v1 sigue pidiendo sesión: %d", rec.Code)
	}

	// Y lo que se monta, sale por el ingress, salvo `/public`: ese es el del BFF.
	routes, _ := router.(chi.Routes)
	ingressroutes.AssertCovered(t, routes, "syphotos", append(ingressroutes.Internal, "/public"))
}
