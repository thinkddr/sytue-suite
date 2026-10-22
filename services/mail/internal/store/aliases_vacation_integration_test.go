// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de ALIAS y VACACIONES contra un PostgreSQL real (pgtest).
// Verifican: alias CRUD; unicidad global de la dirección (no secuestrar la de
// otro, ni siquiera cross-tenant); que la entrega entrante (LookupAddress)
// resuelve el alias a su cuenta dueña ADEMÁS de la primaria; enviar-como
// (AliasBelongsTo) por dueño; get/set de vacaciones; y el dedupe atómico de
// autorespuesta por remitente y ventana (ClaimAutoreply).
package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func TestAliasCRUDResolutionAndUniqueness(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")
	other := seedAccount(t, ctx, st, "bob@sytue.eu")

	// Crear un alias de la cuenta de Ada.
	al, err := st.CreateAlias(ctx, acc.ID, "ventas@sytue.eu")
	if err != nil {
		t.Fatalf("CreateAlias: %v", err)
	}
	if al.ID == uuid.Nil || al.Address != "ventas@sytue.eu" {
		t.Fatalf("alias creado = %+v", al)
	}

	// La ENTREGA ENTRANTE resuelve el alias a la cuenta de Ada, igual que su
	// primaria (sin romperla): ambos llegan a la misma cuenta/tenant.
	for _, addr := range []string{"ada@sytue.eu", "ventas@sytue.eu", "VENTAS@Sytue.eu"} {
		r, err := st.LookupAddress(ctx, addr)
		if err != nil {
			t.Fatalf("LookupAddress(%q): %v", addr, err)
		}
		if r.AccountID != acc.ID || r.Tenant != "acme" {
			t.Fatalf("LookupAddress(%q) => %v/%s, quiero la cuenta de Ada", addr, r.AccountID, r.Tenant)
		}
	}

	// Unicidad: no se puede aliasar una dirección ya en uso como PRIMARIA de otra
	// cuenta (secuestro) ni como otro alias.
	if _, err := st.CreateAlias(ctx, acc.ID, "bob@sytue.eu"); !errors.Is(err, domain.ErrAliasExists) {
		t.Fatalf("alias sobre primaria ajena = %v, quiero ErrAliasExists", err)
	}
	if _, err := st.CreateAlias(ctx, other.ID, "ventas@sytue.eu"); !errors.Is(err, domain.ErrAliasExists) {
		t.Fatalf("alias duplicado = %v, quiero ErrAliasExists", err)
	}

	// Enviar-como: solo un alias propio (o la primaria) es del usuario.
	if ok, _ := st.AliasBelongsTo(ctx, acc.ID, "ventas@sytue.eu"); !ok {
		t.Fatal("AliasBelongsTo(propio) = false, quiero true")
	}
	if ok, _ := st.AliasBelongsTo(ctx, other.ID, "ventas@sytue.eu"); ok {
		t.Fatal("AliasBelongsTo(ajeno) = true, quiero false (no es de esa cuenta)")
	}

	// Listar y borrar (owner-scoped): otra cuenta no puede borrar el alias de Ada.
	list, err := st.ListAliases(ctx, acc.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListAliases = %v (%v)", list, err)
	}
	if err := st.DeleteAlias(ctx, other.ID, al.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("borrar alias ajeno = %v, quiero ErrNotFound", err)
	}
	if err := st.DeleteAlias(ctx, acc.ID, al.ID); err != nil {
		t.Fatalf("DeleteAlias: %v", err)
	}
	// Tras borrar, la dirección deja de resolver (y vuelve a estar libre).
	if _, err := st.LookupAddress(ctx, "ventas@sytue.eu"); !errors.Is(err, domain.ErrNoSuchUser) {
		t.Fatalf("tras borrar, LookupAddress = %v, quiero ErrNoSuchUser", err)
	}
}

