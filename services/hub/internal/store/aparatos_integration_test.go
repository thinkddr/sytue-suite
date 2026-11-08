// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El registro de aparatos de iOS (mig. 0010) contra un PostgreSQL real: el aislamiento
// por organización, el tope de UN despertar cada 5 min por aparato con DOS réplicas
// (dos pools) reservando a la vez, el barrido de pendientes, el olvido y la purga.
package store_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

func token(c string) string { return strings.Repeat(c, 32) }

func TestAparatosAisladosPorOrganizacion(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	acme, otra := tenantCtx("acme"), tenantCtx("otra")
	if err := st.RegistrarAparato(acme, "ana", "eu.sytue.mail", token("a")); err != nil {
		t.Fatal(err)
	}
	// Desde otra organización, ni se reserva ni se olvida lo de acme.
	if got, err := st.ReservarDespertar(otra, "ana", []string{"eu.sytue.mail"}, time.Minute); err != nil || len(got) != 0 {
		t.Fatalf("otra organización reserva %v (%v)", got, err)
	}
	if n, _ := st.OlvidarAparatos(otra, "ana", ""); n != 0 {
		t.Fatalf("otra organización olvida %d", n)
	}
	got, err := st.ReservarDespertar(acme, "ana", []string{"eu.sytue.mail"}, time.Minute)
	if err != nil || len(got) != 1 || got[0].Tenant != "acme" || got[0].Token != token("a") {
		t.Fatalf("acme reserva %v (%v)", got, err)
	}
	// Sin organización (fuera de una petición) no se ve nada: deny-by-default.
	var n int
	if err := h.Pool.QueryRow(context.Background(), `SELECT count(*) FROM aparatos_apns`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("sin organización se ven %d filas (%v)", n, err)
	}
}

