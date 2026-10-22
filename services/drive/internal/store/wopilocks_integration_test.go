// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	db "git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// TestWopiLockExclusion comprueba contra PostgreSQL real lo que el bloqueo promete: que
// DOS editores no pueden tenerlo a la vez.
//
// Se prueba aquí y no solo con un fake porque la exclusión no la hace el código Go: la
// hace el `ON CONFLICT (file_id) DO UPDATE ... WHERE` dentro del índice único. Un fake
// siempre «funciona»; lo que puede fallar es la sentencia.
func TestWopiLockExclusion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	f := seedFile(t, ctx, st, root.ID, "informe.docx", 10)

	// El primer editor lo toma.
	if _, err := st.AcquireWopiLock(ctx, f.ID, "editor-A", "u1", time.Hour); err != nil {
		t.Fatalf("primer LOCK: %v", err)
	}

	// El segundo, con OTRO identificador: la sentencia no debe concederlo.
	if _, err := st.AcquireWopiLock(ctx, f.ID, "editor-B", "u2", time.Hour); err != store.ErrLockConflict {
		t.Fatalf("segundo LOCK = %v, quiero ErrLockConflict", err)
	}

	// El primero refresca el SUYO: sí se concede (es el caso REFRESH_LOCK).
	if _, err := st.AcquireWopiLock(ctx, f.ID, "editor-A", "u1", time.Hour); err != nil {
		t.Fatalf("refresco del propio: %v", err)
	}

	// Soltar con el identificador de otro NO debe funcionar: si funcionara, cualquiera
	// podría liberar el fichero que otro tiene abierto y sobrescribirlo.
	if err := st.ReleaseWopiLock(ctx, f.ID, "editor-B"); err != store.ErrLockConflict {
		t.Fatalf("UNLOCK ajeno = %v, quiero ErrLockConflict", err)
	}

	// Con el suyo, sí.
	if err := st.ReleaseWopiLock(ctx, f.ID, "editor-A"); err != nil {
		t.Fatalf("UNLOCK propio: %v", err)
	}
	if _, err := st.GetWopiLock(ctx, f.ID); err != domain.ErrNotFound {
		t.Fatalf("tras soltar debería no haber lock; err=%v", err)
	}
}

// TestWopiLockCaducadoSeReclama: un editor puede morirse sin soltar el suyo. Sin
// caducidad, ese fichero quedaría inmovilizado para siempre y sin forma de recuperarlo.
func TestWopiLockCaducadoSeReclama(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	f := seedFile(t, ctx, st, root.ID, "hoja.xlsx", 10)

	// TTL negativo ⇒ nace caducado (el reloj es el de PostgreSQL, no el de Go).
	if _, err := st.AcquireWopiLock(ctx, f.ID, "editor-muerto", "u1", -time.Minute); err != nil {
		t.Fatalf("LOCK caducado: %v", err)
	}
	// No se ve como vivo…
	if _, err := st.GetWopiLock(ctx, f.ID); err != domain.ErrNotFound {
		t.Fatalf("un lock caducado no debe verse vivo; err=%v", err)
	}
	// …y otro editor puede reclamarlo.
	if _, err := st.AcquireWopiLock(ctx, f.ID, "editor-nuevo", "u2", time.Hour); err != nil {
		t.Fatalf("reclamar un lock caducado: %v", err)
	}
}

// TestWopiLockAisladoPorOrganizacion: el id de fichero no basta para llegar a un bloqueo
// de otra organización.
func TestWopiLockAisladoPorOrganizacion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	f := seedFile(t, ctx, st, root.ID, "privado.docx", 10)
	if _, err := st.AcquireWopiLock(ctx, f.ID, "editor-A", "u1", time.Hour); err != nil {
		t.Fatalf("LOCK: %v", err)
	}
	if _, err := st.GetWopiLock(tenantCtx("otra"), f.ID); err != domain.ErrNotFound {
		t.Fatal("otra organización vio el bloqueo: el aislamiento por RLS no se aplica")
	}
}
