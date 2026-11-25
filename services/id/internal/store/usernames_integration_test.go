// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El @usuario contra PostgreSQL real (ADR 0040). Lo que se fija es lo que rompe
// sin ruido: dos personas que se llaman igual, un nombre que suplantaría a la
// plataforma, un @usuario que acabaría siendo la dirección de OTRA persona en el
// dominio raíz, y que desde la 0125 ninguna cuenta viva se quede sin él.
package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func TestUsername_SeAsignaAlCrear(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")

	ana := usuarioConNombre(ctx, t, st, "ana@acme.example", "Ana", "Pérez")
	if ana.Username != "ana.perez" {
		t.Fatalf("@usuario por defecto = %q, quiero ana.perez", ana.Username)
	}
	// Homónima: desempata como las direcciones, no se queda con el de la primera.
	otra := usuarioConNombre(ctx, t, st, "ana@otra.example", "Ana", "Pérez")
	if otra.Username != "ana.perez2" {
		t.Fatalf("homónima = %q, quiero ana.perez2", otra.Username)
	}
	// Elegido y ocupado (sin distinguir mayúsculas): ErrConflict, no un desempate.
	_, err := st.CreateUser(ctx, nil, domain.User{Email: "x@y.example", Username: "ana.perez", Name: "X"})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("@usuario elegido y ocupado: err=%v, quiero ErrConflict", err)
	}

	// Un nombre que suplantaría a la plataforma no da @soporte ni @soporte2.
	soporte, err := st.CreateUser(ctx, nil, domain.User{Email: "soporte@acme.example", Name: "Soporte"})
	if err != nil {
		t.Fatalf("CreateUser(soporte): %v", err)
	}
	if domain.UsernameReservado(soporte.Username) || !domain.ValidUsername(soporte.Username) {
		t.Fatalf("@usuario de «Soporte» = %q: reservado o inválido", soporte.Username)
	}

	// luis@sytue.eu ya es de alguien: a otro Luis no se le puede dar @luis, porque
	// @luis ES luis@sytue.eu.
	org := orgConBuzones(ctx, t, st, "corporate")
	dueno := usuarioConNombre(ctx, t, st, "dueno@corporate.example", "Dueño", "Uno")
	if err := st.ReserveMailbox(ctx, nil, org.ID, dueno.ID, "luis@sytue.eu"); err != nil {
		t.Fatalf("ReserveMailbox: %v", err)
	}
	luis, err := st.CreateUser(ctx, nil, domain.User{Email: "luis@acme.example", Name: "Luis"})
	if err != nil {
		t.Fatalf("CreateUser(luis): %v", err)
	}
	if luis.Username != "luis2" {
		t.Fatalf("@usuario con luis@sytue.eu ajeno = %q, quiero luis2", luis.Username)
	}
	if got, err := st.GetUserByUsername(ctx, "@LUIS2"); err != nil || got.ID != luis.ID {
		t.Fatalf("GetUserByUsername normaliza: %+v %v", got, err)
	}
}

// La 0125 hace obligatorio el @usuario: rellena a quien siguiera sin él con uno que no
// pisa a nadie, y desde entonces una cuenta viva no se puede quedar sin él. La purgada
// de «Borrar mi cuenta» sí, que lo pierde a propósito. Se reaplica el SQL REAL de la
// migración con el dueño, como en producción.
func TestUsername_ObligatorioTrasLa0125(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	dueno, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dueno.Close(ctx) }()
	aplicar := func(nombre string) {
		t.Helper()
		sql, err := db.Migrations.ReadFile("migrations/0125_username_obligatorio." + nombre + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dueno.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("0125 %s: %v", nombre, err)
		}
	}
	st := store.New(h.Pool)
	st.SetSuiteDomain("sytue.eu")

	aplicar("down") // como antes: la columna admite NULL
	sin := usuarioConNombre(ctx, t, st, "sin@acme.example", "Sin", "Nombre")
	purgada := usuarioConNombre(ctx, t, st, "ida@acme.example", "Ida", "Ya")
	listo := usuarioConNombre(ctx, t, st, "listo@acme.example", "Listo", "Uno")
	fallo := "u" + strings.ReplaceAll(sin.ID, "-", "")
	// El relleno de `sin` ya es el @usuario de otra persona: tiene que desempatar.
	if _, err := dueno.Exec(ctx, `UPDATE users SET username = $2 WHERE id = $1`, listo.ID, fallo); err != nil {
		t.Fatal(err)
	}
	if _, err := dueno.Exec(ctx, `UPDATE users SET username = NULL WHERE id IN ($1, $2)`, sin.ID, purgada.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := dueno.Exec(ctx, `UPDATE users SET purged_at = now() WHERE id = $1`, purgada.ID); err != nil {
		t.Fatal(err)
	}

	aplicar("up")
	if got, _ := st.GetUserByID(ctx, sin.ID); got.Username != fallo+"1" || !domain.ValidUsername(got.Username) {
		t.Fatalf("relleno = %q, quiero %q y válido", got.Username, fallo+"1")
	}
	if got, _ := st.GetUserByID(ctx, purgada.ID); got.Username != "" {
		t.Fatalf("la purgada recibió @usuario %q", got.Username)
	}
	// Una viva ya no puede quedarse sin él.
	_, err = h.Pool.Exec(ctx, `UPDATE users SET username = NULL WHERE id = $1`, listo.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("vaciar el @usuario de una cuenta viva: err=%v, quiero check_violation", err)
	}
}
