// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 2A de paridad de Rooms contra PostgreSQL real
// (pgtest): canales por defecto al entrar y gestión del roster de un canal. Fijan lo
// que antes era imposible: un tenant nuevo con canales, y un canal privado al que se
// puede añadir y del que se puede expulsar gente (antes nacía congelado, porque los
// miembros solo se sembraban en el INSERT de creación y JoinChannel rechaza privados).
// Sigue el patrón de offboarding/ola18_integration_test.go.
package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// TestDefaultChannelsSeedAndAutoJoin: crear un team siembra «General» y «Aleatorio», y
// cada miembro nuevo del team cae dentro de ambos —en la misma transacción del alta—.
func TestDefaultChannelsSeedAndAutoJoin(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, err := st.CreateTeam(ctx, store.NewTeamRecord{
		Slug: "eq", Name: "Equipo", CreatedBy: "alice", CreatorName: "Alicia",
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	// El creador ya tiene canales (antes el team nacía vacío).
	if slugs := channelSlugs(t, ctx, st, tm.ID, "alice"); !slugs["general"] || !slugs["aleatorio"] {
		t.Fatalf("alice no ve los canales por defecto: %v", slugs)
	}

	// Un miembro nuevo del team cae en los canales por defecto con fila propia de
	// membresía (no solo por ser públicos): la comprobamos por el roster.
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	if slugs := channelSlugs(t, ctx, st, tm.ID, "bob"); !slugs["general"] || !slugs["aleatorio"] {
		t.Fatalf("bob no ve los canales por defecto: %v", slugs)
	}
	general := channelBySlug(t, ctx, st, tm.ID, "alice", "general")
	members, err := st.ListChannelMembers(ctx, general.ID, "alice")
	if err != nil {
		t.Fatalf("ListChannelMembers: %v", err)
	}
	if !hasMember(members, "alice") || !hasMember(members, "bob") {
		t.Fatalf("roster de #general = %+v, quiero alice y bob", members)
	}

	// Idempotencia: re-incorporar a bob no rompe ni duplica.
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember idempotente: %v", err)
	}
	again, err := st.ListChannelMembers(ctx, general.ID, "alice")
	if err != nil {
		t.Fatalf("ListChannelMembers 2: %v", err)
	}
	if len(again) != len(members) {
		t.Fatalf("roster duplicado: %d filas, antes %d", len(again), len(members))
	}
}

// TestEnsureDefaultTeamMembershipSeedsChannels: la vía de auto-aprovisionamiento (el
// primer acceso de un usuario a un tenant nuevo) también deja canales, y el segundo
// usuario del tenant entra directo a ellos.
func TestEnsureDefaultTeamMembershipSeedsChannels(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, created, err := st.EnsureDefaultTeamMembership(ctx, "alice", "Alicia")
	if err != nil || !created {
		t.Fatalf("EnsureDefaultTeamMembership: created=%v err=%v", created, err)
	}
	if slugs := channelSlugs(t, ctx, st, tm.ID, "alice"); !slugs["general"] || !slugs["aleatorio"] {
		t.Fatalf("el tenant nuevo nace sin canales: %v", slugs)
	}
	// Segundo usuario: mismo team, y con membresía explícita de los por defecto.
	tm2, created2, err := st.EnsureDefaultTeamMembership(ctx, "bob", "Bob")
	if err != nil || created2 || tm2.ID != tm.ID {
		t.Fatalf("segundo usuario: created=%v team=%v err=%v", created2, tm2.ID, err)
	}
	general := channelBySlug(t, ctx, st, tm.ID, "bob", "general")
	members, err := st.ListChannelMembers(ctx, general.ID, "bob")
	if err != nil {
		t.Fatalf("ListChannelMembers(bob): %v", err)
	}
	if !hasMember(members, "bob") {
		t.Fatalf("bob no está en el roster de #general: %+v", members)
	}
}

// TestChannelMemberLifecyclePrivateChannel fija el ciclo completo del roster de un
// canal PRIVADO: añadir a un miembro del team le da lectura y escritura de verdad,
// quien no está en el team no se puede añadir, el rol de admin de canal es alcanzable
// y expulsar corta el acceso.
func TestChannelMemberLifecyclePrivateChannel(t *testing.T) {
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
	// Canal privado SIN sembrar a bob: nace congelado para él.
	ch, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "secreto", Name: "Secreto",
		Kind: domain.ChannelPrivate, CreatedBy: "alice",
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if _, err := st.GetChannel(ctx, ch.ID, "bob"); err == nil {
		t.Fatal("bob no debería acceder al privado antes de ser añadido")
	}

	// (c) Quien NO está en el team no se puede añadir: ErrNotMember, no una fila
	// muerta que las verjas de team ya rechazarían.
	if _, err := st.AddChannelMember(ctx, ch.ID, "alice", "eve", "Eve"); !errors.Is(err, domain.ErrNotMember) {
		t.Fatalf("añadir a alguien ajeno al team: err=%v, quiero ErrNotMember", err)
	}

	// (b) Añadir a un miembro del team: lee y escribe.
	m, err := st.AddChannelMember(ctx, ch.ID, "alice", "bob", "")
	if err != nil {
		t.Fatalf("AddChannelMember: %v", err)
	}
	if m.Subject != "bob" || m.Name != "Bob" || m.Role != domain.RoleMember {
		t.Fatalf("miembro devuelto = %+v, quiero bob/Bob/member (nombre heredado del team)", m)
	}
	if _, err := st.GetChannel(ctx, ch.ID, "bob"); err != nil {
		t.Fatalf("bob debería leer el canal privado tras ser añadido: %v", err)
	}
	msg, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "bob", AuthorName: "Bob", Body: "ya estoy dentro",
	})
	if err != nil {
		t.Fatalf("bob debería poder publicar tras ser añadido: %v", err)
	}
	if _, err := st.ListMessages(ctx, ch.ID, "bob", store.MessagePage{Limit: 10}); err != nil {
		t.Fatalf("bob debería leer los mensajes: %v", err)
	}

	// Idempotencia del alta y roster visible para el miembro.
	if _, err := st.AddChannelMember(ctx, ch.ID, "alice", "bob", "Bob"); err != nil {
		t.Fatalf("AddChannelMember idempotente: %v", err)
	}
	roster, err := st.ListChannelMembers(ctx, ch.ID, "bob")
	if err != nil {
		t.Fatalf("ListChannelMembers(bob): %v", err)
	}
	if len(roster) != 2 {
		t.Fatalf("roster = %+v, quiero 2 (alice y bob)", roster)
	}

	// Rol de admin de canal: antes inalcanzable (nadie escribía channel_members.role).
	// Con él, bob administra el canal sin ser admin del team.
	if _, err := st.SetChannelMemberRole(ctx, ch.ID, "alice", "bob", domain.RoleAdmin); err != nil {
		t.Fatalf("SetChannelMemberRole: %v", err)
	}
	name := "Secretísimo"
	if _, err := st.UpdateChannel(ctx, ch.ID, "bob", store.UpdateChannelRecord{Name: &name}); err != nil {
		t.Fatalf("bob (admin de canal) debería poder renombrarlo: %v", err)
	}
	// Y como admin de canal puede añadir a otros miembros del team.
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "caro", "Caro", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember caro: %v", err)
	}
	if _, err := st.AddChannelMember(ctx, ch.ID, "bob", "caro", ""); err != nil {
		t.Fatalf("bob (admin de canal) debería poder añadir a caro: %v", err)
	}

	// (d) Expulsar corta el acceso: ni lee, ni escribe, ni le queda membresía.
	if err := st.RemoveChannelMember(ctx, ch.ID, "alice", "bob"); err != nil {
		t.Fatalf("RemoveChannelMember: %v", err)
	}
	if _, err := st.GetChannel(ctx, ch.ID, "bob"); err == nil {
		t.Fatal("tras la expulsión, bob NO debe poder leer el canal privado")
	} else if !errors.Is(err, domain.ErrForbidden) && !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error inesperado al leer tras la expulsión: %v", err)
	}
	if _, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "bob", AuthorName: "Bob", Body: "sigo aquí",
	}); err == nil {
		t.Fatal("tras la expulsión, bob NO debe poder publicar")
	}
	if _, err := st.ListMessages(ctx, ch.ID, "bob", store.MessagePage{Limit: 10}); err == nil {
		t.Fatalf("tras la expulsión, bob NO debe leer los mensajes (había %s)", msg.ID)
	}
	// Volver a expulsarlo es ErrNotFound (ya no es miembro).
	if err := st.RemoveChannelMember(ctx, ch.ID, "alice", "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expulsar a un no-miembro: err=%v, quiero ErrNotFound", err)
	}
}

