// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Integración de legal hold + eDiscovery de FICHEROS contra PostgreSQL real
// (pgtest). Verifica la INVARIANTE no destructiva: un fichero (y sus versiones) de
// un sujeto bajo hold sobrevive a la papelera + purga (manual, cascada de carpeta y
// auto-purga); sin hold se purga normal; liberar el hold restaura el ciclo. Además:
// eDiscovery encuentra lo preservado y filtra; aislamiento por org bajo el rol
// BYPASSRLS de pgtest (los predicados tenant EXPLÍCITOS son lo que aísla); y auditoría.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// seedOwnedFile sube un fichero con owner_sub y una clave de blob única (dos
// subidas del mismo nombre archivan una versión con blobs distintos).
func seedOwnedFile(t *testing.T, ctx context.Context, st *store.Store, folderID uuid.UUID, name, owner string, size int64) domain.File {
	t.Helper()
	f, err := st.CreateFile(ctx, store.NewFileRecord{
		FolderID:    folderID,
		Name:        name,
		StorageKey:  "files/" + uuid.NewString(),
		SizeBytes:   size,
		ContentType: "application/pdf",
		OwnerSub:    owner,
	})
	if err != nil {
		t.Fatalf("CreateFile(%q, owner=%q): %v", name, owner, err)
	}
	return f
}

