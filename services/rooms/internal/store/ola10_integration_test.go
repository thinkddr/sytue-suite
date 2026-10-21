// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 10 (Group DMs) contra PostgreSQL real (pgtest):
// DM directo de grupo idempotente por CONJUNTO de participantes, retrocompatibilidad
// del 1:1, validación (no-miembro, duplicados, tope 8) y display per-visor (nombre
// compuesto + peers). Detrás del build-tag `integration`. Sigue el patrón de
// ola3_integration_test.go.
package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// TestOpenGroupDirectSetIdempotentAndCap: el mismo CONJUNTO (en cualquier orden y lo
// abra quien lo abra) reusa el mismo canal; el 1:1 sigue funcionando (y no colisiona
// con el grupo); y valida no-miembro/duplicado/actor/tope.
func TestOpenGroupDirectSetIdempotentAndCap(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, _ := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic) // alice = "Creador"
	for _, m := range []struct{ sub, name string }{
		{"bob", "Bob"},
		{"carol", "Carol"},
		{"dave", "Dave"},
		{"eve", "Eve"},
		{"frank", "Frank"},
		{"grace", "Grace"},
		{"heidi", "Heidi"},
		{"ivan", "Ivan"},
	} {
		if err := st.AddTeamMember(ctx, tm.ID, "alice", m.sub, m.name, domain.RoleMember); err != nil {
			t.Fatalf("AddTeamMember %s: %v", m.sub, err)
		}
	}

	// Mismo CONJUNTO {alice,bob,carol} en cualquier orden / cualquier actor → mismo canal.
	g1, err := st.OpenGroupDirect(ctx, tm.ID, "alice", []string{"bob", "carol"}, nil)
	if err != nil {
		t.Fatalf("OpenGroupDirect alice+[bob,carol]: %v", err)
	}
	if g1.Kind != domain.ChannelDirect {
		t.Fatalf("kind = %q, quiero direct", g1.Kind)
	}
	if g2, err := st.OpenGroupDirect(ctx, tm.ID, "carol", []string{"alice", "bob"}, nil); err != nil || g2.ID != g1.ID {
		t.Fatalf("group DM no idempotente por conjunto: %s err=%v (quiero %s)", g2.ID, err, g1.ID)
	}
	if g3, err := st.OpenGroupDirect(ctx, tm.ID, "bob", []string{"carol", "alice"}, nil); err != nil || g3.ID != g1.ID {
		t.Fatalf("tercera apertura no idempotente: %s err=%v", g3.ID, err)
	}

	// Retrocompat 1:1: OpenDirect y OpenGroupDirect (un solo otro) caen en el mismo canal,
	// distinto del grupo.
	d1, err := st.OpenDirect(ctx, tm.ID, "alice", "bob")
	if err != nil {
		t.Fatalf("OpenDirect alice→bob: %v", err)
	}
	if d1.ID == g1.ID {
		t.Fatalf("el 1:1 y el grupo comparten canal, no deberían")
	}
	if d2, err := st.OpenGroupDirect(ctx, tm.ID, "bob", []string{"alice"}, nil); err != nil || d2.ID != d1.ID {
		t.Fatalf("1:1 vía group no coincide con OpenDirect: %s err=%v (quiero %s)", d2.ID, err, d1.ID)
	}

	// No-miembro del team en el conjunto → ErrForbidden.
	if _, err := st.OpenGroupDirect(ctx, tm.ID, "alice", []string{"bob", "stranger"}, nil); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("grupo con no-miembro debe ser ErrForbidden, got %v", err)
	}
	// Duplicado en la lista → ErrInvalidInput.
	if _, err := st.OpenGroupDirect(ctx, tm.ID, "alice", []string{"bob", "bob"}, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("duplicado debe ser ErrInvalidInput, got %v", err)
	}
	// El propio actor en la lista → ErrInvalidInput.
	if _, err := st.OpenGroupDirect(ctx, tm.ID, "alice", []string{"alice"}, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("actor repetido debe ser ErrInvalidInput, got %v", err)
	}
	// Tope: 8 en total OK (alice + 7 otros); 9 en total falla.
	seven := []string{"bob", "carol", "dave", "eve", "frank", "grace", "heidi"}
	if _, err := st.OpenGroupDirect(ctx, tm.ID, "alice", seven, nil); err != nil {
		t.Fatalf("8 participantes deben ir OK: %v", err)
	}
	eight := append(append([]string(nil), seven...), "ivan")
	if _, err := st.OpenGroupDirect(ctx, tm.ID, "alice", eight, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("9 participantes deben fallar con ErrInvalidInput, got %v", err)
	}
}

// TestGroupDirectDisplayPerViewer: ListChannels resuelve el nombre del grupo a los
// OTROS miembros (per-visor, ordenados por nombre) y expone peers para distinguirlo
// de un 1:1.
func TestGroupDirectDisplayPerViewer(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, _ := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic) // alice = "Creador"
	for _, m := range []struct{ sub, name string }{{"bob", "Bob"}, {"carol", "Carol"}} {
		if err := st.AddTeamMember(ctx, tm.ID, "alice", m.sub, m.name, domain.RoleMember); err != nil {
			t.Fatalf("AddTeamMember %s: %v", m.sub, err)
		}
	}
	g, err := st.OpenGroupDirect(ctx, tm.ID, "alice", []string{"bob", "carol"}, nil)
	if err != nil {
		t.Fatalf("OpenGroupDirect: %v", err)
	}

	// alice ve a los OTROS (Bob, Carol) ordenados por nombre; peers (2) lo marca como grupo.
	aliceList, err := st.ListChannels(ctx, tm.ID, "alice", false)
	if err != nil {
		t.Fatalf("ListChannels alice: %v", err)
	}
	ac := findChannel(t, aliceList, g.ID)
	if ac.Name != "Bob, Carol" {
		t.Fatalf("alice ve el grupo como %q, quiero \"Bob, Carol\"", ac.Name)
	}
	if len(ac.Peers) != 2 {
		t.Fatalf("peers de alice = %v, quiero 2 (grupo)", ac.Peers)
	}

	// bob ve a Carol y a alice ("Creador"), ordenados por nombre → "Carol, Creador".
	bobList, err := st.ListChannels(ctx, tm.ID, "bob", false)
	if err != nil {
		t.Fatalf("ListChannels bob: %v", err)
	}
	if bc := findChannel(t, bobList, g.ID); bc.Name != "Carol, Creador" {
		t.Fatalf("bob ve el grupo como %q, quiero \"Carol, Creador\"", bc.Name)
	}
}
