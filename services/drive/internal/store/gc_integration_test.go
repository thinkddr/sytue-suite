// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la AUTO-PURGA de papelera (ClaimAndPurgeExpiredTrash)
// contra un PostgreSQL real (efímero, via pgtest). Verifican, sobre el esquema+RLS
// reales de la mig 0009:
//   - RETENCIÓN: purga lo caducado (> N días), respeta lo reciente,
//   - GUARDA DE BLOB COMPARTIDO: NO marca huérfano un blob que otra copia viva
//     referencia (no se perderían sus bytes),
//   - IDEMPOTENCIA: una fila purgada no se re-reclama (segundo barrido vacío),
//   - CROSS-RÉPLICA: dos reclamos CONCURRENTES no solapan (FOR UPDATE SKIP LOCKED)
//     ⇒ cada elemento se purga exactamente una vez, sin doble descuento de cuota,
//   - CROSS-TENANT + RLS: un barrido purga varios tenants, cada cuota por su lado.
package store_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// backdateDeletedAt envejece el deleted_at de una fila (fichero o carpeta) para
// simular que lleva `age` en la papelera, sin esperar en el reloj.
func backdateDeletedAt(t *testing.T, ctx context.Context, h *pgtest.Harness, table string, id uuid.UUID, age time.Duration) {
	t.Helper()
	// table es un literal de test controlado ("files"/"folders"), no entrada externa.
	sql := "UPDATE " + table + " SET deleted_at = now() - make_interval(secs => $2) WHERE id = $1"
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, id, age.Seconds())
		return err
	}); err != nil {
		t.Fatalf("backdate %s %s: %v", table, id, err)
	}
}