// El espacio de direcciones es global entre tenants: un alias en otro tenant no
// puede secuestrar una dirección primaria/alias existente en un dominio compartido.
func TestAliasUniquenessCrossTenant(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)

	ctxA := tenantCtx("acme")
	accA := seedAccount(t, ctxA, st, "ada@sytue.eu")
	ctxB := tenantCtx("globex")
	accB := seedAccount(t, ctxB, st, "otra@sytue.eu")

	// Tenant B no puede aliasar la PRIMARIA de una cuenta de tenant A (cross-tenant).
	if _, err := st.CreateAlias(ctxB, accB.ID, "ada@sytue.eu"); !errors.Is(err, domain.ErrAliasExists) {
		t.Fatalf("alias cross-tenant sobre primaria ajena = %v, quiero ErrAliasExists", err)
	}
	// A crea un alias; B no puede duplicarlo aunque esté en otro tenant.
	if _, err := st.CreateAlias(ctxA, accA.ID, "hola@sytue.eu"); err != nil {
		t.Fatalf("CreateAlias(A): %v", err)
	}
	if _, err := st.CreateAlias(ctxB, accB.ID, "hola@sytue.eu"); !errors.Is(err, domain.ErrAliasExists) {
		t.Fatalf("alias cross-tenant duplicado = %v, quiero ErrAliasExists", err)
	}
}

func TestVacationSetGetAndDedupe(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@sytue.eu")

	// Sin configurar => desactivada (cero-valor, no error).
	if v, err := st.GetVacation(ctx, acc.ID); err != nil || v.Enabled {
		t.Fatalf("GetVacation inicial = %+v (%v), quiero desactivada", v, err)
	}

	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	want := domain.Vacation{Enabled: true, Subject: "Fuera", Body: "Vuelvo el lunes", StartsAt: &start, EndsAt: &end}
	if err := st.SetVacation(ctx, acc.ID, want); err != nil {
		t.Fatalf("SetVacation: %v", err)
	}
	got, err := st.GetVacation(ctx, acc.ID)
	if err != nil {
		t.Fatalf("GetVacation: %v", err)
	}
	if !got.Enabled || got.Subject != "Fuera" || got.StartsAt == nil || !got.StartsAt.Equal(start) {
		t.Fatalf("GetVacation = %+v, quiero %+v", got, want)
	}
	// Upsert: un segundo Set reemplaza (no duplica) la fila.
	if err := st.SetVacation(ctx, acc.ID, domain.Vacation{Enabled: false, Subject: "x"}); err != nil {
		t.Fatalf("SetVacation(2): %v", err)
	}
	if g, _ := st.GetVacation(ctx, acc.ID); g.Enabled || g.EndsAt != nil {
		t.Fatalf("upsert no reemplazó: %+v", g)
	}

	// Dedupe por remitente y ventana: la 1ª reclama (true), la 2ª dentro de la
	// ventana NO (false). Otro remitente reclama de forma independiente (true).
	win := 48 * time.Hour
	if ok, err := st.ClaimAutoreply(ctx, acc.ID, "bob@x.com", win); err != nil || !ok {
		t.Fatalf("ClaimAutoreply(1) = %v (%v), quiero true", ok, err)
	}
	if ok, _ := st.ClaimAutoreply(ctx, acc.ID, "bob@x.com", win); ok {
		t.Fatal("ClaimAutoreply(2, misma ventana) = true, quiero false (dedupe)")
	}
	if ok, _ := st.ClaimAutoreply(ctx, acc.ID, "BOB@x.com", win); ok {
		t.Fatal("ClaimAutoreply case-insensitive = true, quiero false (mismo remitente)")
	}
	if ok, _ := st.ClaimAutoreply(ctx, acc.ID, "carol@x.com", win); !ok {
		t.Fatal("ClaimAutoreply(otro remitente) = false, quiero true")
	}
	// Ventana cero: siempre reclama (última respuesta ya no está «dentro»).
	if ok, _ := st.ClaimAutoreply(ctx, acc.ID, "bob@x.com", 0); !ok {
		t.Fatal("ClaimAutoreply(ventana 0) = false, quiero true (fuera de ventana)")
	}
}