// TestChannelMemberAuthzGates fija las puertas: un miembro raso no gestiona el roster,
// cualquiera puede salirse solo, y los DM no tienen roster gestionable (un admin del
// team no puede colarse en una conversación ajena).
func TestChannelMemberAuthzGates(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, err := st.CreateTeam(ctx, store.NewTeamRecord{
		Slug: "eq", Name: "Equipo", CreatedBy: "alice", CreatorName: "Alicia",
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	for _, sub := range []string{"bob", "caro"} {
		if err := st.AddTeamMember(ctx, tm.ID, "alice", sub, sub, domain.RoleMember); err != nil {
			t.Fatalf("AddTeamMember %s: %v", sub, err)
		}
	}
	general := channelBySlug(t, ctx, st, tm.ID, "alice", "general")

	// bob es miembro raso del canal por defecto: no gestiona el roster.
	if _, err := st.AddChannelMember(ctx, general.ID, "bob", "caro", "Caro"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("miembro raso añadiendo: err=%v, quiero ErrForbidden", err)
	}
	if err := st.RemoveChannelMember(ctx, general.ID, "bob", "caro"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("miembro raso expulsando: err=%v, quiero ErrForbidden", err)
	}
	if _, err := st.SetChannelMemberRole(ctx, general.ID, "bob", "caro", domain.RoleAdmin); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("miembro raso cambiando rol: err=%v, quiero ErrForbidden", err)
	}
	// Pero sí puede salirse él mismo (equivale a LeaveChannel).
	if err := st.RemoveChannelMember(ctx, general.ID, "bob", "bob"); err != nil {
		t.Fatalf("autoservicio de salida: %v", err)
	}

	// DM entre bob y caro: alice, ADMIN DEL TEAM, no puede añadirse ni añadir a nadie.
	dm, err := st.OpenDirect(ctx, tm.ID, "bob", "caro")
	if err != nil {
		t.Fatalf("OpenDirect: %v", err)
	}
	if _, err := st.AddChannelMember(ctx, dm.ID, "alice", "alice", "Alicia"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin del team colándose en un DM: err=%v, quiero ErrForbidden", err)
	}
	if err := st.RemoveChannelMember(ctx, dm.ID, "alice", "caro"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin del team expulsando de un DM: err=%v, quiero ErrForbidden", err)
	}
	if _, err := st.GetChannel(ctx, dm.ID, "alice"); err == nil {
		t.Fatal("alice no debe poder leer el DM de bob y caro")
	}
	// Salir de un DM sí es autoservicio.
	if err := st.RemoveChannelMember(ctx, dm.ID, "caro", "caro"); err != nil {
		t.Fatalf("caro debería poder salir de su DM: %v", err)
	}
}

// TestChannelMembersTenantIsolation: el roster de otro tenant no existe (RLS ⇒ 404),
// ni siquiera conociendo el channelID.
func TestChannelMembersTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")

	tmA, err := st.CreateTeam(acme, store.NewTeamRecord{
		Slug: "eq", Name: "Equipo", CreatedBy: "alice", CreatorName: "Alicia",
	})
	if err != nil {
		t.Fatalf("CreateTeam acme: %v", err)
	}
	chA := channelBySlug(t, acme, st, tmA.ID, "alice", "general")
	if _, err := st.CreateTeam(globex, store.NewTeamRecord{
		Slug: "eq", Name: "Equipo", CreatedBy: "gwen", CreatorName: "Gwen",
	}); err != nil {
		t.Fatalf("CreateTeam globex: %v", err)
	}

	if _, err := st.ListChannelMembers(globex, chA.ID, "gwen"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto al listar el roster: err=%v, quiero ErrNotFound", err)
	}
	if _, err := st.AddChannelMember(globex, chA.ID, "gwen", "gwen", "Gwen"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto al añadir: err=%v, quiero ErrNotFound", err)
	}
	if err := st.RemoveChannelMember(globex, chA.ID, "gwen", "alice"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto al expulsar: err=%v, quiero ErrNotFound", err)
	}
}

