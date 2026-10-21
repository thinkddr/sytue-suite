// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la GUARDA DE ÚLTIMO OWNER contra PostgreSQL real.
//
// Una organización sin ningún owner es irrecuperable desde el producto: nadie
// puede volver a promover a nadie, así que se sale de ahí editando la base a
// mano. La guarda existía desde la Wave 1, pero vivía en la capa de servicio y
// contaba los owners en una consulta APARTE de la que escribía: entre el recuento
// y la escritura no había nada que impidiera que otra petición hiciera lo mismo.
// Dos administradores degradando a dos owners distintos a la vez veían ambos «hay
// 2 owners», ambos pasaban la guarda y la organización se quedaba con cero. Es el
// residual «TOCTOU del guard de último owner» del roadmap (§6.0).
//
// Estos tests fijan las dos mitades: la regla (rechazar la mutación que deja cero
// owners, por las tres puertas que escriben en memberships) y su atomicidad (la
// segunda mutación concurrente se rechaza en vez de colarse).
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// countOwners devuelve cuántos owners tiene el tenant activo y si `sub` es uno.
func countOwners(t *testing.T, st *store.Store, ctx context.Context, sub string) (int, bool) {
	t.Helper()
	members, err := st.ListMemberships(ctx)
	if err != nil {
		t.Fatalf("ListMemberships: %v", err)
	}
	n, isOwner := 0, false
	for _, m := range members {
		if m.Role != domain.RoleOwner {
			continue
		}
		n++
		if m.Sub == sub {
			isOwner = true
		}
	}
	return n, isOwner
}

// TestLastOwnerGuardRejectsEmptyingTheOrg: con un solo owner, ni degradarlo ni
// eliminarlo; mantenerle el rol sí. Con un segundo owner, ambas cosas se permiten.
func TestLastOwnerGuardRejectsEmptyingTheOrg(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.UpsertMembership(ctx, "owner-1", domain.RoleOwner); err != nil {
		t.Fatalf("sembrar owner-1: %v", err)
	}

	if _, err := st.UpsertMembership(ctx, "owner-1", domain.RoleAdmin); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("degradar al único owner debe rechazarse, got %v", err)
	}
	if err := st.DeleteMembership(ctx, "owner-1"); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("eliminar al único owner debe rechazarse, got %v", err)
	}
	if n, _ := countOwners(t, st, ctx, "owner-1"); n != 1 {
		t.Fatalf("tras los rechazos debe seguir habiendo 1 owner, hay %d", n)
	}

	// Mantener el rol owner nunca reduce su número: se acepta sin consultar.
	if _, err := st.UpsertMembership(ctx, "owner-1", domain.RoleOwner); err != nil {
		t.Fatalf("mantener owner debe permitirse: %v", err)
	}

	// Un member cualquiera no está sujeto a la guarda.
	if _, err := st.UpsertMembership(ctx, "member-1", domain.RoleMember); err != nil {
		t.Fatalf("alta de member: %v", err)
	}
	if err := st.DeleteMembership(ctx, "member-1"); err != nil {
		t.Fatalf("baja de member: %v", err)
	}

	// Con un segundo owner, degradar y eliminar al primero se permiten.
	if _, err := st.UpsertMembership(ctx, "owner-2", domain.RoleOwner); err != nil {
		t.Fatalf("sembrar owner-2: %v", err)
	}
	if _, err := st.UpsertMembership(ctx, "owner-1", domain.RoleAdmin); err != nil {
		t.Fatalf("degradar con otro owner presente: %v", err)
	}
	if err := st.DeleteMembership(ctx, "owner-1"); err != nil {
		t.Fatalf("eliminar con otro owner presente: %v", err)
	}
	if n, _ := countOwners(t, st, ctx, "owner-2"); n != 1 {
		t.Fatalf("debe quedar owner-2 como único owner, hay %d", n)
	}
}

