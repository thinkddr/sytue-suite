// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del aviso «solo despertar» de iOS (mig. 0032 + RLS) contra
// PostgreSQL real: alta/baja del aparato y selección de destinatarios. Lo que se
// comprueba de verdad es que el APNs y el Web Push eligen EXACTAMENTE a la misma
// gente —comparten el CTE de destinatarios—, porque si se separaran, un mensaje
// avisaría en el navegador y no en el móvil sin que nadie se enterara.
package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

const tokenBob = "abcdef0123456789abcdef0123456789"

func TestAPNSDeviceYDestinatarios(t *testing.T) {
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
	if err := st.SaveAPNSDevice(ctx, "bob", tokenBob); err != nil {
		t.Fatalf("SaveAPNSDevice: %v", err)
	}
	// Registrar dos veces el mismo aparato no lo duplica (UPSERT por token).
	if err := st.SaveAPNSDevice(ctx, "bob", tokenBob); err != nil {
		t.Fatalf("SaveAPNSDevice repetido: %v", err)
	}

	// alice menciona a bob: solo bob, y el autor nunca.
	targets, err := st.APNSTargetsForMessage(ctx, ch.ID, "alice", []string{"bob"})
	if err != nil {
		t.Fatalf("APNSTargetsForMessage: %v", err)
	}
	if len(targets) != 1 || targets[0].Subject != "bob" || targets[0].Token != tokenBob {
		t.Fatalf("mención: targets=%+v, quiero solo bob una vez", targets)
	}

	// Mismo criterio que el Web Push: el canal silenciado apaga los DOS caminos.
	none := domain.NotifyLevelNone
	if _, err := st.SetChannelPref(ctx, ch.ID, "bob", "Bob", nil, nil, &none); err != nil {
		t.Fatalf("SetChannelPref none: %v", err)
	}
	apns, _ := st.APNSTargetsForMessage(ctx, ch.ID, "alice", []string{"bob"})
	web, _ := st.PushTargetsForMessage(ctx, ch.ID, "alice", []string{"bob"})
	if len(apns) != 0 || len(web) != 0 {
		t.Fatalf("silenciado: apns=%d web=%d, quiero 0 y 0", len(apns), len(web))
	}

	// Olvidar el aparato lo saca de la lista (misma limpieza que usa el enviador
	// cuando Apple reporta el token caduco).
	mentions := domain.NotifyLevelMentions
	if _, err := st.SetChannelPref(ctx, ch.ID, "bob", "Bob", nil, nil, &mentions); err != nil {
		t.Fatalf("SetChannelPref mentions: %v", err)
	}
	if err := st.DeleteAPNSDevice(ctx, "bob", tokenBob); err != nil {
		t.Fatalf("DeleteAPNSDevice: %v", err)
	}
	if targets, _ := st.APNSTargetsForMessage(ctx, ch.ID, "alice", []string{"bob"}); len(targets) != 0 {
		t.Fatalf("tras olvidar el aparato: targets=%+v, quiero 0", targets)
	}
}
