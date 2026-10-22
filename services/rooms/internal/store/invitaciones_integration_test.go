// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Decidido por el dueño el 2026-10-16: «Que acepte primero», también en DM y grupos.
// Antes, a una persona de otra organización se la añadía y entraba directamente: leía
// el canal y le llegaban sus mensajes sin haber dicho que sí. Ahora recibe una
// invitación, no ve nada del canal hasta aceptarla, y puede rechazarla. Y quien ya
// estaba dentro antes de la 0041 sigue dentro.
//
// Visto en rojo contra main: Bea leía el canal nada más añadirla.
func TestLaGenteDeFueraAceptaAntesDeEntrar(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	acme, bg := tenantCtx("acme"), context.Background()
	const bea, carla = "bea", "carla" // de las organizaciones «otra» y «tercera»

	tm, publico := seedTeamChannel(t, acme, st, "alice", domain.ChannelPublic)
	priv, err := st.CreateChannel(acme, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "proyecto", Name: "Proyecto", Kind: domain.ChannelPrivate, CreatedBy: "alice",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Carla ya estaba en el canal público antes de la 0041 (una fila `externo` como
	// las que había): se queda dentro.
	if err := h.Pool.InTenantTx(acme, func(tx pgx.Tx) error {
		_, err := tx.Exec(acme, `INSERT INTO channel_members (tenant, channel_id, subject, name, role, externo)
			VALUES ('acme', $1, $2, 'Carla', 'member', true)`, publico.ID, carla)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetChannel(acme, publico.ID, carla); err != nil {
		t.Fatalf("quien ya estaba dentro se queda: %v", err)
	}

	// Añadir a Bea al privado es invitarla: no lee, no escribe, no lo tiene en su lista.
	m, err := st.AddExternalChannelMember(acme, priv.ID, "alice", bea, "Bea")
	if err != nil || !m.Pending {
		t.Fatalf("AddExternalChannelMember: %+v %v", m, err)
	}
	fuera := func(cuando string) {
		t.Helper()
		if _, err := st.GetChannel(acme, priv.ID, bea); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("%s: Bea lee el canal (err=%v)", cuando, err)
		}
		if _, err := st.CreateMessage(acme, store.NewMessageRecord{ChannelID: priv.ID, AuthorSub: bea, Body: "hola"}); err == nil {
			t.Errorf("%s: Bea escribe en el canal", cuando)
		}
		if canales, _, _ := st.ListExternalChannels(bg, bea, "otra"); len(canales) != 0 {
			t.Errorf("%s: el canal sale en la lista de Bea: %+v", cuando, canales)
		}
		if got, _ := st.AnfitrionDeCanal(bg, bea, "otra", priv.ID); got != "" {
			t.Errorf("%s: las rutas del canal la llevan a la anfitriona", cuando)
		}
	}
	fuera("invitada")
	inv, err := st.Invitaciones(bg, bea, "otra")
	if err != nil || len(inv) != 1 || inv[0].ChannelID != priv.ID || inv[0].Nombre != "Proyecto" ||
		inv[0].Organizacion != "acme" || inv[0].InvitadoPor != "alice" {
		t.Fatalf("Invitaciones de Bea = %+v (%v)", inv, err)
	}

	// Rechazar: sigue fuera y la invitación desaparece.
	if err := st.RechazarInvitacion(bg, bea, "otra", priv.ID); err != nil {
		t.Fatal(err)
	}
	fuera("tras rechazar")
	if inv, _ := st.Invitaciones(bg, bea, "otra"); len(inv) != 0 {
		t.Fatalf("la invitación rechazada sigue: %+v", inv)
	}
	if _, err := st.AceptarInvitacion(bg, bea, "otra", priv.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aceptar una invitación rechazada: %v", err)
	}

	// Invitada otra vez y acepta: dentro.
	if _, err := st.AddExternalChannelMember(acme, priv.ID, "alice", bea, "Bea"); err != nil {
		t.Fatal(err)
	}
	if got, err := st.AceptarInvitacion(bg, bea, "otra", priv.ID); err != nil || got != "acme" {
		t.Fatalf("AceptarInvitacion = %q (%v)", got, err)
	}
	if _, err := st.GetChannel(acme, priv.ID, bea); err != nil {
		t.Fatalf("tras aceptar, Bea lee el canal: %v", err)
	}

	// Un DM con alguien de fuera, igual: invitación. Y si ella abre el mismo DM desde su
	// organización, lo acepta en vez de abrir una conversación nueva.
	dm, err := st.OpenGroupDirect(acme, tm.ID, "alice", []string{carla}, map[string]string{carla: "Carla"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetChannel(acme, dm.ID, carla); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Carla lee el DM sin aceptarlo: %v", err)
	}
	if ch, hay, err := st.AceptarDirectoPendiente(bg, carla, "tercera", []string{carla, "alice"}); err != nil || !hay || ch.ID != dm.ID {
		t.Fatalf("AceptarDirectoPendiente: hay=%v id=%v err=%v", hay, ch.ID, err)
	}
	if _, err := st.GetChannel(acme, dm.ID, carla); err != nil {
		t.Fatalf("Carla, tras abrir el DM, lo lee: %v", err)
	}
}
