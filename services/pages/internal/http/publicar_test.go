// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/publicar"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

func conClaims(r *http.Request, roles []string, sub string) *http.Request {
	claims := authn.Claims{
		Subject: sub,
		Tenant:  "acme",
		Roles:   roles,
	}
	ctx := authn.Into(r.Context(), claims)
	return r.WithContext(ctx)
}

func TestPublicarHTTP_Validaciones(t *testing.T) {
	st := store.New(nil)
	cfg := config.Config{
		PublicadorSecret: "secreto-publicador",
		Sitios: config.Sitios{
			SitesDomain: "sites.example.test",
			CastOrigin:  "https://cast.example.test",
		},
	}
	h := &Handlers{
		Deps: Deps{
			Cfg:      cfg,
			Store:    st,
			Publicar: publicar.Nuevo(),
		},
	}

	pubGate := httpx.SecretoEnCabecera("X-Sytue-Publicador", cfg.PublicadorSecret)
	r := chi.NewRouter()
	siteID := uuid.New()

	r.Route("/sites/{id}", func(sr chi.Router) {
		sr.Post("/snapshots", h.crearSnapshot)
		sr.Get("/render-data", h.obtenerDatosRender)
		sr.With(pubGate).Post("/previews", h.crearVistaPrevia)
		sr.Get("/publications", h.listarPublicaciones)
		sr.Post("/publications/{number}/rollback", h.rollbackPublicacion)
		sr.Post("/unpublish", h.despublicarSitio)
		sr.Get("/files/{nombre}", h.sitioFichero)
		sr.With(pubGate).Post("/publications", h.subirPublicacion)
	})
	r.Get("/export", h.exportarHub)

	t.Run("snapshot_purpose_invalido_400", func(t *testing.T) {
		cuerpo := []byte(`{"purpose":"inventado"}`)
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/snapshots", bytes.NewReader(cuerpo))
		req.Header.Set("Content-Type", "application/json")
		req = conClaims(req, []string{"member"}, "usr-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code = %d; want 400", w.Code)
		}
	})

	t.Run("preview_sin_secreto_publicador_403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/previews", http.NoBody)
		req = conClaims(req, []string{"member"}, "usr-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d; want 403", w.Code)
		}
	})

	t.Run("publicacion_sin_secreto_publicador_403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/publications?kind=live", http.NoBody)
		req = conClaims(req, []string{"admin"}, "usr-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d; want 403", w.Code)
		}
	})

	t.Run("publicacion_kind_invalido_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/publications?kind=staging", http.NoBody)
		req.Header.Set("X-Sytue-Publicador", "secreto-publicador")
		req = conClaims(req, []string{"admin"}, "usr-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code = %d; want 400", w.Code)
		}
	})

	t.Run("publicacion_preview_sin_token_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/publications?kind=preview", http.NoBody)
		req.Header.Set("X-Sytue-Publicador", "secreto-publicador")
		req = conClaims(req, []string{"member"}, "usr-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code = %d; want 400", w.Code)
		}
	})

	t.Run("rollback_no_admin_403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/publications/1/rollback", http.NoBody)
		req = conClaims(req, []string{"member"}, "usr-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d; want 403", w.Code)
		}
	})

	t.Run("rollback_numero_invalido_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/publications/abc/rollback", http.NoBody)
		req = conClaims(req, []string{"admin"}, "usr-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code = %d; want 400", w.Code)
		}
	})

	t.Run("unpublish_no_admin_403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/sites/"+siteID.String()+"/unpublish", http.NoBody)
		req = conClaims(req, []string{"member"}, "usr-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("code = %d; want 403", w.Code)
		}
	})

	t.Run("fichero_patron_invalido_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sites/"+siteID.String()+"/files/bad..name.jpg", http.NoBody)
		req = conClaims(req, []string{"member"}, "usr-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code = %d; want 400", w.Code)
		}
	})

	t.Run("export_hub_formato_invalido_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/export?format=yaml", http.NoBody)
		req = conClaims(req, []string{"member"}, "usr-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code = %d; want 400", w.Code)
		}
	})
}

func TestEsLegalCompleto(t *testing.T) {
	incompleto := domain.DatosLegales{
		HolderName: "Sytue SL",
		TaxID:      "B12345678",
		Country:    "ES",
		// Faltan dirección y email
	}
	if domain.EsLegalCompleto(incompleto) {
		t.Error("EsLegalCompleto debió ser false para datos incompletos")
	}

	sinRegistroESSociedad := domain.DatosLegales{
		HolderName: "Empresa SL",
		TaxID:      "B12345678",
		Country:    "ES",
		Address: domain.DireccionLegal{
			Street:     "Gran Vía 1",
			PostalCode: "28013",
			City:       "Madrid",
		},
		Email: "info@example.com",
		// Falta Registro Mercantil para una SL en ES (B)
		Registry: "",
	}
	if domain.EsLegalCompleto(sinRegistroESSociedad) {
		t.Error("EsLegalCompleto debió exigir Registro para sociedad mercantil española (B...)")
	}

	completo := domain.DatosLegales{
		HolderName: "Empresa SL",
		TaxID:      "B12345678",
		Country:    "ES",
		Address: domain.DireccionLegal{
			Street:     "Gran Vía 1",
			PostalCode: "28013",
			City:       "Madrid",
		},
		Email:    "info@example.com",
		Registry: "Tomo 1234, Folio 56, Hoja M-7890",
	}
	if !domain.EsLegalCompleto(completo) {
		t.Error("EsLegalCompleto debió ser true para datos completos")
	}
}

func TestTieneContenidoDemo(t *testing.T) {
	irConDemo := []byte(`{"pages":[{"sections":[{"children":[{"content":{"es":[{"text":"Aquí va la opinión de un cliente real"}]}}]}]}]}`)
	if !domain.TieneContenidoDemo(irConDemo, "clinica") {
		t.Error("TieneContenidoDemo debió detectar 'Aquí va la opinión de un cliente real'")
	}

	irLimpio := []byte(`{"pages":[{"sections":[{"children":[{"content":{"es":[{"text":"Texto personalizado de cliente"}]}}]}]}]}`)
	if domain.TieneContenidoDemo(irLimpio, "clinica") {
		t.Error("TieneContenidoDemo no debió detectar texto personalizado como demo")
	}
}
