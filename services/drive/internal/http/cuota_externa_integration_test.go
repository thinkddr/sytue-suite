// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El flujo real de la cuota compartida de extremo a extremo (CAST-ENCARGO §8, mig
// 0026): Cast declara por la puerta interna y la persona ve el total y el desglose en
// GET /api/v1/quota. Servicio y store reales sobre un PostgreSQL efímero; sin S3 (la
// cuota no toca blobs).
package drivehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

func TestGetQuotaTraeElDesgloseExterno(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := service.NewStore(h.Pool)
	cfg := config.Config{PublicURL: "https://drive.sytue.eu", DefaultQuotaBytes: 1000, MaxUploadBytes: 1 << 20}
	svc := service.New(cfg, st, nil, nil, nil)
	hs := &Handlers{svc: svc, cfg: cfg, externalUsage: st}

	// Cast declara 300 en acme. Con chi para que {app} llegue al handler.
	r := chi.NewRouter()
	r.Put("/internal/quota/external/{app}", hs.SetExternalUsage)
	req := httptest.NewRequest(http.MethodPut, "/internal/quota/external/cast", strings.NewReader(`{"tenant":"acme","bytes":300}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("declarar => %d: %s", rec.Code, rec.Body)
	}

	cuotaDe := func(slug string) (used, available int64, external map[string]int64) {
		ctx := tenancy.Into(authn.Into(context.Background(), authn.Claims{Subject: "u-1", Tenant: slug}), tenancy.Tenant{Slug: slug})
		rec := httptest.NewRecorder()
		hs.GetQuota(rec, httptest.NewRequest(http.MethodGet, "/api/v1/quota", http.NoBody).WithContext(ctx))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /quota de %s => %d: %s", slug, rec.Code, rec.Body)
		}
		var got struct {
			Used      int64            `json:"used_bytes"`
			Available int64            `json:"available_bytes"`
			External  map[string]int64 `json:"external_usage"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return got.Used, got.Available, got.External
	}

	used, available, external := cuotaDe("acme")
	if used != 300 || available != 700 || external["cast"] != 300 {
		t.Fatalf("acme: used=%d available=%d external=%v", used, available, external)
	}
	// Otra organización: nada declarado, y el desglose va vacío pero presente.
	used, available, external = cuotaDe("beta")
	if used != 0 || available != 1000 || external == nil || len(external) != 0 {
		t.Fatalf("beta: used=%d available=%d external=%v", used, available, external)
	}
}