// TestLastOwnerGuardCoversInvitationAccept: aceptar una invitación es un UPSERT de
// membresía, así que el único owner podía auto-degradarse por esa puerta, que no
// pasa por AddMember. Se rechaza y la invitación NO se consume.
func TestLastOwnerGuardCoversInvitationAccept(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.UpsertMembership(ctx, "owner-1", domain.RoleOwner); err != nil {
		t.Fatalf("sembrar owner-1: %v", err)
	}
	if _, err := st.UpsertMemberInvitation(ctx, "jefa@acme.eu", domain.RoleMember,
		"owner-1", time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("UpsertMemberInvitation: %v", err)
	}

	_, accepted, err := st.AcceptMemberInvitation(ctx, "owner-1", "jefa@acme.eu")
	if !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("aceptar una invitación que degrada al único owner debe rechazarse, got %v (accepted=%v)", err, accepted)
	}
	if n, isOwner := countOwners(t, st, ctx, "owner-1"); n != 1 || !isOwner {
		t.Fatalf("owner-1 debe seguir siendo owner: owners=%d esOwner=%v", n, isOwner)
	}
	// El rechazo revierte la transacción entera: la invitación sigue disponible.
	invs, err := st.ListMemberInvitations(ctx)
	if err != nil {
		t.Fatalf("ListMemberInvitations: %v", err)
	}
	if len(invs) != 1 {
		t.Fatalf("la invitación no debe consumirse en el rechazo, quedan %d", len(invs))
	}
}

// TestLastOwnerGuardIsAtomic es el test del fallo real. Simula la carrera con una
// transacción abierta que ya degradó a owner-2 sin hacer commit —el estado exacto
// de una petición que YA pasó su guarda y está escribiendo— y lanza la segunda
// degradación, la de owner-1, mientras tanto.
//
// Con el FOR UPDATE de la guarda, la segunda espera a la primera y al re-evaluar
// se encuentra sola: se rechaza. Sin él no espera a nada (tocan filas distintas,
// no hay conflicto de locks), pasa su recuento sobre un estado ya obsoleto y
// commitea: cero owners.
func TestLastOwnerGuardIsAtomic(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	for _, sub := range []string{"owner-1", "owner-2"} {
		if _, err := st.UpsertMembership(ctx, sub, domain.RoleOwner); err != nil {
			t.Fatalf("sembrar %s: %v", sub, err)
		}
	}

	// Transacción en curso: degrada a owner-2 y se queda abierta. Va en SQL crudo a
	// propósito: representa la MITAD ESCRITORA de la otra petición, ya pasada su
	// guarda, que no se puede dejar a medias llamando al store.
	locked, release := make(chan struct{}), make(chan struct{})
	enCurso := make(chan error, 1)
	go func() {
		enCurso <- h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx,
				`UPDATE memberships SET role='admin' WHERE tenant='acme' AND sub='owner-2'`); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-enCurso:
		t.Fatalf("la transacción en curso terminó antes de tiempo: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("la transacción en curso no llegó a degradar a owner-2")
	}

	segunda := make(chan error, 1)
	go func() {
		_, err := st.UpsertMembership(ctx, "owner-1", domain.RoleAdmin)
		segunda <- err
	}()

	// Debe quedarse esperando el lock de las filas owner: si vuelve ya, es que
	// contó owners sin bloquearlos y la carrera está abierta.
	sinEsperar := false
	select {
	case err := <-segunda:
		sinEsperar = true
		t.Errorf("la segunda degradación no esperó a la transacción en curso (err=%v): sin FOR UPDATE ambas pasan y la organización se queda sin owners", err)
	case <-time.After(500 * time.Millisecond):
	}

	close(release)
	if err := <-enCurso; err != nil {
		t.Fatalf("transacción en curso: %v", err)
	}
	if err := <-segunda; !sinEsperar && !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("la segunda degradación debe rechazarse al re-evaluar, got %v", err)
	}

	// El invariante, que es lo que importa: nunca cero owners.
	if n, isOwner := countOwners(t, st, ctx, "owner-1"); n != 1 || !isOwner {
		t.Fatalf("la organización debe conservar exactamente a owner-1 como owner: owners=%d esOwner=%v", n, isOwner)
	}
}
