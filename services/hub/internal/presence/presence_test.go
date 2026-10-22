// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package presence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// fakeStore cuenta las llamadas al camino PostgreSQL (verifica la degradación).
type fakeStore struct {
	heartbeats, offlines, queries int
	presences                     []domain.Presence
}

func (f *fakeStore) Heartbeat(context.Context, string, domain.PresenceState, string) error {
	f.heartbeats++
	return nil
}

func (f *fakeStore) SetOffline(context.Context, string) error {
	f.offlines++
	return nil
}

func (f *fakeStore) GetPresence(context.Context, []string, time.Duration) ([]domain.Presence, error) {
	f.queries++
	return f.presences, nil
}

func (f *fakeStore) calls() int { return f.heartbeats + f.offlines + f.queries }

func testCtx() context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
}

// newCacheSvc arranca un miniredis y un servicio con cache habilitada.
func newCacheSvc(t *testing.T) (*Service, *fakeStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c, err := cache.New(context.Background(), "redis://"+mr.Addr())
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	st := &fakeStore{}
	return New(st, c, 45*time.Second), st, mr
}

// TestCachePath cubre el ciclo completo en Dragonfly: heartbeat -> online con
// estado, expiración por TTL -> offline, offline explícito -> offline; y que el
// camino cache NUNCA toca PostgreSQL.
func TestCachePath(t *testing.T) {
	svc, st, mr := newCacheSvc(t)
	ctx := testCtx()

	if err := svc.Heartbeat(ctx, "u1", domain.StateBusy, "en una llamada"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	got, err := svc.Query(ctx, []string{"u1", "u2"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Query devolvió %d items, quiero 2", len(got))
	}
	if got[0].Subject != "u1" || got[0].State != domain.StateBusy ||
		got[0].Status != "en una llamada" || got[0].LastSeen.IsZero() {
		t.Fatalf("presencia u1 = %+v", got[0])
	}
	if got[1].Subject != "u2" || got[1].State != domain.StateOffline {
		t.Fatalf("u2 sin heartbeat = %+v, quiero offline", got[1])
	}

	// La clave respeta la key convenida y expira sola (sin purga).
	if !mr.Exists("presence:acme:u1") {
		t.Fatal("falta la clave presence:acme:u1")
	}
	mr.FastForward(46 * time.Second)
	got, _ = svc.Query(ctx, []string{"u1"})
	if got[0].State != domain.StateOffline {
		t.Fatalf("tras expirar el TTL: %+v, quiero offline", got[0])
	}

	// Offline explícito borra la clave.
	if err := svc.Heartbeat(ctx, "u1", "", ""); err != nil { // "" => online
		t.Fatalf("Heartbeat: %v", err)
	}
	if err := svc.Offline(ctx, "u1"); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	got, _ = svc.Query(ctx, []string{"u1"})
	if got[0].State != domain.StateOffline {
		t.Fatalf("tras Offline: %+v, quiero offline", got[0])
	}

	if st.calls() != 0 {
		t.Fatalf("el camino cache tocó PostgreSQL: %+v", st)
	}
}

// TestFallbackPostgres verifica la degradación: con Dragonfly caído cada
// operación cae al store y sigue funcionando con la misma forma de respuesta.
func TestFallbackPostgres(t *testing.T) {
	svc, st, mr := newCacheSvc(t)
	ctx := testCtx()
	mr.SetError("dragonfly caído")

	if err := svc.Heartbeat(ctx, "u1", domain.StateOnline, ""); err != nil {
		t.Fatalf("Heartbeat degradado: %v", err)
	}
	if err := svc.Offline(ctx, "u1"); err != nil {
		t.Fatalf("Offline degradado: %v", err)
	}
	st.presences = []domain.Presence{{Subject: "u1", State: domain.StateOnline}}
	got, err := svc.Query(ctx, []string{"u1", "u2"})
	if err != nil {
		t.Fatalf("Query degradada: %v", err)
	}
	if len(got) != 2 || got[0].State != domain.StateOnline || got[1].State != domain.StateOffline {
		t.Fatalf("Query degradada = %+v", got)
	}
	if st.heartbeats != 1 || st.offlines != 1 || st.queries != 1 {
		t.Fatalf("esperaba 1 llamada de cada al store: %+v", st)
	}
}

// TestSinDragonfly verifica que con cache nil (DRAGONFLY_URL vacía) todo va
// directo a PostgreSQL: el comportamiento previo intacto.
func TestSinDragonfly(t *testing.T) {
	st := &fakeStore{presences: []domain.Presence{{Subject: "u1", State: domain.StateAway}}}
	svc := New(st, nil, 0) // ttl<=0 => default 45s
	ctx := testCtx()

	if svc.TTL() != 45*time.Second {
		t.Fatalf("TTL() = %v, quiero 45s por defecto", svc.TTL())
	}
	if err := svc.Heartbeat(ctx, "u1", domain.StateAway, ""); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if err := svc.Offline(ctx, "u1"); err != nil {
		t.Fatalf("Offline: %v", err)
	}
	got, err := svc.Query(ctx, []string{"u1", "u2", "u1", ""})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 2 || got[0].State != domain.StateAway || got[1].State != domain.StateOffline {
		t.Fatalf("Query = %+v", got)
	}
	if st.heartbeats != 1 || st.offlines != 1 || st.queries != 1 {
		t.Fatalf("esperaba 1 llamada de cada al store: %+v", st)
	}
}

// TestHeartbeatValida: la validación aplica antes de tocar cache o store.
func TestHeartbeatValida(t *testing.T) {
	svc, st, _ := newCacheSvc(t)
	ctx := testCtx()

	if err := svc.Heartbeat(ctx, "u1", "teleporting", ""); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("estado inválido: err = %v", err)
	}
	long := make([]byte, 141)
	if err := svc.Heartbeat(ctx, "u1", domain.StateOnline, string(long)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("status demasiado largo: err = %v", err)
	}
	if st.calls() != 0 {
		t.Fatalf("la validación no debe tocar el store: %+v", st)
	}
}
