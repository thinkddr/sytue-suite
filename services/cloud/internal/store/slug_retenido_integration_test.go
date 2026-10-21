// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)

//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// CLOUD-08 con la decisión del dueño (2026-10-17): el slug de un proyecto borrado
// lo recupera solo su organización hasta que pasen 90 días y se hayan borrado sus
// imágenes; después, cualquiera.
func TestSlugDeProyectoBorradoRetenido(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	acme, mallory := tenantCtx("acme"), tenantCtx("mallory")
	nuevo := func(ctx context.Context) error {
		_, err := st.CreateProject(ctx, store.NewProjectRecord{
			Slug: "tienda", Name: "tienda", GitProvider: domain.GitGeneric,
			ProductionRef: "main", Framework: domain.FrameworkDetect,
		}, 0)
		return err
	}
	if err := nuevo(acme); err != nil {
		t.Fatalf("alta: %v", err)
	}
	ps, err := st.ListProjects(acme)
	if err != nil || len(ps) != 1 {
		t.Fatalf("ListProjects: %v %v", ps, err)
	}
	if err := st.DeleteProject(acme, ps[0].ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	if err := nuevo(mallory); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("otra organización toma el slug recién borrado: %v", err)
	}
	if taken, _ := st.SlugsTaken(mallory, []string{"tienda"}); !taken["tienda"] {
		t.Fatal("la disponibilidad lo da por libre")
	}

	// Purgado pero dentro del plazo: sigue retenido.
	if err := st.MarkProjectPurged(context.Background(), ps[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := nuevo(mallory); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("purgado pero dentro de los 90 días: %v", err)
	}

	// 91 días después y purgado: libre para cualquiera.
	admin, err := pgx.Connect(context.Background(), h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	if _, err := admin.Exec(context.Background(),
		`UPDATE projects SET deleted_at = now() - interval '91 days' WHERE id=$1`, ps[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := nuevo(mallory); err != nil {
		t.Fatalf("pasado el plazo y purgado, el slug está libre: %v", err)
	}
}

// La propia organización sí lo recupera antes del plazo.
func TestSlugBorradoLoRecuperaSuOrganizacion(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	acme := tenantCtx("acme")
	rec := store.NewProjectRecord{
		Slug: "tienda", Name: "tienda", GitProvider: domain.GitGeneric,
		ProductionRef: "main", Framework: domain.FrameworkDetect,
	}
	p, err := st.CreateProject(acme, rec, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteProject(acme, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateProject(acme, rec, 0); err != nil {
		t.Fatalf("la misma organización recupera su slug: %v", err)
	}
	pend, err := st.ListProjectsToPurge(context.Background(), 10)
	if err != nil || len(pend) != 1 || !pend[0].SlugVivo {
		t.Fatalf("el borrado con el slug ya recuperado no debe purgar imágenes: %+v %v", pend, err)
	}
}
