// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Regresión del deadlock de pool del Dispatcher: el Handler corría DENTRO de la
// transacción del lote, así que un handler que tocara la base (lo normal: los
// stores abren su propia InTenantTx) pedía una segunda conexión mientras la
// primera seguía retenida. Con el pool al límite nadie avanzaba, la conexión se
// quedaba «idle in transaction» y el servicio dejaba de responder tras el PRIMER
// evento.
package events_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
)

// newPool abre un pool acotado a maxConns sobre el Postgres del harness. El
// tamaño pequeño es el punto: reproduce el pool de dev (DB_MAX_CONNS) donde el
// anidamiento se convertía en deadlock seguro.
func newPool(t *testing.T, dsn string, maxConns int) *pg.Pool {
	t.Helper()
	pool, err := pg.Connect(context.Background(), pg.Config{DSN: dsn, MaxConns: maxConns})
	if err != nil {
		t.Fatalf("conectar pool de %d conexiones: %v", maxConns, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestDispatcherHandlerNoBloqueaElPool(t *testing.T) {
	h := pgtest.New(t, os.DirFS("."), "migrations")
	ctx := context.Background()

	// DOS conexiones: una la retiene el LISTEN del dispatcher y la otra es la que
	// van usando por turnos el claim y el handler. Es justo el margen que el
	// arreglo hace suficiente — con el handler dentro de la tx del lote hacían
	// falta tres (LISTEN + tx + handler) y aquí el test caducaría.
	pool := newPool(t, h.AppDSN, 2)

	ev, err := events.NewEvent(events.Channel("test"), "demo.creado", "acme", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("crear evento: %v", err)
	}
	if err := events.PublishTx(ctx, pool, ev); err != nil {
		t.Fatalf("publicar: %v", err)
	}

	visto := make(chan struct{}, 1)
	d := events.NewDispatcher(pool, func(hctx context.Context, got events.Event) error {
		// Exactamente lo que hacía el handler real de Rooms: consultar la base
		// desde dentro del handler. Antes esto se quedaba en puddle.acquire.
		var n int
		if err := pool.QueryRow(hctx, `SELECT count(*) FROM events_outbox`).Scan(&n); err != nil {
			return err
		}
		visto <- struct{}{}
		return nil
	}, events.DispatcherConfig{
		Channels:     []events.Channel{events.Channel("test")},
		BatchSize:    10,
		PollInterval: 200 * time.Millisecond,
	})

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = d.Run(runCtx) }()

	select {
	case <-visto:
	case <-time.After(20 * time.Second):
		t.Fatal("el handler nunca corrió: el dispatcher sigue reteniendo la conexión del lote")
	}

	// Y el evento queda cerrado, sin lease pendiente.
	esperar(t, pool, `SELECT count(*) FROM events_outbox
	                   WHERE processed_at IS NOT NULL AND locked_until IS NULL`, 1)
}

func TestDispatcherNoReprocesaMientrasElLeaseVive(t *testing.T) {
	h := pgtest.New(t, os.DirFS("."), "migrations")
	ctx := context.Background()
	pool := newPool(t, h.AppDSN, 4)

	ev, err := events.NewEvent(events.Channel("test"), "demo.lento", "acme", map[string]string{})
	if err != nil {
		t.Fatalf("crear evento: %v", err)
	}
	if err := events.PublishTx(ctx, pool, ev); err != nil {
		t.Fatalf("publicar: %v", err)
	}

	// Handler lento: tarda MUCHO más que el poll de respaldo. Sin lease, el
	// siguiente ciclo lo reclamaría otra vez y el efecto se duplicaría.
	entradas := make(chan struct{}, 8)
	soltar := make(chan struct{})
	d := events.NewDispatcher(pool, func(context.Context, events.Event) error {
		entradas <- struct{}{}
		<-soltar
		return nil
	}, events.DispatcherConfig{
		Channels:      []events.Channel{events.Channel("test")},
		BatchSize:     10,
		PollInterval:  100 * time.Millisecond,
		LeaseDuration: time.Minute,
	})

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = d.Run(runCtx) }()

	select {
	case <-entradas:
	case <-time.After(20 * time.Second):
		t.Fatal("el handler no llegó a correr")
	}
	// Varios ciclos de poll con el handler aún dentro: nadie más debe entrar.
	time.Sleep(time.Second)
	if len(entradas) != 0 {
		t.Fatalf("el evento se reclamó %d veces de más: el lease no protege", len(entradas))
	}
	close(soltar)
}

func TestDispatcherLiberaElLeaseAlFallar(t *testing.T) {
	h := pgtest.New(t, os.DirFS("."), "migrations")
	ctx := context.Background()
	pool := newPool(t, h.AppDSN, 4)

	ev, err := events.NewEvent(events.Channel("test"), "demo.falla", "acme", map[string]string{})
	if err != nil {
		t.Fatalf("crear evento: %v", err)
	}
	if err := events.PublishTx(ctx, pool, ev); err != nil {
		t.Fatalf("publicar: %v", err)
	}

	intentos := make(chan struct{}, 8)
	d := events.NewDispatcher(pool, func(context.Context, events.Event) error {
		intentos <- struct{}{}
		return errors.New("fallo de prueba")
	}, events.DispatcherConfig{
		Channels:      []events.Channel{events.Channel("test")},
		BatchSize:     10,
		MaxAttempts:   2,
		PollInterval:  100 * time.Millisecond,
		LeaseDuration: time.Minute,
	})

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = d.Run(runCtx) }()

	// El lease se libera al fallar, así que reintenta sin esperar a que expire,
	// y `attempts` (incrementado al reclamar) lo detiene en MaxAttempts.
	deadline := time.After(20 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-intentos:
		case <-deadline:
			t.Fatalf("solo hubo %d intentos: el lease no se liberó al fallar", i)
		}
	}
	time.Sleep(time.Second)
	if n := len(intentos); n != 0 {
		t.Fatalf("se reintentó %d veces por encima de MaxAttempts", n)
	}
	esperar(t, pool, `SELECT count(*) FROM events_outbox
	                   WHERE processed_at IS NULL AND attempts = 2 AND last_error IS NOT NULL`, 1)
}

