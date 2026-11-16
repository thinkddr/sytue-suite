// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Ventana de deshacer el envío en el SERVIDOR (columna accounts.undo_send_seconds,
// migración 0020): 10 s por defecto (la que había), persiste por cuenta, no cruza
// organizaciones y la base rechaza lo que no es una de las cuatro opciones.
package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func TestUndoSendPersisteYEsPorCuenta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	bob := seedAccount(ctx, t, st, "bob@sytue.eu")

	if secs, err := st.GetUndoSendSeconds(ctx, ada.ID); err != nil || secs != 10 {
		t.Fatalf("por defecto = %d (err=%v), quiero 10", secs, err)
	}
	if err := st.SetUndoSendSeconds(ctx, ada.ID, 30); err != nil {
		t.Fatalf("SetUndoSendSeconds: %v", err)
	}
	if secs, _ := st.GetUndoSendSeconds(ctx, ada.ID); secs != 30 {
		t.Fatalf("tras guardar = %d, quiero 30", secs)
	}
	if secs, _ := st.GetUndoSendSeconds(ctx, bob.ID); secs != 10 {
		t.Fatalf("otra cuenta heredó la ventana: %d", secs)
	}
	// El CHECK es la red de abajo: un valor fuera de la lista no se guarda.
	if err := st.SetUndoSendSeconds(ctx, ada.ID, 17); err == nil {
		t.Fatal("la base aceptó una ventana de 17 s")
	}
	if secs, _ := st.GetUndoSendSeconds(ctx, ada.ID); secs != 30 {
		t.Fatalf("el valor rechazado pisó el guardado: %d", secs)
	}
}

func TestUndoSendNoCruzaOrganizaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ada := seedAccount(tenantCtx("acme"), t, st, "ada@sytue.eu")
	if _, err := st.GetUndoSendSeconds(tenantCtx("other"), ada.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto al leer: %v", err)
	}
	if err := st.SetUndoSendSeconds(tenantCtx("other"), ada.ID, 5); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto al escribir: %v", err)
	}
	if secs, _ := st.GetUndoSendSeconds(tenantCtx("acme"), ada.ID); secs != 10 {
		t.Fatalf("la ventana se pisó desde otra organización: %d", secs)
	}
}
