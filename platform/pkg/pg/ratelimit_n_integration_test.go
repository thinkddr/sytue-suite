// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package pg_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
)

// AllowN cobra n fichas de golpe y solo si las hay todas: lo que no cabe no gasta nada,
// y a la vez no se reparten más de las que hay.
func TestAllowN(t *testing.T) {
	h := pgtest.New(t, os.DirFS("."), "migrations")
	ctx := context.Background()
	rl := pg.NewRateLimiter(h.Pool, pg.RateLimiterConfig{PerSecond: 0.001, Burst: 10})

	if r, err := rl.AllowN(ctx, "k", 11); err != nil || r.Allowed {
		t.Fatalf("11 de 10: %+v %v, quiero no", r, err)
	}
	if r, err := rl.AllowN(ctx, "k", 7); err != nil || !r.Allowed {
		t.Fatalf("7 de 10 tras un cobro que no cupo: %+v %v, quiero sí (no gastó nada)", r, err)
	}
	if r, err := rl.AllowN(ctx, "k", 4); err != nil || r.Allowed {
		t.Fatalf("4 de los 3 que quedan: %+v %v, quiero no", r, err)
	}
	if r, err := rl.AllowN(ctx, "k", 3); err != nil || !r.Allowed {
		t.Fatalf("3 de 3: %+v %v, quiero sí", r, err)
	}

	// Un reloj que va por detrás del de otra réplica no hace retroceder el cubo: ni
	// updated_at vuelve atrás ni el tiempo «negativo» quita fichas.
	futuro := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	if _, err := h.Pool.Exec(ctx, `INSERT INTO platform_rate_limits (bucket_key, tokens, updated_at) VALUES ('reloj', 5, $1)`, futuro); err != nil {
		t.Fatal(err)
	}
	for _, f := range []func() (httpx.RateLimitResult, error){
		func() (httpx.RateLimitResult, error) { return rl.AllowN(ctx, "reloj", 2) },
		func() (httpx.RateLimitResult, error) { return rl.Allow(ctx, "reloj") },
	} {
		if r, err := f(); err != nil || !r.Allowed {
			t.Fatalf("con el reloj de otra réplica por delante: %+v %v, quiero sí", r, err)
		}
	}
	var tokens float64
	var cuando time.Time
	if err := h.Pool.QueryRow(ctx, `SELECT tokens, updated_at FROM platform_rate_limits WHERE bucket_key = 'reloj'`).Scan(&tokens, &cuando); err != nil {
		t.Fatal(err)
	}
	if tokens != 2 || !cuando.Equal(futuro) {
		t.Fatalf("tras 3 fichas de 5: tokens %v y updated_at %v; quiero 2 y %v", tokens, cuando, futuro)
	}

	// Veinte a la vez de 1 contra un cubo nuevo de 10: pasan 10, ni una más.
	var pasan atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if r, err := rl.AllowN(ctx, "nuevo", 1); err == nil && r.Allowed {
				pasan.Add(1)
			}
		})
	}
	wg.Wait()
	if n := pasan.Load(); n != 10 {
		t.Fatalf("a la vez pasaron %d, quiero 10", n)
	}
}
