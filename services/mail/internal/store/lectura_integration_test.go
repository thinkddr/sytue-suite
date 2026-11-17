// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Preferencias de lectura y remitentes de confianza en el SERVIDOR (migración
// 0023): valores por defecto del plan, persisten por cuenta, no cruzan
// organizaciones y la base rechaza lo que no es una opción.
package store_test

import (
	"errors"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func TestReadingPrefsPersistenYSonPorCuenta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	bob := seedAccount(ctx, t, st, "bob@sytue.eu")

	if p, err := st.GetReadingPrefs(ctx, ada.ID); err != nil || p != domain.ReadingPrefsPorDefecto {
		t.Fatalf("por defecto = %+v (err=%v)", p, err)
	}
	nuevas := domain.ReadingPrefs{AutoAdvance: "list", SwipeRight: "read", SwipeLeft: "none"}
	if err := st.SetReadingPrefs(ctx, ada.ID, nuevas); err != nil {
		t.Fatalf("SetReadingPrefs: %v", err)
	}
	if p, _ := st.GetReadingPrefs(ctx, ada.ID); p != nuevas {
		t.Fatalf("tras guardar = %+v", p)
	}
	if p, _ := st.GetReadingPrefs(ctx, bob.ID); p != domain.ReadingPrefsPorDefecto {
		t.Fatalf("otra cuenta heredó las preferencias: %+v", p)
	}
	if err := st.SetReadingPrefs(ctx, ada.ID, domain.ReadingPrefs{AutoAdvance: "x", SwipeRight: "read", SwipeLeft: "none"}); err == nil {
		t.Fatal("se aceptó un auto-avance desconocido")
	}
	if _, err := st.GetReadingPrefs(tenantCtx("other"), ada.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto al leer: %v", err)
	}
	if err := st.SetReadingPrefs(tenantCtx("other"), ada.ID, nuevas); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto al escribir: %v", err)
	}
}

func TestRemitentesDeConfianza(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	bob := seedAccount(ctx, t, st, "bob@sytue.eu")

	if l, err := st.ListTrustedSenders(ctx, ada.ID); err != nil || len(l) != 0 {
		t.Fatalf("al empezar = %v (err=%v)", l, err)
	}
	if addr, err := st.AddTrustedSender(ctx, ada.ID, " Boletin@Tienda.EU "); err != nil || addr != "boletin@tienda.eu" {
		t.Fatalf("AddTrustedSender = %q (err=%v)", addr, err)
	}
	// Idempotente, y sin distinguir mayúsculas.
	if _, err := st.AddTrustedSender(ctx, ada.ID, "boletin@tienda.eu"); err != nil {
		t.Fatalf("repetir: %v", err)
	}
	if _, err := st.AddTrustedSender(ctx, ada.ID, "ana@x.eu"); err != nil {
		t.Fatalf("segundo: %v", err)
	}
	if l, _ := st.ListTrustedSenders(ctx, ada.ID); !slices.Equal(l, []string{"ana@x.eu", "boletin@tienda.eu"}) {
		t.Fatalf("lista = %v", l)
	}
	if l, _ := st.ListTrustedSenders(ctx, bob.ID); len(l) != 0 {
		t.Fatalf("otra cuenta ve la lista: %v", l)
	}
	if l, _ := st.ListTrustedSenders(tenantCtx("other"), ada.ID); len(l) != 0 {
		t.Fatalf("otra organización ve la lista: %v", l)
	}
	if _, err := st.AddTrustedSender(tenantCtx("other"), ada.ID, "x@y.eu"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto al añadir: %v", err)
	}
	if err := st.RemoveTrustedSender(ctx, ada.ID, "BOLETIN@tienda.eu"); err != nil {
		t.Fatalf("RemoveTrustedSender: %v", err)
	}
	if l, _ := st.ListTrustedSenders(ctx, ada.ID); !slices.Equal(l, []string{"ana@x.eu"}) {
		t.Fatalf("tras quitar = %v", l)
	}
}
