// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// La administración de la organización ve lo que el producto esconde, y solo eso.
//
// El listado normal (`ListChannels`) filtra por pertenencia —`c.kind = 'public'
// OR mem.subject IS NOT NULL`—, que es lo correcto para quien usa Rooms y lo que
// dejaba al administrador sin poder deshacer nada en un canal privado del que no
// forma parte. Aquí se fija la diferencia en las dos direcciones, porque las dos
// se rompen en silencio: si el filtro se cuela, el admin no ve los privados y la
// pantalla parece vacía; si sobra, los MENSAJES DIRECTOS acabarían en una tabla
// de administración, enseñando con quién habla cada cual sin abrir un mensaje.
package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestAdminVeLosPrivadosSinSerMiembroYNuncaLosDirectos(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, err := st.CreateTeam(ctx, store.NewTeamRecord{
		Slug: "eq", Name: "Equipo", CreatedBy: "alice", CreatorName: "Alicia",
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	// Un canal privado de Alicia. La administradora (Berta) NO está dentro: es
	// justo el caso que el listado del producto le esconde.
	priv, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "secreto", Name: "Secreto",
		Kind: domain.ChannelPrivate, CreatedBy: "alice",
	})
	if err != nil {
		t.Fatalf("CreateChannel privado: %v", err)
	}

	// Y un mensaje directo entre dos personas, que NO debe salir nunca. Las DOS
	// tienen que estar en el equipo: `OpenDirect` resuelve el nombre de cada una
	// con `teamMemberName`, así que con una fuera falla con «no eres miembro» y
	// el test moría antes de comprobar lo que venía a comprobar.
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "carla", "Carla", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember(carla): %v", err)
	}
	if _, err := st.OpenDirect(ctx, tm.ID, "alice", "carla"); err != nil {
		t.Fatalf("OpenDirect: %v", err)
	}

	items, err := st.ListAllChannels(ctx, false)
	if err != nil {
		t.Fatalf("ListAllChannels: %v", err)
	}
	var vioPrivado bool
	for _, c := range items {
		if c.Kind == domain.ChannelDirect {
			t.Fatalf("un mensaje directo se coló en la administración: %s", c.ID)
		}
		if c.ID == priv.ID {
			vioPrivado = true
			if c.TeamName != "Equipo" {
				t.Errorf("team_name = %q, se esperaba «Equipo»", c.TeamName)
			}
			if c.Members == 0 {
				t.Error("members = 0; el creador cuenta como miembro")
			}
		}
	}
	if !vioPrivado {
		t.Fatal("el canal privado NO aparece: la administración está mirando por pertenencia")
	}
}

func TestAdminArchivaYRestauraSinPertenecerAlCanal(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, err := st.CreateTeam(ctx, store.NewTeamRecord{
		Slug: "eq", Name: "Equipo", CreatedBy: "alice", CreatorName: "Alicia",
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	ch, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "secreto", Name: "Secreto",
		Kind: domain.ChannelPrivate, CreatedBy: "alice",
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	// Berta no es miembro. Por la vía del producto esto sería ErrForbidden
	// (`requireChannelAdmin`), y por eso un archivado por error era irreversible
	// desde fuera del canal.
	arch, err := st.SetChannelArchivedAsAdmin(ctx, ch.ID, true)
	if err != nil {
		t.Fatalf("archivar como admin: %v", err)
	}
	if !arch.Archived {
		t.Fatal("el canal no quedó archivado")
	}

	// Y ya no sale en el listado por defecto, pero sí al pedir los archivados:
	// esconderlos es lo que mantiene la lista utilizable, no una forma de perderlos.
	vivos, err := st.ListAllChannels(ctx, false)
	if err != nil {
		t.Fatalf("ListAllChannels(false): %v", err)
	}
	for _, c := range vivos {
		if c.ID == ch.ID {
			t.Fatal("un canal archivado sale en el listado por defecto")
		}
	}
	todos, err := st.ListAllChannels(ctx, true)
	if err != nil {
		t.Fatalf("ListAllChannels(true): %v", err)
	}
	var estaba bool
	for _, c := range todos {
		if c.ID == ch.ID {
			estaba = true
		}
	}
	if !estaba {
		t.Fatal("el canal archivado no aparece ni pidiéndolo")
	}

	// Deshacer, que es para lo que existe todo esto.
	rest, err := st.SetChannelArchivedAsAdmin(ctx, ch.ID, false)
	if err != nil {
		t.Fatalf("restaurar como admin: %v", err)
	}
	if rest.Archived {
		t.Fatal("el canal sigue archivado tras restaurarlo")
	}

	st2, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st2.Private < 1 {
		t.Errorf("Stats.Private = %d, se esperaba al menos 1", st2.Private)
	}
	if st2.Archived != 0 {
		t.Errorf("Stats.Archived = %d tras restaurar, se esperaba 0", st2.Archived)
	}
	if st2.Teams < 1 {
		t.Errorf("Stats.Teams = %d, se esperaba al menos 1", st2.Teams)
	}
}
