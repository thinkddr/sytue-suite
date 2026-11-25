// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// «Marcar como no leído» devuelve el aviso a sin leer y el contador lo nota; el de otra
// persona es ErrNotFound y no cambia. Y «Marcar todas» con un producto solo toca los de
// ese producto: con «Drive» elegido, el correo sin leer sigue sin leer.
func TestNoLeidoYMarcarTodasDeUnProducto(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	ana := domain.ConBuzonesPersonales(tenantCtx("acme"), "ana", nil, time.Now())
	bea := domain.ConBuzonesPersonales(tenantCtx("acme"), "bea", nil, time.Now())

	deDrive, err := st.CreateNotification(ana, sampleNotification("ana", "compartido"))
	if err != nil {
		t.Fatal(err)
	}
	correo := sampleNotification("ana", "correo nuevo")
	correo.Source, correo.Type = domain.SourceMail, "message"
	deCorreo, err := st.CreateNotification(ana, correo)
	if err != nil {
		t.Fatal(err)
	}
	deBea, err := st.CreateNotification(bea, sampleNotification("bea", "de bea"))
	if err != nil {
		t.Fatal(err)
	}

	if n, err := st.MarkAllRead(ana, []string{"ana"}, string(domain.SourceDrive)); err != nil || n != 1 {
		t.Fatalf("marcar las de Drive: %d (%v), quiero 1", n, err)
	}
	if sinLeer, _ := st.UnreadCount(ana, []string{"ana"}); sinLeer != 1 {
		t.Fatalf("sin leer tras marcar las de Drive: %d, quiero 1 (el correo)", sinLeer)
	}
	// Lo que hay de verdad en la tabla, sin la RLS de por medio.
	sinRLS := h.PoolSinRLS(t)
	leida := func(id uuid.UUID) bool {
		t.Helper()
		var read bool
		if err := sinRLS.QueryRow(ana, `SELECT read FROM notifications WHERE id=$1`, id).Scan(&read); err != nil {
			t.Fatal(err)
		}
		return read
	}
	if !leida(deDrive.ID) || leida(deCorreo.ID) {
		t.Fatal("«Marcar todas» de Drive tocó lo que no era de Drive")
	}

	if err := st.MarkUnread(ana, deDrive.ID, []string{"ana"}); err != nil {
		t.Fatalf("marcar como no leído: %v", err)
	}
	if err := st.MarkUnread(ana, deDrive.ID, []string{"ana"}); err != nil {
		t.Fatalf("marcar como no leído otra vez (idempotente): %v", err)
	}
	if leida(deDrive.ID) {
		t.Fatal("el aviso sigue leído")
	}
	if sinLeer, _ := st.UnreadCount(ana, []string{"ana"}); sinLeer != 2 {
		t.Fatalf("sin leer: %d, quiero 2", sinLeer)
	}
	if err := st.MarkRead(bea, deBea.ID, []string{"bea"}); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkUnread(ana, deBea.ID, []string{"ana"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("el de otra persona: %v, quiero ErrNotFound", err)
	}
	if !leida(deBea.ID) {
		t.Fatal("cambió el aviso de otra persona")
	}
}