// Dos réplicas reciben a la vez el mismo cambio para el mismo aparato: sale UN
// despertar, y el siguiente cambio dentro de la ventana queda pendiente para el
// barrido, que tampoco lo manda hasta que la ventana pasa. Visto en rojo quitando la
// condición de `ultimo_push` de la reserva (salían dos).
func TestUnDespertarPorAparatoConDosReplicas(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	otroPool, err := pg.Connect(context.Background(), pg.Config{DSN: h.AppDSN, MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(otroPool.Close)
	replicas := []*store.Store{store.New(h.Pool), store.New(otroPool)}
	ctx := tenantCtx("acme")
	if err := replicas[0].RegistrarAparato(ctx, "ana", "eu.sytue.calendar", token("b")); err != nil {
		t.Fatal(err)
	}
	if err := replicas[0].RegistrarAparato(ctx, "ana", "eu.sytue.one", token("c")); err != nil {
		t.Fatal(err)
	}
	bundles := []string{"eu.sytue.calendar", "eu.sytue.one"}

	var mu sync.Mutex
	enviados := map[string]int{}
	var wg sync.WaitGroup
	for range 8 {
		for _, r := range replicas {
			wg.Go(func() {
				got, err := r.ReservarDespertar(ctx, "ana", bundles, 5*time.Minute)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				for _, a := range got {
					enviados[a.Token]++
				}
				mu.Unlock()
			})
		}
	}
	wg.Wait()
	if enviados[token("b")] != 1 || enviados[token("c")] != 1 || len(enviados) != 2 {
		t.Fatalf("enviados = %v, quiero uno por aparato", enviados)
	}

	// Los cambios que llegaron detrás del primero, dentro de la ventana, quedan
	// pendientes: el barrido no los manda mientras dura, y pasada (se simula con una
	// ventana de 0 s) los recoge UNA vez aunque barran las dos réplicas.
	barrer := func(ventana time.Duration) map[string]int {
		vistos := map[string]int{}
		for _, r := range replicas {
			wg.Go(func() {
				got, err := r.BarrerPendientes(context.Background(), ventana, 100)
				if err != nil {
					t.Error(err)
				}
				mu.Lock()
				for _, a := range got {
					if a.Tenant != "acme" {
						t.Errorf("barrido de %q", a.Tenant)
					}
					vistos[a.Token]++
				}
				mu.Unlock()
			})
		}
		wg.Wait()
		return vistos
	}
	if got := barrer(5 * time.Minute); len(got) != 0 {
		t.Fatalf("el barrido manda %v dentro de la ventana", got)
	}
	if got := barrer(0); got[token("b")] != 1 || got[token("c")] != 1 || len(got) != 2 {
		t.Fatalf("barridos = %v, quiero uno por aparato", got)
	}
	// Ya sin pendientes, nada; y un cambio de Calendar solo deja pendiente su app.
	if got := barrer(0); len(got) != 0 {
		t.Fatalf("lo pendiente sale dos veces: %v", got)
	}
	if got, _ := replicas[1].ReservarDespertar(ctx, "ana", []string{"eu.sytue.calendar"}, 5*time.Minute); len(got) != 0 {
		t.Fatalf("dentro de la ventana se reserva %v", got)
	}
	if got := barrer(0); got[token("b")] != 1 || len(got) != 1 {
		t.Fatalf("barridos = %v, quiero solo el de Calendar", got)
	}
}

func TestOlvidarYPurgarAparatos(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	for _, a := range []struct{ sub, bundle, tok string }{
		{"ana", "eu.sytue.mail", token("1")},
		{"ana", "eu.sytue.drive", token("2")},
		{"ana", "eu.sytue.drive", token("3")},
		{"luis", "eu.sytue.mail", token("4")},
	} {
		if err := st.RegistrarAparato(ctx, a.sub, a.bundle, a.tok); err != nil {
			t.Fatal(err)
		}
	}
	// Uno: el de ese token. Sin token: TODOS los de la persona, y nada de los demás.
	if n, err := st.OlvidarAparatos(ctx, "ana", token("2")); err != nil || n != 1 {
		t.Fatalf("olvidar uno = %d (%v)", n, err)
	}
	if n, err := st.OlvidarAparatos(ctx, "ana", ""); err != nil || n != 2 {
		t.Fatalf("olvidar todos = %d (%v)", n, err)
	}
	if n, _ := st.OlvidarAparatos(ctx, "ana", token("4")); n != 0 {
		t.Fatal("ana olvida el aparato de luis")
	}
	// El token que Apple rechaza se borra.
	if err := st.BorrarAparato(context.Background(), store.Aparato{Tenant: "acme", Bundle: "eu.sytue.mail", Token: token("4")}); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.ReservarDespertar(ctx, "luis", []string{"eu.sytue.mail"}, 0); len(got) != 0 {
		t.Fatalf("el aparato borrado sigue: %v", got)
	}
	// La purga se lleva lo que no se ha vuelto a registrar, en todas las organizaciones.
	if err := st.RegistrarAparato(tenantCtx("otra"), "eva", "eu.sytue.notes", token("5")); err != nil {
		t.Fatal(err)
	}
	if n, err := st.PurgarAparatos(context.Background(), time.Now().Add(time.Minute)); err != nil || n != 1 {
		t.Fatalf("purga = %d (%v)", n, err)
	}
}

// Una persona no llena la tabla registrándose en bucle: pasado el tope, se olvidan los
// que llevan más tiempo sin registrarse.
func TestTopeDeAparatosPorPersona(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	for i := range store.TopeAparatosPorPersona + 5 {
		tok := strings.Repeat("0", 31) + string("0123456789abcdef"[i%16]) + strings.Repeat("f", i/16+1)
		if err := st.RegistrarAparato(ctx, "ana", "eu.sytue.mail", tok); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.ReservarDespertar(ctx, "ana", []string{"eu.sytue.mail"}, 0)
	if err != nil || len(got) != store.TopeAparatosPorPersona {
		t.Fatalf("quedan %d (%v), quiero %d", len(got), err, store.TopeAparatosPorPersona)
	}
}
