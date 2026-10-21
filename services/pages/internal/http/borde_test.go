// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/testutil"
)

func routerBordeTest(hosts map[string]string) *chi.Mux {
	lector := mapa.NuevoLector(nil, 10*time.Second)
	// Inicializar lector con hosts
	_ = lector.Refrescar(context.Background())

	h := &Handlers{
		Deps: Deps{
			Lector: lector,
		},
	}

	r := chi.NewRouter()
	g := grupos{
		borde: r,
	}
	montarBorde(g, h)
	return r
}

func TestBordePermitido(t *testing.T) {
	r := routerBordeTest(map[string]string{
		"panaderia-aurora.sites.sytue.eu": "site-1",
		"www.panaderia-aurora.es":         "site-1",
	})

	t.Run("Sin parametro domain devuelve 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/_borde/permitido", http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("esperado 404, obtenido %d", rec.Code)
		}
	})

	t.Run("Dominio invalido o IP literal devuelve 404", func(t *testing.T) {
		casos := []string{
			"127.0.0.1",
			"192.168.1.1",
			"invalido..com",
			"sin-extension",
		}
		for _, dom := range casos {
			req := httptest.NewRequest(http.MethodGet, "/_borde/permitido?domain="+dom, http.NoBody)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("host %s: esperado 404, obtenido %d", dom, rec.Code)
			}
		}
	})

	t.Run("Host no registrado devuelve 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/_borde/permitido?domain=desconocido.com", http.NoBody)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("esperado 404, obtenido %d", rec.Code)
		}
	})
}

// WEB-11. Inundar el ask con dominios inventados no deja sin respuesta a los hosts
// publicados, que se contestan del mapa y sin cubo; lo que va a la base sí lo lleva.
func TestElAskNoSeAgotaConDominiosInventados(t *testing.T) {
	almacen := testutil.NuevoAlmacen(t)
	doc, err := json.Marshal(mapa.HostsMapa{
		Version: 1,
		Sites:   map[string]mapa.SiteMapa{"s1": {Status: "live", Canonical: "tienda.example.es"}},
		Hosts:   map[string]string{"tienda.example.es": "s1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	almacen.Escribir(mapa.ClaveHostsJSON, doc)
	lector := mapa.NuevoLector(almacen.Objetos, time.Minute)
	if err := lector.Refrescar(context.Background()); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	montarBorde(grupos{borde: r}, &Handlers{Deps: Deps{Lector: lector, Store: store.New(nil)}})
	pedir := func(dominio string) int {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_borde/permitido?domain="+dominio, http.NoBody))
		return rec.Code
	}

	limitados := 0
	for i := 0; i < 100; i++ {
		if pedir(fmt.Sprintf("inventado-%d.example.org", i)) == http.StatusTooManyRequests {
			limitados++
		}
	}
	if limitados == 0 {
		t.Fatal("lo que pregunta a la base no lleva cubo")
	}
	if code := pedir("tienda.example.es"); code != http.StatusOK {
		t.Fatalf("tras la inundación, el ask de un host publicado: %d; quiero 200", code)
	}
}
