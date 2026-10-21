// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// stubRow devuelve un valor float64 fijo al escanear (simula RETURNING tokens).
type stubRow struct{ tokens float64 }

func (r stubRow) Scan(dest ...any) error {
	if len(dest) == 1 {
		if p, ok := dest[0].(*float64); ok {
			*p = r.tokens
		}
	}
	return nil
}

// stubQuerier devuelve una fila con tokens configurables.
type stubQuerier struct{ tokens float64 }

func (s stubQuerier) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (s stubQuerier) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil //nolint:nilnil // stub de test: nunca se consume el resultado.
}

func (s stubQuerier) QueryRow(context.Context, string, ...any) pgx.Row {
	return stubRow(s)
}

func TestPostgresRateLimiterAllows(t *testing.T) {
	rl := NewRateLimiter(stubQuerier{tokens: 4}, RateLimiterConfig{PerSecond: 5, Burst: 10})
	res, err := rl.Allow(context.Background(), "ip:1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Allowed || res.Remaining != 4 || res.Limit != 10 {
		t.Fatalf("resultado inesperado: %+v", res)
	}
}

func TestPostgresRateLimiterDenies(t *testing.T) {
	rl := NewRateLimiter(stubQuerier{tokens: -2}, RateLimiterConfig{PerSecond: 2, Burst: 5})
	res, err := rl.Allow(context.Background(), "ip:1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed {
		t.Fatal("tokens negativos deben denegar")
	}
	if res.RetryAfter <= 0 {
		t.Fatal("debe calcular Retry-After")
	}
}

// sinFilaUnaVez devuelve ErrNoRows en el primer QueryRow y luego «permitido».
type sinFilaUnaVez struct {
	stubQuerier
	llamadas *int
}

type filaAllowN struct{ err error }

func (f filaAllowN) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	*dest[0].(*bool) = true
	*dest[1].(*float64) = 3
	return nil
}

func (s sinFilaUnaVez) QueryRow(context.Context, string, ...any) pgx.Row {
	*s.llamadas++
	if *s.llamadas == 1 {
		return filaAllowN{err: pgx.ErrNoRows}
	}
	return filaAllowN{}
}

// Si la purga se lleva la fila entre sembrarla y gastarla, AllowN vuelve a sembrar en
// vez de contestar con un error.
func TestAllowNReintentaSiLaPurgaSeLlevaLaFila(t *testing.T) {
	n := 0
	rl := NewRateLimiter(sinFilaUnaVez{llamadas: &n}, RateLimiterConfig{PerSecond: 1, Burst: 10})
	res, err := rl.AllowN(context.Background(), "k", 2)
	if err != nil || !res.Allowed || n != 2 {
		t.Fatalf("%+v %v tras %d intentos; quiero permitido al segundo", res, err, n)
	}
}
