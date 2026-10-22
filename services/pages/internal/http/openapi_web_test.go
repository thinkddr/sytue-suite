// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/tienda"
)

type openAPIWebSpec struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

func TestOpenAPIWebAtaRutas(t *testing.T) {
	// 1. Leer openapi-web.yaml
	yamlPath := filepath.Join("..", "..", "openapi-web.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("leer openapi-web.yaml: %v", err)
	}

	var spec openAPIWebSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("deserializar openapi-web.yaml: %v", err)
	}

	// 2. Construir router público con dependencias mínimas
	st := store.New(nil)
	tiendaSvc := tienda.Nuevo(st, nil, "https://web.sytue.eu", "sites.sytue.eu", nil, nil)

	r := httpx.NewRouter(httpx.RouterOptions{MaxBodyBytes: maxCuerpo})
	var g grupos
	r.Route("/_pages", func(pub chi.Router) {
		pub.Use(cabecerasDeWeb, conPlazo(plazoAPI), httpx.MaxBodyBytes(maxCuerpoPublico))
		g.publico = pub
	})
	h := &Handlers{Deps: Deps{
		Store:  st,
		Tienda: tiendaSvc,
	}}

	montarPublicoTienda(g, h)
	montarDenuncias(g, h)

	// 3. Recopilar rutas bajo /_pages/*
	rutasRouter := make(map[string]map[string]bool)
	err = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/_pages") {
			// Normalizar variables chi como {id} / {form_id}
			normRoute := strings.ReplaceAll(route, "/*", "")
			if normRoute == "/_pages" {
				return nil
			}
			if rutasRouter[normRoute] == nil {
				rutasRouter[normRoute] = make(map[string]bool)
			}
			rutasRouter[normRoute][strings.ToUpper(method)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk: %v", err)
	}

	// 4. Comprobar que cada ruta y método de OpenAPI está en el router
	for path, methods := range spec.Paths {
		routerMethods, ok := rutasRouter[path]
		if !ok {
			t.Errorf("Ruta de OpenAPI %s no está montada en el router", path)
			continue
		}
		for m := range methods {
			metodo := strings.ToUpper(m)
			if metodo == "PARAMETERS" || metodo == "SUMMARY" || metodo == "DESCRIPTION" {
				continue
			}
			if !routerMethods[metodo] {
				t.Errorf("Método %s para ruta %s definido en OpenAPI pero no en router", metodo, path)
			}
		}
	}

	// 5. Comprobar que no hay rutas en /_pages que no estén en OpenAPI
	for path, methods := range rutasRouter {
		specMethods, ok := spec.Paths[path]
		if !ok {
			t.Errorf("Ruta del router %s no está definida en openapi-web.yaml", path)
			continue
		}
		for m := range methods {
			if m == "OPTIONS" {
				continue // OPTIONS es preflight CORS
			}
			metodo := strings.ToLower(m)
			if _, ok := specMethods[metodo]; !ok {
				t.Errorf("Método %s en ruta %s presente en router pero no en openapi-web.yaml", m, path)
			}
		}
	}
}
