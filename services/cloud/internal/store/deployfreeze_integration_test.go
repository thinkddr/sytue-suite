// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// La congelación de producción contra un PostgreSQL real: lo que solo la base
// garantiza es que una organización no congela —ni lee, ni descongela— el
// proyecto de otra, y que volver a congelar no reescribe desde cuándo.
package store_test

import (
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func TestDeployFreezeIntegration(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)

	acme := tenantCtx("acme")
	otra := tenantCtx("globex")
	p := seedProject(t, acme, st, "tienda")

	if _, hay, err := st.GetDeployFreeze(acme, p.ID); err != nil || hay {
		t.Fatalf("sin congelar no debe haber congelación, got %v, %v", hay, err)
	}

	primera, err := st.SetDeployFreeze(acme, p.ID, "cierre contable", "user-1")
	if err != nil {
		t.Fatalf("SetDeployFreeze: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	segunda, err := st.SetDeployFreeze(acme, p.ID, "campaña", "user-2")
	if err != nil {
		t.Fatalf("SetDeployFreeze (2.ª): %v", err)
	}
	if segunda.Reason != "campaña" {
		t.Errorf("el motivo debe actualizarse, got %q", segunda.Reason)
	}
	if !segunda.FrozenAt.Equal(primera.FrozenAt) || segunda.FrozenBy != "user-1" {
		t.Errorf("volver a congelar no puede reescribir desde cuándo ni quién: %+v vs %+v", segunda, primera)
	}

	// Aislamiento: otra organización no ve, no congela y no descongela este proyecto.
	if _, hay, err := st.GetDeployFreeze(otra, p.ID); err != nil || hay {
		t.Fatalf("otra organización no debe ver la congelación, got %v, %v", hay, err)
	}
	if _, err := st.SetDeployFreeze(otra, p.ID, "x", "intruso"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("congelar un proyecto ajeno debe dar ErrNotFound, got %v", err)
	}
	if borrada, err := st.ClearDeployFreeze(otra, p.ID); err != nil || borrada {
		t.Fatalf("descongelar un proyecto ajeno no puede borrar nada, got %v, %v", borrada, err)
	}

	if borrada, err := st.ClearDeployFreeze(acme, p.ID); err != nil || !borrada {
		t.Fatalf("ClearDeployFreeze: %v, %v", borrada, err)
	}
	if borrada, err := st.ClearDeployFreeze(acme, p.ID); err != nil || borrada {
		t.Fatalf("descongelar dos veces no borra nada, got %v, %v", borrada, err)
	}
}