// TestTrashGCRetention: purga lo caducado, respeta lo reciente.
func TestTrashGCRetention(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, _ := st.EnsureRoot(ctx)

	old := seedFile(t, ctx, st, root.ID, "viejo.txt", 100)
	recent := seedFile(t, ctx, st, root.ID, "reciente.txt", 100)
	if _, err := st.DeleteFile(ctx, old.ID); err != nil {
		t.Fatalf("DeleteFile viejo: %v", err)
	}
	if _, err := st.DeleteFile(ctx, recent.ID); err != nil {
		t.Fatalf("DeleteFile reciente: %v", err)
	}
	// El viejo lleva 40 días en papelera; el reciente, recién borrado.
	backdateDeletedAt(t, ctx, h, "files", old.ID, 40*24*time.Hour)

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	claimed, blobs, err := st.ClaimAndPurgeExpiredTrash(context.Background(), cutoff, 100)
	if err != nil {
		t.Fatalf("ClaimAndPurgeExpiredTrash: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("claimed=%d, quiero 1 (solo el caducado)", claimed)
	}
	if len(blobs) != 1 || !blobs[0].Orphaned || blobs[0].Tenant != "acme" || blobs[0].StorageKey != old.StorageKey {
		t.Fatalf("blob liberado inesperado: %+v", blobs)
	}
	// El reciente sigue en papelera; el viejo ya no.
	_, files, _ := st.ListTrash(ctx)
	if len(files) != 1 || files[0].ID != recent.ID {
		t.Fatalf("papelera tras purga=%v, quiero solo el reciente", files)
	}
	// Cuota: liberó los 100 bytes del viejo (200 - 100 = 100 en uso).
	if q, _ := st.GetQuota(ctx); q.UsedBytes != 100 {
		t.Fatalf("cuota tras purga=%d, quiero 100 (solo el viejo liberado)", q.UsedBytes)
	}
}

// TestTrashGCSharedBlobGuard: NO borra el blob si otra copia viva lo referencia.
func TestTrashGCSharedBlobGuard(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, _ := st.EnsureRoot(ctx)

	src := seedFile(t, ctx, st, root.ID, "informe.pdf", 100)
	// Copia server-side: comparte el MISMO blob por referencia.
	cp, err := st.CopyFile(ctx, src.ID, root.ID, "bob")
	if err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	if cp.StorageKey != src.StorageKey {
		t.Fatalf("la copia debe compartir el blob: %q != %q", cp.StorageKey, src.StorageKey)
	}
	// Se envía a papelera SOLO el original y se envejece.
	if _, err := st.DeleteFile(ctx, src.ID); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	backdateDeletedAt(t, ctx, h, "files", src.ID, 40*24*time.Hour)

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	claimed, blobs, err := st.ClaimAndPurgeExpiredTrash(context.Background(), cutoff, 100)
	if err != nil {
		t.Fatalf("ClaimAndPurgeExpiredTrash: %v", err)
	}
	if claimed != 1 || len(blobs) != 1 {
		t.Fatalf("claimed=%d blobs=%d, quiero 1/1", claimed, len(blobs))
	}
	if blobs[0].Orphaned {
		t.Fatal("el blob que la copia viva comparte NO debe marcarse huérfano (se perderían sus bytes)")
	}
	// La copia sigue viva y sigue apuntando al blob.
	if got, err := st.GetFile(ctx, cp.ID); err != nil || got.StorageKey != src.StorageKey {
		t.Fatalf("la copia debe seguir viva con su blob: %+v err=%v", got, err)
	}
}

// TestTrashGCIdempotent: una fila purgada no se re-reclama.
func TestTrashGCIdempotent(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, _ := st.EnsureRoot(ctx)
	f := seedFile(t, ctx, st, root.ID, "x.txt", 50)
	if _, err := st.DeleteFile(ctx, f.ID); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	backdateDeletedAt(t, ctx, h, "files", f.ID, 40*24*time.Hour)

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	if claimed, _, err := st.ClaimAndPurgeExpiredTrash(context.Background(), cutoff, 100); err != nil || claimed != 1 {
		t.Fatalf("primer barrido=(%d,%v), quiero (1,nil)", claimed, err)
	}
	// Segundo barrido: la fila ya no existe ⇒ nada que reclamar (idempotente).
	if claimed, _, err := st.ClaimAndPurgeExpiredTrash(context.Background(), cutoff, 100); err != nil || claimed != 0 {
		t.Fatalf("segundo barrido=(%d,%v), quiero (0,nil) idempotente", claimed, err)
	}
}

// TestTrashGCCrossReplicaNoOverlap: dos reclamos CONCURRENTES no solapan (SKIP
// LOCKED) ⇒ cada elemento se purga UNA vez, sin doble descuento de cuota.
func TestTrashGCCrossReplicaNoOverlap(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, _ := st.EnsureRoot(ctx)

	const n = 20
	for i := 0; i < n; i++ {
		f := seedFile(t, ctx, st, root.ID, "f"+string(rune('a'+i))+".txt", 10)
		if _, err := st.DeleteFile(ctx, f.ID); err != nil {
			t.Fatalf("DeleteFile: %v", err)
		}
		backdateDeletedAt(t, ctx, h, "files", f.ID, 40*24*time.Hour)
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	// Dos "réplicas" reclaman+purgan a la vez, en lotes pequeños, hasta drenar.
	var mu sync.Mutex
	total := 0
	var wg sync.WaitGroup
	for r := 0; r < 2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				claimed, _, err := st.ClaimAndPurgeExpiredTrash(context.Background(), cutoff, 3)
				if err != nil {
					t.Errorf("ClaimAndPurgeExpiredTrash: %v", err)
					return
				}
				mu.Lock()
				total += claimed
				mu.Unlock()
				if claimed == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()

	// Cada elemento purgado EXACTAMENTE una vez (sin doble trabajo entre réplicas).
	if total != n {
		t.Fatalf("purgados en total=%d, quiero %d (sin solape entre réplicas)", total, n)
	}
	if folders, files, _ := st.ListTrash(ctx); len(folders) != 0 || len(files) != 0 {
		t.Fatalf("papelera no vacía tras drenar: %d carpetas, %d ficheros", len(folders), len(files))
	}
	// Cuota liberada UNA sola vez por fila (no negativa, no doble).
	if q, _ := st.GetQuota(ctx); q.UsedBytes != 0 {
		t.Fatalf("cuota tras drenar=%d, quiero 0 (liberada una vez por fila)", q.UsedBytes)
	}
}

// TestTrashGCCrossTenant: un barrido purga varios tenants; cada cuota por su lado.
func TestTrashGCCrossTenant(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)

	for _, slug := range []string{"acme", "globex"} {
		ctx := tenantCtx(slug)
		if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
			t.Fatalf("EnsureQuota %s: %v", slug, err)
		}
		root, _ := st.EnsureRoot(ctx)
		f := seedFile(t, ctx, st, root.ID, "doc.txt", 70)
		if _, err := st.DeleteFile(ctx, f.ID); err != nil {
			t.Fatalf("DeleteFile %s: %v", slug, err)
		}
		backdateDeletedAt(t, ctx, h, "files", f.ID, 40*24*time.Hour)
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	claimed, blobs, err := st.ClaimAndPurgeExpiredTrash(context.Background(), cutoff, 100)
	if err != nil {
		t.Fatalf("ClaimAndPurgeExpiredTrash: %v", err)
	}
	if claimed != 2 {
		t.Fatalf("claimed=%d, quiero 2 (uno por tenant)", claimed)
	}
	// Cada blob liberado va etiquetado con SU tenant.
	tenants := map[string]bool{}
	for _, b := range blobs {
		tenants[b.Tenant] = true
	}
	if !tenants["acme"] || !tenants["globex"] {
		t.Fatalf("blobs liberados sin ambos tenants: %+v", blobs)
	}
	// Cada cuota liberada por su lado (aislamiento por tenant se mantuvo).
	for _, slug := range []string{"acme", "globex"} {
		if q, _ := st.GetQuota(tenantCtx(slug)); q.UsedBytes != 0 {
			t.Fatalf("cuota %s tras purga=%d, quiero 0", slug, q.UsedBytes)
		}
	}
}