func TestDispatcherConEntregaObligatoriaSuperaElLimiteAnterior(t *testing.T) {
	h := pgtest.New(t, os.DirFS("."), "migrations")
	pool := newPool(t, h.AppDSN, 4)
	ev, err := events.NewEvent("test", "acceso.activo", "acme", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := events.PublishTx(t.Context(), pool, ev); err != nil {
		t.Fatal(err)
	}
	// A delivery outage longer than the old ten attempts must not leave an
	// account permanently denied after its committed reactivation.
	if _, err := pool.Exec(t.Context(), `UPDATE events_outbox SET attempts=15 WHERE id=$1`, ev.ID); err != nil {
		t.Fatal(err)
	}
	d := events.NewDispatcher(pool, func(context.Context, events.Event) error { return nil }, events.DispatcherConfig{
		Channels: []events.Channel{"test"}, Types: []string{"acceso.activo"},
		MaxAttempts: -1, PollInterval: 100 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = d.Run(ctx) }()
	esperar(t, pool, `SELECT count(*) FROM events_outbox WHERE processed_at IS NOT NULL AND attempts=16`, 1)
}

func esperar(t *testing.T, pool *pg.Pool, query string, quiero int) {
	t.Helper()
	for i := 0; i < 40; i++ {
		var n int
		if err := pool.QueryRow(context.Background(), query).Scan(&n); err != nil {
			t.Fatalf("consulta de comprobación: %v", err)
		}
		if n == quiero {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("el outbox nunca alcanzó el estado esperado: %s", query)
}

// TestDispatcherPorTipoNoEsperaAlTrabajoLargo (NEG-20): con dos Dispatcher por
// tipo, un handler que no acaba no deja esperando a los eventos del otro tipo.
func TestDispatcherPorTipoNoEsperaAlTrabajoLargo(t *testing.T) {
	h := pgtest.New(t, os.DirFS("."), "migrations")
	ctx := context.Background()
	pool := newPool(t, h.AppDSN, 6)
	for _, tipo := range []string{"largo", "corto"} {
		ev, err := events.NewEvent(events.Channel("test"), tipo, "acme", map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		if err := events.PublishTx(ctx, pool, ev); err != nil {
			t.Fatal(err)
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	largo := events.NewDispatcher(pool, func(hctx context.Context, got events.Event) error {
		if got.Type != "largo" {
			t.Errorf("el Dispatcher de «largo» recibió %q", got.Type)
		}
		<-hctx.Done() // no acaba nunca
		return hctx.Err()
	}, events.DispatcherConfig{Channels: []events.Channel{"test"}, PollInterval: 100 * time.Millisecond, Types: []string{"largo"}})
	go func() { _ = largo.Run(runCtx) }()
	time.Sleep(300 * time.Millisecond)

	visto := make(chan string, 1)
	corto := events.NewDispatcher(pool, func(_ context.Context, got events.Event) error {
		visto <- got.Type
		return nil
	}, events.DispatcherConfig{Channels: []events.Channel{"test"}, PollInterval: 100 * time.Millisecond, Types: []string{"corto"}})
	go func() { _ = corto.Run(runCtx) }()
	select {
	case tipo := <-visto:
		if tipo != "corto" {
			t.Fatalf("el Dispatcher de «corto» recibió %q", tipo)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("el evento corto no se procesó mientras el largo seguía")
	}
}
