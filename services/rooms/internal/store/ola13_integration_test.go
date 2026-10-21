// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 13 (preferencias de notificación) contra PostgreSQL
// real (pgtest): notify_level por canal (mig. 0012) como fuente de verdad —con Muted
// DERIVADO (⟺ 'none') y el `muted` legado traducido a notify_level— y las preferencias
// de notificación por usuario (user_notify_prefs) round-trip + aislamiento por tenant.
// Detrás del build-tag `integration`. Sigue el patrón de ola3/ola12_integration_test.go.
package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// TestChannelNotifyLevelDerivesMuted verifica que notify_level es la fuente de verdad:
// 'mentions' no silencia, 'none' deriva Muted, ListChannels lo refleja, y el `muted`
// legado se traduce a notify_level='none'.
func TestChannelNotifyLevelDerivesMuted(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	// Por defecto: notify_level='all', sin silenciar.
	if list, _ := st.ListChannels(ctx, tm.ID, "alice", false); findChannel(t, list, ch.ID).NotifyLevel != domain.NotifyLevelAll {
		t.Fatalf("por defecto notify_level debe ser 'all'")
	}

	// 'mentions' → no silenciado (Muted derivado false).
	mentions := domain.NotifyLevelMentions
	got, err := st.SetChannelPref(ctx, ch.ID, "alice", "Alice", nil, nil, &mentions)
	if err != nil {
		t.Fatalf("SetChannelPref mentions: %v", err)
	}
	if got.NotifyLevel != domain.NotifyLevelMentions || got.Muted {
		t.Fatalf("mentions: level=%q muted=%v, quiero mentions/false", got.NotifyLevel, got.Muted)
	}

	// 'none' → Muted derivado true.
	none := domain.NotifyLevelNone
	got, err = st.SetChannelPref(ctx, ch.ID, "alice", "Alice", nil, nil, &none)
	if err != nil {
		t.Fatalf("SetChannelPref none: %v", err)
	}
	if got.NotifyLevel != domain.NotifyLevelNone || !got.Muted {
		t.Fatalf("none: level=%q muted=%v, quiero none/true", got.NotifyLevel, got.Muted)
	}
	// ListChannels refleja notify_level y el Muted derivado.
	if c := findChannel(t, mustList(t, st, ctx, tm.ID, "alice"), ch.ID); c.NotifyLevel != domain.NotifyLevelNone || !c.Muted {
		t.Fatalf("ListChannels: level=%q muted=%v, quiero none/true", c.NotifyLevel, c.Muted)
	}

	// El `muted` legado (true) se traduce a notify_level='none'; false → 'all'.
	yes := true
	if got, err = st.SetChannelPref(ctx, ch.ID, "alice", "Alice", nil, &yes, nil); err != nil || got.NotifyLevel != domain.NotifyLevelNone || !got.Muted {
		t.Fatalf("muted legado true: level=%q muted=%v err=%v, quiero none/true", got.NotifyLevel, got.Muted, err)
	}
	no := false
	if got, err = st.SetChannelPref(ctx, ch.ID, "alice", "Alice", nil, &no, nil); err != nil || got.NotifyLevel != domain.NotifyLevelAll || got.Muted {
		t.Fatalf("muted legado false: level=%q muted=%v err=%v, quiero all/false", got.NotifyLevel, got.Muted, err)
	}
}

// TestNotifyPrefsRoundTrip verifica el round-trip de las preferencias por usuario y su
// aislamiento por tenant (RLS).
func TestNotifyPrefsRoundTrip(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	// Sin fila: por defecto vacío + DND apagado.
	p, err := st.GetNotifyPrefs(ctx, "alice")
	if err != nil || len(p.Keywords) != 0 || p.DNDStart != 0 || p.DNDEnd != 0 {
		t.Fatalf("por defecto: %+v err=%v", p, err)
	}

	// Guardar (DND que cruza medianoche 22:00→07:00) y releer.
	if _, err := st.PutNotifyPrefs(ctx, "alice", domain.NotifyPrefs{
		Keywords: []string{"deploy", "incidencia"}, DNDStart: 1320, DNDEnd: 420,
	}); err != nil {
		t.Fatalf("PutNotifyPrefs: %v", err)
	}
	got, err := st.GetNotifyPrefs(ctx, "alice")
	if err != nil || len(got.Keywords) != 2 || got.DNDStart != 1320 || got.DNDEnd != 420 {
		t.Fatalf("round-trip: %+v err=%v", got, err)
	}

	// Sobrescribir (upsert) reemplaza el conjunto.
	if _, err := st.PutNotifyPrefs(ctx, "alice", domain.NotifyPrefs{Keywords: []string{"solo"}}); err != nil {
		t.Fatalf("PutNotifyPrefs upsert: %v", err)
	}
	if got, _ := st.GetNotifyPrefs(ctx, "alice"); len(got.Keywords) != 1 || got.Keywords[0] != "solo" || got.DNDStart != 0 {
		t.Fatalf("upsert no reemplazó: %+v", got)
	}

	// Aislamiento por tenant: otro tenant no ve las prefs de acme.
	if p, err := st.GetNotifyPrefs(tenantCtx("other"), "alice"); err != nil || len(p.Keywords) != 0 {
		t.Fatalf("aislamiento tenant: %+v err=%v", p, err)
	}
}

func mustList(t *testing.T, st *store.Store, ctx context.Context, teamID uuid.UUID, subject string) []domain.Channel {
	t.Helper()
	list, err := st.ListChannels(ctx, teamID, subject, false)
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	return list
}
