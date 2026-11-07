// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// A quién despierta la app de iOS un mensaje nuevo, contra PostgreSQL real: a quien le
// cambia el contador del widget de Rooms (api.PendientesRooms), que NO es a quien avisa
// el Web Push. Un canal con el nivel por defecto suma todo lo no leído aunque no
// mencione a nadie; con «menciones», solo lo que le menciona (o a todo el canal); y
// silenciado, nada.
package store_test

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestDespertablesSonLosDelContadorDelWidget(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember bob: %v", err)
	}
	if _, err := st.JoinChannel(ctx, ch.ID, "bob", "Bob"); err != nil {
		t.Fatalf("JoinChannel bob: %v", err)
	}

	despertables := func(menciones []string, todos bool) []string {
		t.Helper()
		q, err := st.DespertablesForMessage(ctx, ch.ID, "alice", menciones, todos)
		if err != nil {
			t.Fatalf("DespertablesForMessage: %v", err)
		}
		return q
	}

	// alice menciona a bob: solo bob, y el autor nunca, aunque se mencione.
	if q := despertables([]string{"bob", "alice"}, false); !slices.Equal(q, []string{"bob"}) {
		t.Fatalf("mención: %v, quiero solo bob", q)
	}
	// Sin mención, en un canal con el nivel por defecto, el contador de bob se mueve:
	// se le despierta, aunque el Web Push no le avise.
	if q := despertables(nil, false); !slices.Equal(q, []string{"bob"}) {
		t.Fatalf("sin mención, nivel por defecto: %v, quiero bob", q)
	}
	if w, _ := st.PushTargetsForMessage(ctx, ch.ID, "alice", nil); len(w) != 0 {
		t.Fatalf("el Web Push sin mención en un canal público avisa a %v", w)
	}

	// Con «menciones», solo lo que le menciona a él o a todo el canal.
	menciones := domain.NotifyLevelMentions
	if _, err := st.SetChannelPref(ctx, ch.ID, "bob", "Bob", nil, nil, &menciones); err != nil {
		t.Fatalf("SetChannelPref mentions: %v", err)
	}
	if q := despertables(nil, false); len(q) != 0 {
		t.Fatalf("«menciones» sin mención: %v, quiero nadie", q)
	}
	if q := despertables([]string{"bob"}, false); !slices.Equal(q, []string{"bob"}) {
		t.Fatalf("«menciones» con @bob: %v, quiero bob", q)
	}
	if q := despertables(nil, true); !slices.Equal(q, []string{"bob"}) {
		t.Fatalf("«menciones» con @channel: %v, quiero bob", q)
	}

	// Silenciado no suma: no se despierta ni con mención.
	none := domain.NotifyLevelNone
	if _, err := st.SetChannelPref(ctx, ch.ID, "bob", "Bob", nil, nil, &none); err != nil {
		t.Fatalf("SetChannelPref none: %v", err)
	}
	if q := despertables([]string{"bob"}, true); len(q) != 0 {
		t.Fatalf("silenciado: %v, quiero nadie", q)
	}
}
