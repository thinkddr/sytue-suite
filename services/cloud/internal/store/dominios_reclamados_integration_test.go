// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func proyectoDe(t *testing.T, st *store.Store, ctx context.Context, slug string) uuid.UUID {
	t.Helper()
	p, err := st.CreateProject(ctx, store.NewProjectRecord{
		Slug: slug, Name: slug, GitProvider: domain.GitGeneric,
		ProductionRef: "main", Framework: domain.FrameworkDetect,
	}, 0)
	if err != nil {
		t.Fatalf("CreateProject(%s): %v", slug, err)
	}
	return p.ID
}

// CLOUD-10: otra organización reclama el dominio de la víctima y no lo verifica
// nunca. La víctima puede añadirlo igual; gana quien lo verifica.
func TestDominioPendienteNoOcupa(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	acme, mallory := tenantCtx("acme"), tenantCtx("mallory")
	pa, pm := proyectoDe(t, st, acme, "tienda"), proyectoDe(t, st, mallory, "intruso")

	dm, err := st.AddDomain(mallory, pm, "tienda.example.eu", "t-m")
	if err != nil {
		t.Fatalf("reclamación de mallory: %v", err)
	}
	da, err := st.AddDomain(acme, pa, "tienda.example.eu", "t-a")
	if err != nil {
		t.Fatalf("la dueña no puede añadir su dominio: %v", err)
	}
	if err := st.SetDomainStatus(context.Background(), "acme", da.ID, domain.DomainVerified); err != nil {
		t.Fatalf("verificar el de la dueña: %v", err)
	}
	if err := st.SetDomainStatus(context.Background(), "mallory", dm.ID, domain.DomainVerified); !errors.Is(err, domain.ErrDomainTaken) {
		t.Fatalf("el segundo en verificar tiene que perder: %v", err)
	}
	if _, err := st.AddDomain(mallory, pm, "TIENDA.example.eu", "t-m2"); !errors.Is(err, domain.ErrDomainTaken) {
		t.Fatalf("un dominio ya verificado sigue ocupado: %v", err)
	}
}

// CLOUD-06: cincuenta dominios que nunca se verifican no impiden mirar el 51.
func TestColaDeDominiosSeReparte(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	pid := proyectoDe(t, st, ctx, "web")
	for i := range 50 {
		if _, err := st.AddDomain(ctx, pid, fmt.Sprintf("d%d.example.eu", i), "t"); err != nil {
			t.Fatal(err)
		}
	}
	ultimo, err := st.AddDomain(ctx, pid, "ultimo.example.eu", "t")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ListPendingDomains(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	segunda, err := st.ListPendingDomains(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range segunda {
		if d.ID == ultimo.ID {
			return
		}
	}
	t.Fatalf("la segunda vuelta no recoge el que no se miró (%d dominios)", len(segunda))
}
