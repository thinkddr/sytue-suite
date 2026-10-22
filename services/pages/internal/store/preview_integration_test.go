// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

func TestDespublicarRevocaPreviewsEnSuTenant(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	s := New(h.Pool)
	ctx := context.Background()
	crear := func(tenant string) (uuid.UUID, uuid.UUID) {
		t.Helper()
		var id, version uuid.UUID
		if err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, `INSERT INTO sites (tenant,slug,name,draft,created_by,published_number) VALUES ($1,$1,'Preview','{}','sub',1) RETURNING id`, tenant).Scan(&id); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `INSERT INTO site_versions (tenant,site_id,kind,ir,draft_rev,created_by) VALUES ($1,$2,'preview','{}',1,'sub') RETURNING id`, tenant, id).Scan(&version)
		}); err != nil {
			t.Fatal(err)
		}
		org := tenancy.Into(ctx, tenancy.Tenant{Slug: tenant})
		if err := s.CrearPublicacionPreview(org, id, version, "0123456789abcdef0123456789abcdef", time.Now().Add(time.Hour), 1, 1, "test", json.RawMessage("{}"), "sub"); err != nil {
			t.Fatal(err)
		}
		return id, version
	}
	id, _ := crear("acme")
	otro, _ := crear("globex")
	org := tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	contar := func(tenant string, id uuid.UUID) int {
		t.Helper()
		n, err := s.ContarVistasVivas(tenancy.Into(ctx, tenancy.Tenant{Slug: tenant}), id)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := s.Despublicar(org, otro); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("retirar otro tenant=%v", err)
	}
	if contar("acme", id) != 1 || contar("globex", otro) != 1 {
		t.Fatal("intento ajeno cambió previews")
	}
	if err := s.Despublicar(org, id); err != nil {
		t.Fatal(err)
	}
	site, err := s.SitioCompleto(org, id)
	if err != nil {
		t.Fatal(err)
	}
	if site.PublishedNumber != nil {
		t.Fatal("la publicación sigue vigente")
	}
	if contar("acme", id) != 0 {
		t.Fatal("retirar dejó el token preview vigente")
	}
	if contar("globex", otro) != 1 {
		t.Fatal("retirar revocó previews de otro tenant")
	}
	_, previews, _, _, err := s.ObtenerDatosParaMapa(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range previews {
		if p.SiteID == id {
			t.Fatal("el mapa aún anuncia la preview retirada")
		}
	}
	// La revocación conserva el historial y permite una autorización nueva explícita.
	pubs, err := s.ListarPublicaciones(org, id)
	if err != nil || len(pubs) != 1 {
		t.Fatalf("historial perdido: %v,%v", pubs, err)
	}
	if err := s.Despublicar(org, id); err != nil {
		t.Fatalf("retirada no idempotente: %v", err)
	}
}
