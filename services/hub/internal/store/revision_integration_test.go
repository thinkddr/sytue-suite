// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	hubdb "git.sytue.eu/sytue/one/services/hub/db"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/notify"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

// Lo que dejó la revisión de seguridad de los avisos de la persona (ADR 0053), contra
// Postgres de verdad.
func TestAvisosRevision(t *testing.T) {
	h := pgtest.New(t, hubdb.Migrations, hubdb.MigrationsDir)
	st := store.New(h.Pool)
	svc := notify.NewService(st, notify.NewHub(1))
	acme, globex := tenantCtx("acme"), tenantCtx("globex")

	t.Run("de un aviso solo se cambia si está leído", func(t *testing.T) {
		n, err := st.CreateNotification(acme, sampleNotification("bea", "original"))
		if err != nil {
			t.Fatal(err)
		}
		err = h.Pool.InTenantTx(acme, func(tx pgx.Tx) error {
			_, e := tx.Exec(context.Background(), `UPDATE notifications SET title = 'otro' WHERE id = $1`, n.ID)
			return e
		})
		if err == nil {
			t.Fatal("se pudo cambiar el título de un aviso")
		}
		if err := st.MarkRead(domain.ConBuzonesPersonales(acme, []string{"bea"}, time.Now()), n.ID, []string{"bea"}); err != nil {
			t.Fatalf("marcar como leído: %v", err)
		}
	})

	t.Run("lo que calla en una organización calla lo que le llega de otra", func(t *testing.T) {
		if err := st.SetNotificationPref(globex, "carla", domain.NotificationPref{Source: domain.SourceDrive, Muted: true}); err != nil {
			t.Fatal(err)
		}
		n, err := svc.Create(acme, sampleNotification("carla", "desde acme"))
		if err != nil {
			t.Fatal(err)
		}
		if n.ID.String() != "00000000-0000-0000-0000-000000000000" {
			t.Errorf("carla calló Drive en globex y le llegó uno de acme: %+v", n)
		}
		if l, _ := st.ListNotifications(domain.ConBuzonesPersonales(globex, []string{"carla"}, time.Now()), []string{"carla"}, false, nil, 50); len(l) != 0 {
			t.Errorf("en su campana: %+v", l)
		}
	})

	t.Run("una organización no puede llenar la campana de nadie", func(t *testing.T) {
		for i := range notify.TopeAvisosPorHora {
			if _, err := svc.Create(acme, sampleNotification("dani", "uno")); err != nil {
				t.Fatalf("aviso %d: %v", i, err)
			}
		}
		if _, err := svc.Create(acme, sampleNotification("dani", "de más")); !errors.Is(err, domain.ErrTooMany) {
			t.Fatalf("pasado el tope: %v", err)
		}
		if _, err := svc.Create(globex, sampleNotification("dani", "de otra")); err != nil {
			t.Fatalf("el tope es por emisor, y globex no ha mandado nada: %v", err)
		}
		otroTipo := sampleNotification("dani", "una invitación")
		otroTipo.Type = "invite"
		if _, err := svc.Create(acme, otroTipo); err != nil {
			t.Fatalf("el tope es por tipo, y agotar uno calla los demás: %v", err)
		}
		// Y variar el tipo no esquiva el total.
		for i := range notify.TopeAvisosTotalPorHora {
			n := sampleNotification("gus", "variado")
			n.Type = fmt.Sprintf("t%d", i)
			if _, err := svc.Create(acme, n); err != nil {
				t.Fatalf("aviso variado %d: %v", i, err)
			}
		}
		n := sampleNotification("gus", "uno más")
		n.Type = "otro-mas"
		if _, err := svc.Create(acme, n); !errors.Is(err, domain.ErrTooMany) {
			t.Fatalf("variando el tipo se pasa el total: %v", err)
		}
	})

	t.Run("una dirección retirada se lleva sus avisos de todas las organizaciones", func(t *testing.T) {
		for _, org := range []context.Context{acme, globex} {
			if _, err := st.CreateNotification(org, sampleNotification("email:vieja@acme.test", "para la vieja")); err != nil {
				t.Fatal(err)
			}
		}
		if n, err := st.OlvidarBuzon(context.Background(), "email:vieja@acme.test"); err != nil || n != 2 {
			t.Fatalf("olvidados %d (%v), quiero 2", n, err)
		}
		nueva := domain.ConBuzonesPersonales(globex, []string{"otra", "email:vieja@acme.test"}, time.Now())
		if l, _ := st.ListNotifications(nueva, []string{"otra", "email:vieja@acme.test"}, false, nil, 50); len(l) != 0 {
			t.Errorf("quien recibe la dirección hereda: %+v", l)
		}
	})

	t.Run("desde una organización ve los avisos a su buzón de otra", func(t *testing.T) {
		// Eva es de acme (eva@acme.test) y de globex (eva@globex.test): su token de
		// globex trae las dos (claim `mailboxes`). Una tercera la invita a la de acme.
		if _, err := st.CreateNotification(tenantCtx("initech"), sampleNotification("email:eva@acme.test", "invitación")); err != nil {
			t.Fatal(err)
		}
		antes := time.Now().Add(-time.Minute)
		enGlobex := domain.ConBuzonesPersonales(globex, []string{"eva", "email:eva@globex.test", "email:eva@acme.test"}, antes)
		if c, _ := st.UnreadCount(enGlobex, []string{"eva", "email:eva@globex.test"}); c != 1 {
			t.Fatalf("desde globex, eva cuenta %d avisos a su buzón de acme; quiero 1", c)
		}
		// Sin esa dirección en el token (ya no es miembro de acme), no la ve.
		sinAcme := domain.ConBuzonesPersonales(globex, []string{"eva", "email:eva@globex.test"}, time.Now())
		if c, _ := st.UnreadCount(sinAcme, []string{"eva", "email:eva@globex.test"}); c != 0 {
			t.Fatalf("fuera de acme, eva sigue viendo %d avisos a su buzón de allí", c)
		}
		// Sytue ID retira la dirección: un token de ANTES ya no la usa, aunque la traiga.
		if _, err := st.OlvidarBuzon(context.Background(), "email:eva@acme.test"); err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateNotification(tenantCtx("initech"), sampleNotification("email:eva@acme.test", "para quien la recibe")); err != nil {
			t.Fatal(err)
		}
		if c, _ := st.UnreadCount(enGlobex, []string{"eva", "email:eva@globex.test"}); c != 0 {
			t.Fatalf("con un token anterior a la retirada, eva ve %d avisos del nuevo titular", c)
		}
		// Quien la recibe después, con un token nuevo, sí.
		nuevo := domain.ConBuzonesPersonales(acme, []string{"fran", "email:eva@acme.test"}, time.Now().Add(time.Second))
		if c, _ := st.UnreadCount(nuevo, []string{"fran", "email:eva@acme.test"}); c != 1 {
			t.Fatalf("el nuevo titular cuenta %d, quiero 1", c)
		}
	})
}
