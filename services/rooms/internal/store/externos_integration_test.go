// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Personas de OTRA organización en una conversación (ADR 0040) contra PostgreSQL
// real. Lo que se fija son las verjas: quien viene de fuera lee y escribe en la
// conversación a la que le añadieron —y en ninguna otra—, desde su organización la
// encuentra, y la búsqueda entre organizaciones no ve nada más que eso.
package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestPersonaDeOtraOrganizacion_DirectoYCanal(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	acme := tenantCtx("acme")
	const bea = "bea" // de la organización «otra»

	tm, publico := seedTeamChannel(t, acme, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(acme, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}

	// Sin marca de externa, alguien de fuera no entra en un DM del equipo.
	if _, err := st.OpenGroupDirect(acme, tm.ID, "alice", []string{bea}, nil); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("DM con alguien que no es del equipo y sin externos: err=%v", err)
	}
	fuera, err := st.SubjectsFueraDelEquipo(acme, tm.ID, []string{"bob", bea})
	if err != nil || len(fuera) != 1 || fuera[0] != bea {
		t.Fatalf("SubjectsFueraDelEquipo = %v (%v), quiero [bea]", fuera, err)
	}

	dm, err := st.OpenGroupDirect(acme, tm.ID, "alice", []string{bea}, map[string]string{bea: "Bea de Otra"})
	if err != nil {
		t.Fatalf("OpenGroupDirect con externa: %v", err)
	}
	// Desde la 0041 entra al aceptar la invitación (invitaciones_integration_test.go).
	if _, err := st.AceptarInvitacion(context.Background(), bea, "otra", dm.ID); err != nil {
		t.Fatalf("aceptar el DM: %v", err)
	}
	// Lee y escribe en SU conversación…
	if _, err := st.GetChannel(acme, dm.ID, bea); err != nil {
		t.Fatalf("la externa no lee su DM: %v", err)
	}
	respuesta, err := st.CreateMessage(acme, store.NewMessageRecord{ChannelID: dm.ID, AuthorSub: bea, AuthorName: "Bea", Body: "hola, Alice"})
	if err != nil {
		t.Fatalf("la externa no escribe en su DM: %v", err)
	}
	// Después de lo suyo, para que cuente como no leído (escribir marca leído).
	if _, err := st.CreateMessage(acme, store.NewMessageRecord{ChannelID: dm.ID, AuthorSub: "alice", Body: "¿qué tal?"}); err != nil {
		t.Fatalf("mensaje de alice: %v", err)
	}
	// …y en ninguna otra del equipo.
	if _, err := st.GetChannel(acme, publico.ID, bea); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("la externa lee un canal público del equipo: err=%v", err)
	}
	miembros, err := st.ListChannelMembers(acme, dm.ID, "alice")
	if err != nil {
		t.Fatalf("ListChannelMembers: %v", err)
	}
	for _, m := range miembros {
		if (m.Subject == bea) != m.External {
			t.Fatalf("marca de externa mal puesta: %+v", m)
		}
		if m.Subject == bea && m.Name != "Bea de Otra" {
			t.Fatalf("el nombre de la externa es el que dio Sytue ID: %q", m.Name)
		}
	}

	// Desde SU organización: encuentra dónde vive la conversación y la lista.
	if got, err := st.AnfitrionDeCanal(context.Background(), bea, "otra", dm.ID); err != nil || got != "acme" {
		t.Fatalf("AnfitrionDeCanal = %q (%v)", got, err)
	}
	if got, err := st.AnfitrionDeMensaje(context.Background(), bea, "otra", respuesta.ID); err != nil || got != "acme" {
		t.Fatalf("AnfitrionDeMensaje = %q (%v)", got, err)
	}
	// Alguien del equipo no «cambia» a ningún sitio: la conversación es de la suya.
	if got, _ := st.AnfitrionDeCanal(context.Background(), "alice", "acme", dm.ID); got != "" {
		t.Fatalf("alice no es externa: %q", got)
	}
	canales, leidos, err := st.ListExternalChannels(context.Background(), bea, "otra")
	if err != nil || len(canales) != 1 || canales[0].ID != dm.ID || !canales[0].External {
		t.Fatalf("ListExternalChannels = %+v (%v)", canales, err)
	}
	if canales[0].Name != "Creador" || len(leidos) != 1 || leidos[0].Unread != 1 {
		t.Fatalf("nombre por visor y no leídos: canal=%q leidos=%+v", canales[0].Name, leidos)
	}
	if ch, hay, err := st.DirectoDeFuera(context.Background(), bea, "otra", []string{bea, "alice"}); err != nil || !hay || ch.ID != dm.ID {
		t.Fatalf("DirectoDeFuera: hay=%v id=%v err=%v", hay, ch.ID, err)
	}

	// La sala de vídeo NACIDA de su DM la lleva a la anfitriona; una reunión suelta
	// de esa organización, no.
	var salaDelDM, reunionSuelta string
	if err := h.Pool.InTenantTx(acme, func(tx pgx.Tx) error {
		if err := tx.QueryRow(acme,
			`INSERT INTO rooms (tenant, name, title, metadata)
			 VALUES ('acme', 'acme_dm', 'DM', jsonb_build_object('source', jsonb_build_object('kind', 'chat', 'ref_id', $1::text)))
			 RETURNING id::text`, dm.ID).Scan(&salaDelDM); err != nil {
			return err
		}
		return tx.QueryRow(acme,
			`INSERT INTO rooms (tenant, name, title) VALUES ('acme', 'acme_suelta', 'Suelta') RETURNING id::text`).Scan(&reunionSuelta)
	}); err != nil {
		t.Fatalf("salas: %v", err)
	}
	if got, err := st.AnfitrionDeSala(context.Background(), bea, "otra", uuid.MustParse(salaDelDM)); err != nil || got != "acme" {
		t.Fatalf("AnfitrionDeSala(del DM) = %q (%v)", got, err)
	}
	if got, _ := st.AnfitrionDeSala(context.Background(), bea, "otra", uuid.MustParse(reunionSuelta)); got != "" {
		t.Fatalf("una reunión suelta de la anfitriona no se abre a la externa: %q", got)
	}

	// Un canal PRIVADO: el admin del equipo la añade como externa y entra en ese.
	priv, err := st.CreateChannel(acme, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "privado", Name: "Privado", Kind: domain.ChannelPrivate, CreatedBy: "alice",
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if _, err := st.AddChannelMember(acme, priv.ID, "alice", bea, ""); !errors.Is(err, domain.ErrNotMember) {
		t.Fatalf("sin decir que es de fuera sigue exigiéndose el equipo: %v", err)
	}
	if _, err := st.AddExternalChannelMember(acme, priv.ID, "bob", bea, "Bea"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("solo un admin del canal o del equipo añade gente: %v", err)
	}
	m, err := st.AddExternalChannelMember(acme, priv.ID, "alice", bea, "Bea")
	if err != nil || !m.External || !m.Pending {
		t.Fatalf("AddExternalChannelMember: %+v %v", m, err)
	}
	if _, err := st.AceptarInvitacion(context.Background(), bea, "otra", priv.ID); err != nil {
		t.Fatalf("aceptar el privado: %v", err)
	}
	if _, err := st.CreateMessage(acme, store.NewMessageRecord{ChannelID: priv.ID, AuthorSub: bea, Body: "gracias"}); err != nil {
		t.Fatalf("la externa no escribe en el privado al que la añadieron: %v", err)
	}
	// Añadir a alguien que SÍ es del equipo por la vía de fuera no le marca.
	if m, err := st.AddExternalChannelMember(acme, priv.ID, "alice", "bob", "Bob"); err != nil || m.External {
		t.Fatalf("bob es del equipo: %+v %v", m, err)
	}

	// La búsqueda entre organizaciones SOLO ve las filas de externos.
	var visibles, externas int
	if err := h.Pool.InMaintenanceTx(context.Background(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `SELECT count(*) FROM channel_members`).Scan(&visibles); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM channel_members WHERE externo`).Scan(&externas)
	}); err != nil {
		t.Fatalf("mantenimiento: %v", err)
	}
	if visibles != 2 || externas != 2 {
		t.Fatalf("en mantenimiento se ven %d filas (%d externas): solo deben verse las 2 de bea", visibles, externas)
	}
}