// Mudar la dirección principal cuando la organización conecta su dominio propio.
//
// Sytue ID muda a toda la plantilla y encola las direcciones nuevas; hasta ahora
// el reconciliador solo abría la sesión JMAP, que no toca la cuenta que ya
// existe, y la dirección nueva no llegaba a existir en Sytue Mail: el MX
// contestaba 550 al correo del dominio que el cliente acababa de conectar. Lo
// que se fija aquí es que la mudanza deja entregando LAS DOS —la anterior queda
// como alias, que es la diferencia entre mudarse y perder el correo de antes—.
func TestMoveAccountAddress(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	acc := seedAccount(t, ctx, st, "ada@acme.sytue.eu")
	otra := seedAccount(t, ctx, st, "bob@acme.sytue.eu")

	movida, err := st.MoveAccountAddress(ctx, acc, "ada@acme.eu")
	if err != nil {
		t.Fatalf("MoveAccountAddress: %v", err)
	}
	if movida.Address != "ada@acme.eu" || movida.ID != acc.ID {
		t.Fatalf("cuenta mudada = %+v, quiero la misma cuenta en ada@acme.eu", movida)
	}
	// Las dos entregan, y las dos a la misma cuenta.
	for _, addr := range []string{"ada@acme.eu", "ada@acme.sytue.eu"} {
		r, err := st.LookupAddress(ctx, addr)
		if err != nil {
			t.Fatalf("LookupAddress(%q): %v", addr, err)
		}
		if r.AccountID != acc.ID {
			t.Fatalf("LookupAddress(%q) => %v, quiero la cuenta de Ada", addr, r.AccountID)
		}
	}

	// Idempotente: el reconciliador repite el barrido, y repetir no puede
	// convertir la dirección actual en alias de sí misma.
	if _, err := st.MoveAccountAddress(ctx, movida, "ADA@acme.eu"); err != nil {
		t.Fatalf("mudanza repetida: %v", err)
	}
	alias, err := st.ListAliases(ctx, acc.ID)
	if err != nil {
		t.Fatalf("ListAliases: %v", err)
	}
	if len(alias) != 1 || alias[0].Address != "ada@acme.sytue.eu" {
		t.Fatalf("alias = %+v, quiero solo la dirección anterior", alias)
	}

	// Promover un alias suyo a principal: deja de estar en la lista de alias, y la
	// que era principal ocupa su sitio. Estar en las dos tablas dejaría una fila
	// que «borrar» desde la pantalla de alias sin dejar de entregar.
	if _, err := st.CreateAlias(ctx, acc.ID, "ventas@acme.eu"); err != nil {
		t.Fatalf("CreateAlias: %v", err)
	}
	promovida, err := st.MoveAccountAddress(ctx, movida, "ventas@acme.eu")
	if err != nil {
		t.Fatalf("promover alias: %v", err)
	}
	if promovida.Address != "ventas@acme.eu" {
		t.Fatalf("principal = %q, quiero ventas@acme.eu", promovida.Address)
	}
	alias, err = st.ListAliases(ctx, acc.ID)
	if err != nil {
		t.Fatalf("ListAliases: %v", err)
	}
	var direcciones []string
	for _, a := range alias {
		direcciones = append(direcciones, a.Address)
	}
	if len(direcciones) != 2 {
		t.Fatalf("alias = %v, quiero las dos anteriores y no la promovida", direcciones)
	}

	// Y NO se le quita la dirección a otra persona: eso es un choque, no una
	// mudanza, y la cuenta se queda como estaba.
	if _, err := st.MoveAccountAddress(ctx, promovida, otra.Address); !errors.Is(err, domain.ErrAliasExists) {
		t.Fatalf("mudanza a la dirección de otro = %v, quiero ErrAliasExists", err)
	}
	sigue, err := st.LookupAddress(ctx, "ventas@acme.eu")
	if err != nil || sigue.AccountID != acc.ID {
		t.Fatalf("tras el choque la cuenta cambió: %v / %v", sigue, err)
	}
}
