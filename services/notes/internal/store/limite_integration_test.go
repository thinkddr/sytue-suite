// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El límite del enlace público tiene que ser del SERVICIO, no de cada réplica.
//
// Es la única superficie de Sytue Notepads que puede visitar cualquiera sin sesión,
// y lo que tiene enfrente es adivinar tokens de cuaderno a base de peticiones.
// Con un cubo en memoria, quien reparte sus intentos entre las instancias
// multiplica su presupuesto por el número de pods —y ese número CRECE con la
// carga, que es justo lo que provoca un ataque—.
//
// Esto no se ve en producción hasta que alguien lo aprovecha: con una réplica
// funciona igual, y el `replicaCount: 2` del chart ya lo duplicaba en silencio.
package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/notes/db"
)

func TestElLimitePublicoSeComparteEntreReplicas(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)

	// Dos limitadores sobre el MISMO Postgres: dos réplicas del servicio. Con la
	// implementación en memoria cada una tendría su propio cubo lleno.
	const burst = 5
	cfg := pg.RateLimiterConfig{PerSecond: 0.0001, Burst: burst} // recarga ~nula durante el test
	replicaA := pg.NewRateLimiter(h.Pool, cfg)
	replicaB := pg.NewRateLimiter(h.Pool, cfg)

	ctx := t.Context()
	const ip = "203.0.113.7" // TEST-NET-3: nunca es de nadie

	// La primera réplica agota el presupuesto.
	for i := 0; i < burst; i++ {
		res, err := replicaA.Allow(ctx, ip)
		if err != nil {
			t.Fatalf("Allow #%d en la réplica A: %v", i+1, err)
		}
		if !res.Allowed {
			t.Fatalf("la petición %d de %d se rechazó dentro del presupuesto", i+1, burst)
		}
	}

	// LO QUE IMPORTA: la SEGUNDA réplica ya no tiene nada que dar. Si este
	// `Allowed` sale true, el límite se multiplica por el número de pods.
	res, err := replicaB.Allow(ctx, ip)
	if err != nil {
		t.Fatalf("Allow en la réplica B: %v", err)
	}
	if res.Allowed {
		t.Fatal("la segunda réplica dejó pasar con el presupuesto ya agotado: el límite es por réplica, no del servicio")
	}
}

// Y el presupuesto es POR CLIENTE: agotar el de una IP no puede dejar fuera al
// resto, que sería una denegación de servicio trivial contra un cuaderno público.
func TestElLimitePublicoNoMezclaClientes(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	lim := pg.NewRateLimiter(h.Pool, pg.RateLimiterConfig{PerSecond: 0.0001, Burst: 2})
	ctx := t.Context()

	for i := 0; i < 3; i++ {
		if _, err := lim.Allow(ctx, "203.0.113.7"); err != nil {
			t.Fatalf("Allow: %v", err)
		}
	}
	res, err := lim.Allow(ctx, "203.0.113.8")
	if err != nil {
		t.Fatalf("Allow (otra IP): %v", err)
	}
	if !res.Allowed {
		t.Fatal("una IP agotada bloqueó a otra distinta")
	}
}