// backdateCreatedAt envejece created_at de un fichero (para el filtro de fechas de
// eDiscovery). Escribe con el pool del harness (rol de test, tabla literal).
func backdateCreatedAt(t *testing.T, ctx context.Context, h *pgtest.Harness, id uuid.UUID, when time.Time) {
	t.Helper()
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE files SET created_at=$2 WHERE id=$1`, id, when)
		return err
	}); err != nil {
		t.Fatalf("backdate created_at %s: %v", id, err)
	}
}

// countArchivedVersions cuenta las versiones ARCHIVADAS de un fichero (para probar
// que las versiones sobreviven a un intento de purga bajo hold).
func countArchivedVersions(t *testing.T, ctx context.Context, st *store.Store, fileID uuid.UUID) int {
	t.Helper()
	vs, _, err := st.ListVersions(ctx, fileID, 0, 200)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	return len(vs)
}

// ediscoveryCount cuenta cuántos ficheros (incl. preservados) del sujeto sobreviven.
func ediscoveryCount(t *testing.T, ctx context.Context, st *store.Store, subject string) int {
	t.Helper()
	hits, err := st.EDiscoverySearch(ctx, subject, store.EDiscoveryFilter{})
	if err != nil {
		t.Fatalf("EDiscoverySearch: %v", err)
	}
	return len(hits)
}

// TestLegalHoldPreservesFileAndVersionsThroughPurge: con hold, el fichero y su
// versión archivada sobreviven a la purga manual; sin hold se purga. Liberar el
// hold restaura el ciclo (la purga vuelve a llevárselo).
func TestLegalHoldPreservesFileAndVersionsThroughPurge(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, _ := st.EnsureRoot(ctx)

	// Fichero de alice con UNA versión archivada (dos subidas del mismo nombre).
	seedOwnedFile(t, ctx, st, root.ID, "doc.pdf", "alice", 100)
	held := seedOwnedFile(t, ctx, st, root.ID, "doc.pdf", "alice", 120)
	if countArchivedVersions(t, ctx, st, held.ID) != 1 {
		t.Fatalf("preparación: quiero 1 versión archivada")
	}
	free := seedOwnedFile(t, ctx, st, root.ID, "libre.pdf", "bob", 50)

	if _, err := st.CreateHold(ctx, domain.LegalHold{Subject: "alice", Case: "C1", CreatedBy: "admin-1"}); err != nil {
		t.Fatalf("CreateHold: %v", err)
	}

	// Ambos a la papelera (soft-delete).
	if _, err := st.DeleteFile(ctx, held.ID); err != nil {
		t.Fatalf("DeleteFile held: %v", err)
	}
	if _, err := st.DeleteFile(ctx, free.ID); err != nil {
		t.Fatalf("DeleteFile free: %v", err)
	}

	// Purga manual: el de alice se NIEGA (ErrLegalHold); el de bob se purga.
	if _, err := st.PurgeFile(ctx, held.ID); !errors.Is(err, domain.ErrLegalHold) {
		t.Fatalf("PurgeFile bajo hold = %v, quiero ErrLegalHold", err)
	}
	if _, err := st.PurgeFile(ctx, free.ID); err != nil {
		t.Fatalf("PurgeFile sin hold: %v", err)
	}
	// El fichero de alice y su versión sobreviven; el de bob no.
	if countArchivedVersions(t, ctx, st, held.ID) != 1 {
		t.Fatal("la versión archivada del fichero retenido debe sobrevivir")
	}
	if got := ediscoveryCount(t, ctx, st, "alice"); got != 1 {
		t.Fatalf("fichero retenido: %d supervivientes, quiero 1", got)
	}
	if got := ediscoveryCount(t, ctx, st, "bob"); got != 0 {
		t.Fatalf("fichero libre: %d supervivientes, quiero 0 (purgado)", got)
	}

	// Liberar el hold restaura el ciclo: ahora la purga sí se lo lleva (con su versión).
	holds, _ := st.ListHolds(ctx, false)
	if len(holds) != 1 {
		t.Fatalf("holds activos = %d, quiero 1", len(holds))
	}
	if err := st.ReleaseHold(ctx, holds[0].ID, "admin-1"); err != nil {
		t.Fatalf("ReleaseHold: %v", err)
	}
	if _, err := st.PurgeFile(ctx, held.ID); err != nil {
		t.Fatalf("PurgeFile tras liberar: %v", err)
	}
	if got := ediscoveryCount(t, ctx, st, "alice"); got != 0 {
		t.Fatalf("tras liberar: %d supervivientes, quiero 0", got)
	}
}

// TestLegalHoldRefusesFolderCascade: la purga de una carpeta cascadearía a sus
// ficheros; bajo hold se niega si el subárbol guarda alguno del sujeto (preserva
// también los hermanos no retenidos). Sin hold, la purga de carpeta funciona normal.
func TestLegalHoldRefusesFolderCascade(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, _ := st.EnsureRoot(ctx)

	// Carpeta con un fichero de alice (retenido) y uno de bob (no).
	dir, err := st.CreateFolder(ctx, root.ID, "Proyecto", "alice", false)
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	sub, err := st.CreateFolder(ctx, dir.ID, "sub", "alice", false)
	if err != nil {
		t.Fatalf("CreateFolder sub: %v", err)
	}
	seedOwnedFile(t, ctx, st, sub.ID, "caso.pdf", "alice", 100) // profundo en el subárbol
	seedOwnedFile(t, ctx, st, dir.ID, "otro.pdf", "bob", 40)

	if _, err := st.CreateHold(ctx, domain.LegalHold{Subject: "alice", Case: "C2", CreatedBy: "admin-1"}); err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	if _, err := st.DeleteFolder(ctx, dir.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	// Purga de la carpeta: negada (el subárbol guarda un fichero retenido).
	if _, err := st.PurgeFolder(ctx, dir.ID); !errors.Is(err, domain.ErrLegalHold) {
		t.Fatalf("PurgeFolder bajo hold = %v, quiero ErrLegalHold", err)
	}
	if got := ediscoveryCount(t, ctx, st, "alice"); got != 1 {
		t.Fatalf("el fichero del subárbol retenido debe seguir vivo: %d", got)
	}

	// Sin hold (otra carpeta solo con fichero de bob): la purga de carpeta funciona.
	dir2, _ := st.CreateFolder(ctx, root.ID, "Temp", "bob", false)
	seedOwnedFile(t, ctx, st, dir2.ID, "borrable.pdf", "bob", 30)
	if _, err := st.DeleteFolder(ctx, dir2.ID); err != nil {
		t.Fatalf("DeleteFolder Temp: %v", err)
	}
	if _, err := st.PurgeFolder(ctx, dir2.ID); err != nil {
		t.Fatalf("PurgeFolder sin hold debe funcionar: %v", err)
	}
}

// TestLegalHoldAutoPurgeSkipsHeld: la auto-purga (worker cross-tenant) NO reclama ni
// purga los ficheros bajo hold (top-level) ni las carpetas cuyo subárbol guarda
// alguno; sí purga lo caducado no retenido. Al liberar, la auto-purga se lo lleva.
func TestLegalHoldAutoPurgeSkipsHeld(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, _ := st.EnsureRoot(ctx)

	held := seedOwnedFile(t, ctx, st, root.ID, "retenido.pdf", "alice", 100) // top-level
	free := seedOwnedFile(t, ctx, st, root.ID, "caduco.pdf", "bob", 60)      // top-level
	dir, _ := st.CreateFolder(ctx, root.ID, "Carpeta", "alice", false)       // subárbol con retenido
	seedOwnedFile(t, ctx, st, dir.ID, "dentro.pdf", "alice", 70)

	hold, err := st.CreateHold(ctx, domain.LegalHold{Subject: "alice", Case: "C3", CreatedBy: "admin-1"})
	if err != nil {
		t.Fatalf("CreateHold: %v", err)
	}

	// Todo a la papelera y envejecido 40 días.
	for _, id := range []uuid.UUID{held.ID, free.ID} {
		if _, err := st.DeleteFile(ctx, id); err != nil {
			t.Fatalf("DeleteFile: %v", err)
		}
		backdateDeletedAt(t, ctx, h, "files", id, 40*24*time.Hour)
	}
	if _, err := st.DeleteFolder(ctx, dir.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	backdateDeletedAt(t, ctx, h, "folders", dir.ID, 40*24*time.Hour)
	backdateDeletedAt(t, ctx, h, "files", dir.ID, 40*24*time.Hour) // no-op sobre files; inocuo

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	claimed, _, err := st.ClaimAndPurgeExpiredTrash(context.Background(), cutoff, 100)
	if err != nil {
		t.Fatalf("ClaimAndPurgeExpiredTrash: %v", err)
	}
	// Solo el fichero libre de bob (1 top-level). El fichero retenido y la carpeta con
	// subárbol retenido se SALTAN.
	if claimed != 1 {
		t.Fatalf("auto-purga reclamó %d, quiero 1 (solo el no retenido)", claimed)
	}
	if got := ediscoveryCount(t, ctx, st, "alice"); got != 2 {
		t.Fatalf("ficheros de alice tras auto-purga = %d, quiero 2 (ambos preservados)", got)
	}
	if got := ediscoveryCount(t, ctx, st, "bob"); got != 0 {
		t.Fatalf("fichero de bob tras auto-purga = %d, quiero 0 (purgado)", got)
	}

	// Liberar: la auto-purga ahora se lleva lo de alice también.
	if err := st.ReleaseHold(ctx, hold.ID, "admin-1"); err != nil {
		t.Fatalf("ReleaseHold: %v", err)
	}
	if _, _, err := st.ClaimAndPurgeExpiredTrash(context.Background(), cutoff, 100); err != nil {
		t.Fatalf("ClaimAndPurgeExpiredTrash tras liberar: %v", err)
	}
	if got := ediscoveryCount(t, ctx, st, "alice"); got != 0 {
		t.Fatalf("tras liberar, ficheros de alice = %d, quiero 0", got)
	}
}

// TestPurgeNoHoldUnchanged: sin ningún hold, la purga (manual y auto) borra lo
// soft-deleted como siempre (comportamiento normal INALTERADO).
func TestPurgeNoHoldUnchanged(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, _ := st.EnsureRoot(ctx)
	f := seedOwnedFile(t, ctx, st, root.ID, "efimero.pdf", "alice", 100)
	if _, err := st.DeleteFile(ctx, f.ID); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if _, err := st.PurgeFile(ctx, f.ID); err != nil {
		t.Fatalf("PurgeFile sin hold: %v", err)
	}
	if got := ediscoveryCount(t, ctx, st, "alice"); got != 0 {
		t.Fatalf("sin hold la purga debe llevarse el fichero: %d", got)
	}
}

// TestEDiscoveryFindsPreservedAndFilters: la búsqueda encuentra lo preservado
// (soft-deleted bajo hold) y filtra por nombre, tipo y rango de fechas.
func TestEDiscoveryFindsPreservedAndFilters(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.EnsureQuota(ctx, 1<<30); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, _ := st.EnsureRoot(ctx)

	old := seedOwnedFile(t, ctx, st, root.ID, "Factura antigua.pdf", "sue", 100)
	recent := seedOwnedFile(t, ctx, st, root.ID, "Factura reciente.pdf", "sue", 100)
	// Un .txt de sue (para confirmar que el filtro por content_type lo excluye).
	if _, err := st.CreateFile(ctx, store.NewFileRecord{
		FolderID: root.ID, Name: "Reunion.txt", StorageKey: "files/" + uuid.NewString(),
		SizeBytes: 20, ContentType: "text/plain", OwnerSub: "sue",
	}); err != nil {
		t.Fatalf("CreateFile txt: %v", err)
	}
	backdateCreatedAt(t, ctx, h, old.ID, time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC))
	backdateCreatedAt(t, ctx, h, recent.ID, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))

	// Preservar (borrar) la reciente bajo hold: sigue encontrándose.
	if _, err := st.CreateHold(ctx, domain.LegalHold{Subject: "sue", Case: "C4", CreatedBy: "admin-1"}); err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	if _, err := st.DeleteFile(ctx, recent.ID); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	// Por nombre: ambas facturas, incluida la preservada.
	hits, err := st.EDiscoverySearch(ctx, "sue", store.EDiscoveryFilter{Name: "Factura"})
	if err != nil {
		t.Fatalf("EDiscoverySearch(nombre): %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("nombre 'Factura' => %d, quiero 2 (incl. preservada)", len(hits))
	}
	var sawPreserved bool
	for _, hit := range hits {
		if hit.File.ID == recent.ID {
			if !hit.Preserved {
				t.Fatal("el fichero borrado bajo hold debe venir Preserved=true")
			}
			sawPreserved = true
		}
	}
	if !sawPreserved {
		t.Fatal("la búsqueda debe incluir el fichero preservado")
	}

	// Por rango de fechas: solo la antigua (2020).
	hits, err = st.EDiscoverySearch(ctx, "sue", store.EDiscoveryFilter{
		From: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("EDiscoverySearch(fecha): %v", err)
	}
	if len(hits) != 1 || hits[0].File.ID != old.ID {
		t.Fatalf("filtro de fecha 2019-2021 => %d, quiero 1 (la antigua)", len(hits))
	}

	// Por tipo: los dos PDF (no el .txt).
	hits, err = st.EDiscoverySearch(ctx, "sue", store.EDiscoveryFilter{ContentType: "application/pdf"})
	if err != nil {
		t.Fatalf("EDiscoverySearch(tipo): %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("filtro content_type=pdf => %d, quiero 2", len(hits))
	}
}

// TestComplianceOrgIsolation: bajo el rol BYPASSRLS de pgtest, el aislamiento lo
// impone el predicado tenant EXPLÍCITO. Un admin de A no ve los ficheros/holds/traza
// de B, y un hold de A (mismo subject) NO preserva el fichero de B.
func TestComplianceOrgIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctxA := tenantCtx("acme")
	ctxB := tenantCtx("globex")
	if _, err := st.EnsureQuota(ctxB, 1<<30); err != nil {
		t.Fatalf("EnsureQuota B: %v", err)
	}
	rootB, _ := st.EnsureRoot(ctxB)
	bFile := seedOwnedFile(t, ctxB, st, rootB.ID, "confidencial.pdf", "spy", 100)

	// Hold en A para el MISMO subject "spy" (org acme).
	if _, err := st.CreateHold(ctxA, domain.LegalHold{Subject: "spy", Case: "X", CreatedBy: "admin-A"}); err != nil {
		t.Fatalf("CreateHold A: %v", err)
	}
	// Búsqueda de A por "spy": 0 (el fichero de B no es visible para A).
	if got := ediscoveryCount(t, ctxA, st, "spy"); got != 0 {
		t.Fatalf("admin de A ve %d ficheros de B, quiero 0", got)
	}
	// El hold de A NO preserva el fichero de B: la purga en B funciona.
	if _, err := st.DeleteFile(ctxB, bFile.ID); err != nil {
		t.Fatalf("DeleteFile B: %v", err)
	}
	if _, err := st.PurgeFile(ctxB, bFile.ID); err != nil {
		t.Fatalf("PurgeFile B (hold de A no debe preservarlo) = %v", err)
	}
	// B no ve el hold de A.
	if holdsB, _ := st.ListHolds(ctxB, true); len(holdsB) != 0 {
		t.Fatalf("org B ve %d holds de A, quiero 0", len(holdsB))
	}
}

// TestLegalHoldAndAuditIsolation: crear hold + WriteAudit escriben en audit_log, y
// la traza está aislada por org (A no ve la de B).
func TestLegalHoldAndAuditIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctxA := tenantCtx("acme")
	ctxB := tenantCtx("globex")

	if _, err := st.CreateHold(ctxA, domain.LegalHold{Subject: "alice", Case: "C5", CreatedBy: "admin-A"}); err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	if err := st.WriteAudit(ctxA, "admin-A", "ediscovery.search", "alice", map[string]any{"case": "C5"}); err != nil {
		t.Fatalf("WriteAudit: %v", err)
	}
	entries, err := st.ListAudit(ctxA, 50)
	if err != nil {
		t.Fatalf("ListAudit A: %v", err)
	}
	var sawCreate, sawSearch bool
	for _, e := range entries {
		switch e.Action {
		case "legalhold.create":
			sawCreate = true
		case "ediscovery.search":
			sawSearch = true
		}
		if e.Actor != "admin-A" {
			t.Fatalf("actor inesperado en traza: %q", e.Actor)
		}
	}
	if !sawCreate || !sawSearch {
		t.Fatalf("faltan entradas de auditoría: create=%v search=%v", sawCreate, sawSearch)
	}
	// Aislamiento: B no ve la traza de A.
	if entriesB, _ := st.ListAudit(ctxB, 50); len(entriesB) != 0 {
		t.Fatalf("org B ve %d entradas de auditoría de A, quiero 0", len(entriesB))
	}
}
