// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración del INVARIANTE DE BAJA DE EQUIPO contra PostgreSQL real
// (pgtest): quitar a alguien de un team le revoca de verdad el acceso —lectura Y
// escritura— a los canales privados de ese team, y su canal deja de listarse.
//
// Hallazgo del comité de paridad de Rooms (docs/roadmap.md, dominio «canales»):
// RemoveTeamMember solo borraba de team_members, y las verjas de canal privado solo
// consultaban channel_members, así que un empleado dado de baja conservaba acceso
// completo entrando por el channelID. Los DM quedan FUERA a propósito (son de ámbito
// de organización). Sigue el patrón de ola15/ola18_integration_test.go.
package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// TestRemoveTeamMemberRevokesPrivateChannelAccess: bob es miembro del team y del
// canal PRIVADO; al quitarlo del team pierde lectura, escritura y listado.
func TestRemoveTeamMemberRevokesPrivateChannelAccess(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, err := st.CreateTeam(ctx, store.NewTeamRecord{
		Slug: "eq", Name: "Equipo", CreatedBy: "alice", CreatorName: "Alicia",
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	// Canal privado con bob sembrado como miembro (es la única vía hoy: JoinChannel
	// rechaza los privados — ese hueco es otro gap del informe de paridad).
	ch, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "secreto", Name: "Secreto",
		Kind: domain.ChannelPrivate, CreatedBy: "alice", Members: []string{"bob"},
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	// Antes de la baja: bob lee, escribe y ve el canal en su lista.
	if _, err := st.GetChannel(ctx, ch.ID, "bob"); err != nil {
		t.Fatalf("bob debería leer el canal privado siendo miembro: %v", err)
	}
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "bob", AuthorName: "Bob", Body: "hola",
	}); err != nil {
		t.Fatalf("bob debería poder publicar siendo miembro: %v", err)
	}

	// BAJA del team.
	if err := st.RemoveTeamMember(ctx, tm.ID, "alice", "bob"); err != nil {
		t.Fatalf("RemoveTeamMember: %v", err)
	}

	// Después: ni lectura ni escritura, aunque conozca el channelID.
	if _, err := st.GetChannel(ctx, ch.ID, "bob"); err == nil {
		t.Fatal("tras la baja del team, bob NO debe poder leer el canal privado")
	} else if !errors.Is(err, domain.ErrForbidden) && !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error inesperado al leer tras la baja: %v", err)
	}
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "bob", AuthorName: "Bob", Body: "sigo aquí",
	}); err == nil {
		t.Fatal("tras la baja del team, bob NO debe poder publicar en el canal privado")
	}

	// Y la membresía del canal no queda huérfana: alice (que sigue en el team) lo ve,
	// bob no. Con la fila viva, el canal seguiría apareciendo en su barra lateral.
	if _, err := st.ListChannels(ctx, tm.ID, "bob", false); err == nil {
		t.Fatal("tras la baja, ListChannels de bob sobre ese team debe fallar (ya no es miembro)")
	}
	mine, err := st.ListChannels(ctx, tm.ID, "alice", false)
	if err != nil {
		t.Fatalf("ListChannels(alice): %v", err)
	}
	if len(mine) == 0 {
		t.Fatal("alice sigue en el team: debe seguir viendo el canal")
	}
}
