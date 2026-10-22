// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El @usuario contra PostgreSQL real (ADR 0040). Lo que se fija es lo que rompe
// sin ruido: dos personas que se llaman igual, un nombre que suplantaría a la
// plataforma, un @usuario que acabaría siendo la dirección de OTRA persona en el
// dominio raíz, y el barrido que da nombre a todo el que existía antes.
package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestUsername_SeAsignaAlCrear(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")

	ana := usuarioConNombre(t, ctx, st, "ana@acme.example", "Ana", "Pérez")
	if ana.Username != "ana.perez" {
		t.Fatalf("@usuario por defecto = %q, quiero ana.perez", ana.Username)
	}
	// Homónima: desempata como las direcciones, no se queda con el de la primera.
	otra := usuarioConNombre(t, ctx, st, "ana@otra.example", "Ana", "Pérez")
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
	org := orgConBuzones(t, ctx, st, "corporate")
	dueno := usuarioConNombre(t, ctx, st, "dueno@corporate.example", "Dueño", "Uno")
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

// El barrido da @usuario a quien no lo tenía, y quien tiene dirección en el
// dominio raíz se queda con SU parte local aunque otra persona con el mismo
// nombre sea más antigua.
func TestUsername_BarridoDePendientes(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")

	antigua := usuarioConNombre(t, ctx, st, "ana@acme.example", "Ana", "Ruiz")
	personal := usuarioConNombre(t, ctx, st, "ana@gmail.example", "Ana", "Gómez")
	org, err := st.CreateOrg(ctx, nil, domain.Organization{
		Slug: domain.PersonalSlug("ana@gmail.example"), Name: "ana@gmail.example", Kind: domain.OrgKindPersonal,
	})
	if err != nil {
		t.Fatalf("CreateOrg personal: %v", err)
	}
	if err := st.ReserveMailbox(ctx, nil, org.ID, personal.ID, "ana@sytue.eu"); err != nil {
		t.Fatalf("ReserveMailbox: %v", err)
	}
	// Como antes de la mig 0065: nadie tiene @usuario.
	if _, err := st.DB().Exec(ctx, `UPDATE users SET username = NULL`); err != nil {
		t.Fatalf("vaciar @usuario: %v", err)
	}
	// «Ana Ruiz» se llamaría ana.ruiz; se fuerza el choque con la raíz «ana» dándole
	// a la antigua el nombre corto, que es el caso que importa.
	if _, err := st.DB().Exec(ctx, `UPDATE users SET given_name = 'Ana', family_name = '', name = 'Ana' WHERE id = $1`, antigua.ID); err != nil {
		t.Fatalf("renombrar: %v", err)
	}

	n, err := st.AsignarUsernamesPendientes(ctx, 100)
	if err != nil || n != 2 {
		t.Fatalf("barrido: n=%d err=%v, quiero 2", n, err)
	}
	if got, _ := st.GetUserByID(ctx, personal.ID); got.Username != "ana" {
		t.Fatalf("la de ana@sytue.eu = %q, quiero ana", got.Username)
	}
	if got, _ := st.GetUserByID(ctx, antigua.ID); got.Username != "ana2" {
		t.Fatalf("la otra Ana = %q, quiero ana2", got.Username)
	}
	// Idempotente: la segunda pasada no encuentra nada.
	if n, err := st.AsignarUsernamesPendientes(ctx, 100); err != nil || n != 0 {
		t.Fatalf("segunda pasada: n=%d err=%v", n, err)
	}
}
