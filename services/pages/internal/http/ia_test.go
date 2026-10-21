// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/ia"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

func conAutenticacion(r *http.Request, roles []string, plan string) *http.Request {
	claims := authn.Claims{
		Subject: "sub-123",
		Tenant:  "acme",
		Roles:   roles,
		Plan:    plan,
	}
	ctx := authn.Into(r.Context(), claims)
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	return r.WithContext(ctx)
}

func TestIAHTTP_RutasYPermisos(t *testing.T) {
	kek := []byte("01234567890123456789012345678901")
	kc, err := keycrypto.New(kek)
	if err != nil {
		t.Fatalf("keycrypto: %v", err)
	}

	aiMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"modelos": []string{"mistral-large", "gpt-4o"}})
	}))
	defer aiMock.Close()

	cli := asistente.Nuevo(aiMock.URL, 5*time.Second)
	cfg := config.Config{
		AISecret:         "secret",
		IACupoPro:        100,
		IACupoEnterprise: 500,
	}
	st := store.New(nil)
	iaSvc := ia.Nuevo(cfg, st, cli, kc, nil, nil, nil, nil)

	h := &Handlers{
		Deps: Deps{
			Cfg:   cfg,
			Store: st,
			IA:    iaSvc,
		},
	}

	r := chi.NewRouter()
	r.Route("/ia", func(ir chi.Router) {
		ir.Get("/key", h.obtenerClaveAI)
		ir.Put("/key", h.guardarClaveAI)
		ir.Delete("/key", h.borrarClaveAI)
		ir.Post("/key:check", h.comprobarClaveAI)
		ir.Get("/quota", h.obtenerCupoAI)
		ir.Get("/jobs/{jid}", h.obtenerTrabajoAI)
		ir.Get("/jobs/{jid}/zip", h.descargarZipTrabajoAI)
	})
	r.Route("/sites/{id}/ia", func(ir chi.Router) {
		ir.Post("/design", h.crearTrabajoDesign)
		ir.Post("/translate", h.crearTrabajoTranslate)
		ir.Post("/export", h.crearTrabajoExport)
	})

	siteID := uuid.New()

	t.Run("GET /ia/key exige rol admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ia/key", http.NoBody)
		req = conAutenticacion(req, []string{"member"}, "pro")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("esperaba 403, obtuvo %d", rec.Code)
		}
	})

	t.Run("PUT /ia/key exige aviso si openai", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"provider":        "openai",
			"model":           "gpt-4o",
			"key":             "sk-test",
			"notice_accepted": false,
		})
		req := httptest.NewRequest(http.MethodPut, "/ia/key", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = conAutenticacion(req, []string{"admin"}, "pro")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("esperaba 400 por aviso requerido, obtuvo %d", rec.Code)
		}
	})

	t.Run("POST /ia/key:check devuelve modelos con admin", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"provider": "mistral",
			"key":      "api-key-test",
		})
		req := httptest.NewRequest(http.MethodPost, "/ia/key:check", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = conAutenticacion(req, []string{"admin"}, "pro")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("esperaba 200, obtuvo %d", rec.Code)
		}
		var res struct {
			Models []string `json:"models"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil || len(res.Models) == 0 {
			t.Errorf("esperaba modelos, obtuvo: %v", res)
		}
	})

	t.Run("DELETE /ia/key exige admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/ia/key", http.NoBody)
		req = conAutenticacion(req, []string{"member"}, "pro")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("esperaba 403, obtuvo %d", rec.Code)
		}
	})

	t.Run("POST /sites/{id}/ia/design valida prompt", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"prompt": "",
		})
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/ia/design", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = conAutenticacion(req, []string{"editor"}, "pro")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("esperaba 400, obtuvo %d", rec.Code)
		}
	})

	t.Run("POST /sites/{id}/ia/translate scope store exige admin", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"from":  "es",
			"to":    []string{"en"},
			"scope": "store",
		})
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/ia/translate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = conAutenticacion(req, []string{"editor"}, "pro")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("esperaba 403 admin_required, obtuvo %d", rec.Code)
		}
	})

	t.Run("POST translate no amplía un alcance inválido ni finge traducir tienda", func(t *testing.T) {
		for _, test := range []struct {
			scope  string
			status int
			code   string
		}{
			{"page", http.StatusBadRequest, "invalid_translation_scope"},
			{"page:", http.StatusBadRequest, "invalid_translation_scope"},
			{"store", http.StatusUnprocessableEntity, "translation_scope_unavailable"},
		} {
			body, _ := json.Marshal(map[string]any{"from": "es", "to": []string{"en"}, "scope": test.scope})
			req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/ia/translate", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = conAutenticacion(req, []string{"admin"}, "pro")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if rec.Code != test.status || response.Error.Code != test.code {
				t.Errorf("%s: %d %s", test.scope, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("POST /sites/{id}/ia/export exige admin", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"destination": "shopify",
		})
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/ia/export", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = conAutenticacion(req, []string{"editor"}, "pro")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("esperaba 403, obtuvo %d", rec.Code)
		}
	})

	t.Run("POST translate rechaza idiomas sin soporte antes de crear trabajos", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"from": "es", "to": []string{"fr"}, "scope": "site"})
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/ia/translate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = conAutenticacion(req, []string{"editor"}, "pro")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var response struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusBadRequest || response.Error.Code != "invalid_languages" {
			t.Fatalf("%d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /ia/jobs/{jid}/zip exige admin y valida exportación", func(t *testing.T) {
		jid := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/ia/jobs/"+jid.String()+"/zip", http.NoBody)
		req = conAutenticacion(req, []string{"editor"}, "pro")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("esperaba 403, obtuvo %d", rec.Code)
		}
	})
}
