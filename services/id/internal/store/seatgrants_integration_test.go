// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Plazas regaladas, contra PostgreSQL real.
//
// Se podían dar desde la consola y no se podían retirar desde ninguna parte: la
// API existía y ninguna pantalla la llamaba, así que un gesto comercial de hace
// dos años seguía sumando aforo sin que nadie se enterara. Lo que se prueba aquí
// es lo que hace falta para que la pantalla nueva no mienta: que retirar deje de
// contar, que no se pueda retirar dos veces, y que la retirada diga a QUIÉN se
// le quitaron y cuántas —el registro solo tenía el uuid de la concesión, que
// nadie va a resolver seis meses después—.
package store_test

import (
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func TestRetirarPlazasRegaladas(t *testing.T) {
	st, ctx := newStore(t)
	org, _ := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	admin := seedUser(t, ctx, st, "global@sytue.eu")

	dar := func(seats int, kind, motivo string) store.SeatGrant {
		t.Helper()
		g, err := st.CreateSeatGrant(ctx, nil, store.SeatGrant{
			OrgID: org.ID, Seats: seats, Kind: kind, Reason: motivo, GrantedBy: admin.ID,
		})
		if err != nil {
			t.Fatalf("CreateSeatGrant: %v", err)
		}
		return g
	}
	gesto := dar(10, "goodwill", "migración desde Google")
	dar(3, "employee", "equipo de soporte")

	// Las dos cuentan para el aforo efectivo.
	antes, err := st.GetPlatformOrg(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetPlatformOrg: %v", err)
	}
	if antes.SeatsGranted != 13 {
		t.Fatalf("plazas regaladas = %d, quiero 13", antes.SeatsGranted)
	}

	// Retirar devuelve la concesión: es lo que necesita quien audita para poder
	// escribir a quién y cuántas, en vez del uuid a secas.
	retirada, err := st.RevokeSeatGrant(ctx, nil, gesto.ID)
	if err != nil {
		t.Fatalf("RevokeSeatGrant: %v", err)
	}
	if retirada.OrgID != org.ID || retirada.Seats != 10 || retirada.Reason != "migración desde Google" {
		t.Errorf("la retirada no dice qué se quitó: %+v", retirada)
	}
	if retirada.RevokedAt == nil {
		t.Error("la concesión vuelve sin fecha de retirada")
	}

	// Y deja de sumar aforo, que es el punto de todo esto.
	despues, err := st.GetPlatformOrg(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetPlatformOrg: %v", err)
	}
	if despues.SeatsGranted != 3 {
		t.Errorf("tras retirar 10, quedan %d plazas regaladas; quiero 3", despues.SeatsGranted)
	}

	// Retirarla otra vez NO es idempotente en silencio: si lo fuera, un doble
	// clic diría «hecho» dos veces y la segunda no habría hecho nada.
	if _, err := st.RevokeSeatGrant(ctx, nil, gesto.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("segunda retirada: err=%v, quiero ErrNotFound", err)
	}

	// El histórico conserva la retirada: la pantalla la enseña apagada, y sin
	// esto una concesión retirada sería indistinguible de una que nunca existió.
	lista, err := st.ListSeatGrants(ctx, org.ID)
	if err != nil || len(lista) != 2 {
		t.Fatalf("ListSeatGrants = %d filas (%v), quiero 2", len(lista), err)
	}
	var vistas int
	for _, g := range lista {
		if g.ID == gesto.ID && g.RevokedAt != nil {
			vistas++
		}
	}
	if vistas != 1 {
		t.Error("la concesión retirada desapareció del histórico")
	}
}

// Una concesión caducada tampoco suma. Se prueba porque la caducidad la aplica
// la CONSULTA del aforo, no un barrido: si alguien la moviera a un job, esto
// seguiría pasando hasta el día en que el job no corriera.
func TestPlazasCaducadasNoSuman(t *testing.T) {
	st, ctx := newStore(t)
	org, _ := st.CreateOrg(ctx, nil, domain.Organization{Slug: "temporal", Name: "Temporal"})
	admin := seedUser(t, ctx, st, "global2@sytue.eu")

	ayer := time.Now().Add(-24 * time.Hour)
	if _, err := st.CreateSeatGrant(ctx, nil, store.SeatGrant{
		OrgID: org.ID, Seats: 25, Kind: "trial", Reason: "prueba de dos semanas",
		GrantedBy: admin.ID, ExpiresAt: &ayer,
	}); err != nil {
		t.Fatalf("CreateSeatGrant: %v", err)
	}
	o, err := st.GetPlatformOrg(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetPlatformOrg: %v", err)
	}
	if o.SeatsGranted != 0 {
		t.Errorf("una concesión vencida ayer aporta %d plazas: la caducidad no se evalúa al leer", o.SeatsGranted)
	}
}