// --- Helpers -----------------------------------------------------------------

// channelSlugs proyecta los slugs de los canales visibles para un usuario en un team.
func channelSlugs(t *testing.T, ctx context.Context, st *store.Store, teamID uuid.UUID, subject string) map[string]bool {
	t.Helper()
	list, err := st.ListChannels(ctx, teamID, subject, false)
	if err != nil {
		t.Fatalf("ListChannels(%s): %v", subject, err)
	}
	out := map[string]bool{}
	for _, c := range list {
		out[c.Slug] = true
	}
	return out
}

// channelBySlug localiza un canal del team por su slug (falla si no está).
func channelBySlug(t *testing.T, ctx context.Context, st *store.Store, teamID uuid.UUID, subject, slug string) domain.Channel {
	t.Helper()
	list, err := st.ListChannels(ctx, teamID, subject, false)
	if err != nil {
		t.Fatalf("ListChannels(%s): %v", subject, err)
	}
	for _, c := range list {
		if c.Slug == slug {
			return c
		}
	}
	t.Fatalf("no hay canal con slug %q para %s", slug, subject)
	return domain.Channel{}
}

func hasMember(members []domain.Member, subject string) bool {
	for _, m := range members {
		if m.Subject == subject {
			return true
		}
	}
	return false
}
