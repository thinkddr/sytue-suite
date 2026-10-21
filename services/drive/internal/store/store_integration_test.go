// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integracion de Sytue Drive contra un PostgreSQL real (efimero, via
// platform/pkg/pgtest sobre testcontainers-go). Detras del build-tag
// `integration`: no corren en el build/CI por defecto (sin Docker). Verifican:
//   - que las migraciones aplican (esquema + RLS),
//   - carpetas/ficheros CRUD basico y contabilidad de cuota,
//   - PAGINACION KEYSET estable de ficheros (ListFilesPage),
//   - AISLAMIENTO por tenant (RLS): datos de un tenant no visibles para otro.
package store_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// seedFile sube un metadato de fichero a la carpeta dada, reservando cuota.
func seedFile(t *testing.T, ctx context.Context, st *store.Store, folderID uuid.UUID, name string, size int64) domain.File {
	t.Helper()
	f, err := st.CreateFile(ctx, store.NewFileRecord{
		FolderID:    folderID,
		Name:        name,
		StorageKey:  "files/" + name,
		SizeBytes:   size,
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("CreateFile(%q): %v", name, err)
	}
	return f
}

func TestDriveMigrationsAndQuota(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	q, err := st.EnsureQuota(ctx, 1000)
	if err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	if q.LimitBytes != 1000 || q.UsedBytes != 0 {
		t.Fatalf("cuota inicial inesperada: %+v", q)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	if !root.IsRoot {
		t.Fatal("EnsureRoot no devolvio la raiz")
	}

	seedFile(t, ctx, st, root.ID, "a.txt", 300)
	q2, _ := st.GetQuota(ctx)
	if q2.UsedBytes != 300 {
		t.Fatalf("used_bytes tras subir=%d, quiero 300", q2.UsedBytes)
	}

	// Superar la cuota debe fallar con ErrQuotaExceeded.
	_, err = st.CreateFile(ctx, store.NewFileRecord{
		FolderID: root.ID, Name: "b.txt", StorageKey: "files/b", SizeBytes: 800,
	})
	if !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("esperaba ErrQuotaExceeded, obtuve %v", err)
	}
}

// TestListFilesPageKeyset comprueba que la paginacion keyset recorre TODOS los
// ficheros en orden estable (lower(name), id), sin repetir ni saltar, y que el
// cursor termina en nil.
func TestListFilesPageKeyset(t *testing.T) {
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

	const total = 25
	for i := 0; i < total; i++ {
		seedFile(t, ctx, st, root.ID, fmt.Sprintf("file-%02d.txt", i), 1)
	}

	// Recorrer en paginas de 7; acumular y verificar cobertura completa.
	var seen []string
	cursor := store.FileCursor{}
	pages := 0
	for {
		page, err := st.ListFilesPage(ctx, root.ID, cursor, 7)
		if err != nil {
			t.Fatalf("ListFilesPage: %v", err)
		}
		pages++
		for _, f := range page.Files {
			seen = append(seen, f.Name)
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
		if pages > 100 {
			t.Fatal("paginacion no termina (posible cursor no avanzando)")
		}
	}
	if len(seen) != total {
		t.Fatalf("recorridos %d ficheros, quiero %d", len(seen), total)
	}
	// Orden estable ascendente y sin duplicados.
	for i := 1; i < len(seen); i++ {
		if strings.ToLower(seen[i-1]) >= strings.ToLower(seen[i]) {
			t.Fatalf("orden no estrictamente creciente: %q >= %q", seen[i-1], seen[i])
		}
	}
}

// TestDriveTrashFileRoundTrip comprueba el ciclo de papelera de un fichero:
// borrar (soft, la cuota SIGUE contando porque el blob se conserva) → listar
// papelera → restaurar → borrar+purgar (libera cuota, sin rastro).
func TestDriveTrashFileRoundTrip(t *testing.T) {
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
	file := seedFile(t, ctx, st, root.ID, "informe.txt", 500)

	// Soft delete: fuera del listado activo, pero la cuota se mantiene (blob vivo)
	// y aparece en la papelera.
	if _, err := st.DeleteFile(ctx, file.ID); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if q, _ := st.GetQuota(ctx); q.UsedBytes != 500 {
		t.Fatalf("cuota tras papelera=%d, quiero 500 (el blob se conserva)", q.UsedBytes)
	}
	if _, files, err := st.ListTrash(ctx); err != nil || len(files) != 1 || files[0].ID != file.ID {
		t.Fatalf("ListTrash=%v err=%v, quiero el fichero borrado", files, err)
	}

	// Restaurar: vuelve al listado activo y desaparece de la papelera.
	if _, err := st.RestoreFile(ctx, file.ID); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}
	if _, err := st.GetFile(ctx, file.ID); err != nil {
		t.Fatalf("tras restaurar, GetFile: %v", err)
	}
	if _, files, _ := st.ListTrash(ctx); len(files) != 0 {
		t.Fatalf("papelera tras restaurar debe estar vacía, got %+v", files)
	}

	// Borrar de nuevo y purgar: libera cuota, devuelve la clave S3 y no queda rastro.
	if _, err := st.DeleteFile(ctx, file.ID); err != nil {
		t.Fatalf("DeleteFile 2: %v", err)
	}
	df, err := st.PurgeFile(ctx, file.ID)
	if err != nil {
		t.Fatalf("PurgeFile: %v", err)
	}
	if df.StorageKey == "" {
		t.Fatal("PurgeFile debe devolver la clave S3 a borrar")
	}
	if q, _ := st.GetQuota(ctx); q.UsedBytes != 0 {
		t.Fatalf("cuota tras purgar=%d, quiero 0", q.UsedBytes)
	}
	if _, err := st.PurgeFile(ctx, file.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("purgar dos veces => %v, quiero ErrNotFound", err)
	}
}

// TestDriveTrashFolderSubtree comprueba que borrar una carpeta manda el subárbol
// a la papelera mostrando SOLO la carpeta (no su contenido suelto), que
// restaurarla recupera el contenido y que purgarla arrastra (cascade) los
// ficheros y libera su cuota.
func TestDriveTrashFolderSubtree(t *testing.T) {
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
	dir, err := st.CreateFolder(ctx, root.ID, "proyecto", "sub", false)
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	child := seedFile(t, ctx, st, dir.ID, "dentro.txt", 300)

	if _, err := st.DeleteFolder(ctx, dir.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	folders, files, err := st.ListTrash(ctx)
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(folders) != 1 || folders[0].ID != dir.ID {
		t.Fatalf("papelera carpetas=%+v, quiero solo la carpeta", folders)
	}
	if len(files) != 0 {
		t.Fatalf("el fichero de dentro NO debe listarse suelto en la papelera, got %+v", files)
	}

	if _, err := st.RestoreFolder(ctx, dir.ID); err != nil {
		t.Fatalf("RestoreFolder: %v", err)
	}
	if _, err := st.GetFile(ctx, child.ID); err != nil {
		t.Fatalf("tras restaurar la carpeta, el fichero de dentro debe volver: %v", err)
	}
	if f, _, _ := st.ListTrash(ctx); len(f) != 0 {
		t.Fatalf("papelera tras restaurar debe estar vacía, got %+v", f)
	}

	if _, err := st.DeleteFolder(ctx, dir.ID); err != nil {
		t.Fatalf("DeleteFolder 2: %v", err)
	}
	purged, err := st.PurgeFolder(ctx, dir.ID)
	if err != nil {
		t.Fatalf("PurgeFolder: %v", err)
	}
	if len(purged) != 1 || purged[0].ID != child.ID {
		t.Fatalf("PurgeFolder debe devolver el fichero del subárbol, got %+v", purged)
	}
	if q, _ := st.GetQuota(ctx); q.UsedBytes != 0 {
		t.Fatalf("cuota tras purgar la carpeta=%d, quiero 0", q.UsedBytes)
	}
}

// TestDriveRecentAndFavorites comprueba el listado «Recientes» (updated_at DESC
// con rutas resueltas) y el ciclo de «Favoritos» (marcar fichero+carpeta, listar
// con carpetas primero, aislamiento por usuario, ocultar en papelera y revivir al
// restaurar, desmarcar idempotente y cascade al purgar).
func TestDriveRecentAndFavorites(t *testing.T) {
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
	docs, err := st.CreateFolder(ctx, root.ID, "Docs", "sub", false)
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	a := seedFile(t, ctx, st, root.ID, "a.txt", 1)
	b := seedFile(t, ctx, st, docs.ID, "b.txt", 2)
	// Renombrar a.txt bumpea su updated_at → debe encabezar «Recientes».
	if _, err := st.RenameFile(ctx, a.ID, "a2.txt"); err != nil {
		t.Fatalf("RenameFile: %v", err)
	}

	entries, next, err := st.ListRecent(ctx, store.RecentCursor{}, 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if next != nil {
		t.Fatal("con 2 ficheros no debe haber página siguiente")
	}
	if len(entries) != 2 {
		t.Fatalf("recientes=%d, quiero 2", len(entries))
	}
	if entries[0].ID != a.ID || entries[0].Path != "/a2.txt" {
		t.Fatalf("el más reciente debe ser a2.txt en raíz, got %+v", entries[0])
	}
	if entries[1].ID != b.ID || entries[1].Path != "/Docs/b.txt" {
		t.Fatalf("ruta anidada esperada /Docs/b.txt, got %+v", entries[1])
	}

	// Keyset: recorrido en páginas de 1 cubre ambos sin repetir ni saltar.
	var seen []uuid.UUID
	cur := store.RecentCursor{}
	for i := 0; i < 5; i++ {
		page, nx, err := st.ListRecent(ctx, cur, 1)
		if err != nil {
			t.Fatalf("ListRecent página: %v", err)
		}
		seen = append(seen, page[0].ID)
		if nx == nil {
			break
		}
		cur = *nx
	}
	if len(seen) != 2 || seen[0] != a.ID || seen[1] != b.ID {
		t.Fatalf("keyset recientes: %v", seen)
	}

	// Favoritos: marcar el fichero b y la carpeta Docs (idempotente).
	if err := st.SetFavorite(ctx, "user-1", &b.ID, nil, true); err != nil {
		t.Fatalf("SetFavorite fichero: %v", err)
	}
	if err := st.SetFavorite(ctx, "user-1", nil, &docs.ID, true); err != nil {
		t.Fatalf("SetFavorite carpeta: %v", err)
	}
	if err := st.SetFavorite(ctx, "user-1", &b.ID, nil, true); err != nil {
		t.Fatalf("SetFavorite idempotente: %v", err)
	}

	favs, err := st.ListFavorites(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListFavorites: %v", err)
	}
	if len(favs) != 2 {
		t.Fatalf("favoritos=%d, quiero 2", len(favs))
	}
	if !favs[0].IsDir || favs[0].ID != docs.ID || favs[0].Path != "/Docs" {
		t.Fatalf("carpetas primero: %+v", favs[0])
	}
	if favs[1].IsDir || favs[1].ID != b.ID || favs[1].Path != "/Docs/b.txt" {
		t.Fatalf("fichero favorito: %+v", favs[1])
	}

	// Aislamiento por usuario.
	if other, _ := st.ListFavorites(ctx, "user-2"); len(other) != 0 {
		t.Fatalf("otro usuario no debe ver favoritos ajenos: %+v", other)
	}

	// Papelera oculta el favorito; restaurar lo devuelve.
	if _, err := st.DeleteFile(ctx, b.ID); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if favs, _ := st.ListFavorites(ctx, "user-1"); len(favs) != 1 {
		t.Fatalf("un favorito en papelera no debe listarse: %+v", favs)
	}
	if _, err := st.RestoreFile(ctx, b.ID); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}
	if favs, _ := st.ListFavorites(ctx, "user-1"); len(favs) != 2 {
		t.Fatalf("tras restaurar debe reaparecer, got %d", len(favs))
	}

	// Desmarcar el fichero (idempotente).
	if err := st.SetFavorite(ctx, "user-1", &b.ID, nil, false); err != nil {
		t.Fatalf("desmarcar: %v", err)
	}
	if err := st.SetFavorite(ctx, "user-1", &b.ID, nil, false); err != nil {
		t.Fatalf("desmarcar idempotente: %v", err)
	}
	if favs, _ := st.ListFavorites(ctx, "user-1"); len(favs) != 1 {
		t.Fatalf("tras desmarcar el fichero queda %d, quiero 1 (la carpeta)", len(favs))
	}

	// Purga en duro de la carpeta favorita → ON DELETE CASCADE borra la marca.
	if _, err := st.DeleteFolder(ctx, docs.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	if _, err := st.PurgeFolder(ctx, docs.ID); err != nil {
		t.Fatalf("PurgeFolder: %v", err)
	}
	if favs, _ := st.ListFavorites(ctx, "user-1"); len(favs) != 0 {
		t.Fatalf("tras purgar, cascade debe vaciar favoritos, got %d", len(favs))
	}

	// Marcar un fichero inexistente falla (no crea marcas huérfanas).
	if err := st.SetFavorite(ctx, "user-1", &[]uuid.UUID{uuid.New()}[0], nil, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("marcar id inexistente => %v, quiero ErrNotFound", err)
	}
}

// TestDriveFileVersions comprueba el historial de versiones: reemplazar el
// contenido ARCHIVA la versión anterior REFERENCIANDO su blob (sin duplicar), el
// listado keyset va más-nueva-primero, restaurar es NO DESTRUCTIVO (la actual se
// archiva y la objetivo revive como versión nueva) y ajusta la cuota, descargar
// una versión devuelve su clave de blob, y el aislamiento por tenant (RLS) niega
// versiones de ficheros ajenos.
func TestDriveFileVersions(t *testing.T) {
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

	put := func(key, author string, size int64) domain.File {
		t.Helper()
		f, err := st.CreateFile(ctx, store.NewFileRecord{
			FolderID: root.ID, Name: "doc.txt", StorageKey: key,
			SizeBytes: size, ContentType: "text/plain", OwnerSub: author,
		})
		if err != nil {
			t.Fatalf("CreateFile(%s): %v", key, err)
		}
		return f
	}

	// v1 (k1, 10B) → reemplazo v2 (k2, 20B) → reemplazo v3 (k3, 30B).
	file := put("k1", "alice", 10)
	put("k2", "bob", 20)
	cur := put("k3", "carol", 30)
	if cur.ID != file.ID || cur.Version != 3 {
		t.Fatalf("tras 2 reemplazos: id=%v version=%d, quiero mismo id y version 3", cur.ID, cur.Version)
	}
	// La cuota solo cuenta el tamaño ACTUAL (las versiones no suman): 30B.
	if q, _ := st.GetQuota(ctx); q.UsedBytes != 30 {
		t.Fatalf("cuota=%d, quiero 30 (solo el tamaño actual)", q.UsedBytes)
	}

	// Listado: dos versiones ARCHIVADAS, más nueva primero (v2, v1); cada una
	// REFERENCIA el blob de su subida (k2, k1) — no un duplicado.
	vers, next, err := st.ListVersions(ctx, file.ID, 0, 10)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if next != nil {
		t.Fatal("con 2 versiones no debe haber página siguiente")
	}
	if len(vers) != 2 || vers[0].Version != 2 || vers[1].Version != 1 {
		t.Fatalf("orden versiones: %+v", vers)
	}
	if vers[0].StorageKey != "k2" || vers[0].SizeBytes != 20 || vers[0].AuthorSub != "bob" {
		t.Fatalf("v2 archivada: %+v", vers[0])
	}
	if vers[1].StorageKey != "k1" || vers[1].AuthorSub != "alice" {
		t.Fatalf("v1 archivada: %+v", vers[1])
	}

	// Keyset en páginas de 1: cubre v2 y v1 sin repetir ni saltar.
	page1, nx1, err := st.ListVersions(ctx, file.ID, 0, 1)
	if err != nil || len(page1) != 1 || page1[0].Version != 2 || nx1 == nil || *nx1 != 2 {
		t.Fatalf("página 1: v=%+v next=%v err=%v", page1, nx1, err)
	}
	page2, nx2, err := st.ListVersions(ctx, file.ID, *nx1, 1)
	if err != nil || len(page2) != 1 || page2[0].Version != 1 || nx2 != nil {
		t.Fatalf("página 2: v=%+v next=%v err=%v", page2, nx2, err)
	}

	// Descargar una versión: devuelve la clave del blob de ESA versión.
	gv, err := st.GetVersion(ctx, file.ID, 1)
	if err != nil || gv.StorageKey != "k1" || gv.SizeBytes != 10 {
		t.Fatalf("GetVersion(1): %+v err=%v", gv, err)
	}

	// Restaurar v1 (NO destructivo): la actual (v3, k3) se archiva y v1 revive como
	// versión NUEVA (v4) referenciando k1. La cuota baja a 10B (30→10).
	restored, err := st.RestoreVersion(ctx, file.ID, 1, "dave")
	if err != nil {
		t.Fatalf("RestoreVersion: %v", err)
	}
	if restored.Version != 4 || restored.StorageKey != "k1" || restored.SizeBytes != 10 || restored.ModifiedBy != "dave" {
		t.Fatalf("restaurado: %+v", restored)
	}
	if got, _ := st.GetFile(ctx, file.ID); got.Version != 4 || got.StorageKey != "k1" {
		t.Fatalf("tras restaurar, el fichero actual: %+v", got)
	}
	if q, _ := st.GetQuota(ctx); q.UsedBytes != 10 {
		t.Fatalf("cuota tras restaurar=%d, quiero 10", q.UsedBytes)
	}
	// El historial conserva las 3 versiones anteriores (v3 recién archivada).
	vers, _, err = st.ListVersions(ctx, file.ID, 0, 10)
	if err != nil {
		t.Fatalf("ListVersions tras restaurar: %v", err)
	}
	if len(vers) != 3 || vers[0].Version != 3 || vers[0].StorageKey != "k3" {
		t.Fatalf("historial tras restaurar: %+v", vers)
	}

	// Versión inexistente / aislamiento por tenant: ErrNotFound (RLS, sin fuga).
	if _, err := st.GetVersion(ctx, file.ID, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("versión inexistente => %v, quiero ErrNotFound", err)
	}
	other := tenantCtx("otro")
	if _, err := st.GetVersion(other, file.ID, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento GetVersion roto: %v", err)
	}
	if _, err := st.RestoreVersion(other, file.ID, 1, "mallory"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento RestoreVersion roto: %v", err)
	}
	if v, _, _ := st.ListVersions(other, file.ID, 0, 10); len(v) != 0 {
		t.Fatalf("otro tenant no debe ver versiones ajenas: %+v", v)
	}
}

// TestDriveBatchMove comprueba el movimiento en lote (WHERE id = ANY): mueve N
// ficheros y una carpeta a un destino en UNA operación; el aislamiento por tenant
// (RLS) omite ids ajenos (cuenta 0, sin mover); un destino descendiente de una
// carpeta movida da ErrCycle; un choque de nombre en el destino aborta con
// ErrConflict (atómico).
func TestDriveBatchMove(t *testing.T) {
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
	dst, err := st.CreateFolder(ctx, root.ID, "Destino", "sub", false)
	if err != nil {
		t.Fatalf("CreateFolder destino: %v", err)
	}
	moved, err := st.CreateFolder(ctx, root.ID, "Movida", "sub", false)
	if err != nil {
		t.Fatalf("CreateFolder movida: %v", err)
	}
	a := seedFile(t, ctx, st, root.ID, "a.txt", 1)
	b := seedFile(t, ctx, st, root.ID, "b.txt", 1)

	// Mover a.txt + b.txt + la carpeta «Movida» a «Destino» en una operación.
	res, err := st.BatchMove(ctx, []uuid.UUID{a.ID, b.ID}, []uuid.UUID{moved.ID}, dst.ID)
	if err != nil {
		t.Fatalf("BatchMove: %v", err)
	}
	if res.Files != 2 || res.Folders != 1 {
		t.Fatalf("BatchMove result = %+v, quiero 2 ficheros / 1 carpeta", res)
	}
	if got, _ := st.GetFile(ctx, a.ID); got.FolderID != dst.ID {
		t.Fatalf("a.txt no se movió a Destino, folder=%v", got.FolderID)
	}
	if got, _ := st.GetFolder(ctx, moved.ID); got.ParentID == nil || *got.ParentID != dst.ID {
		t.Fatalf("carpeta no reubicada bajo Destino: %+v", got.ParentID)
	}

	// Aislamiento: otro tenant no mueve nada del tenant acme (ANY no casa por RLS).
	res2, err := st.BatchMove(tenantCtx("otro"), []uuid.UUID{a.ID, b.ID}, nil, dst.ID)
	if !errors.Is(err, domain.ErrInvalidParent) {
		// El destino tampoco existe en «otro»: destino inválido, y desde luego 0 movidos.
		t.Fatalf("otro tenant => %v, quiero ErrInvalidParent (destino ajeno)", err)
	}
	_ = res2

	// Ciclo: mover «Destino» dentro de su descendiente «Movida» (ahora bajo Destino).
	if _, err := st.BatchMove(ctx, nil, []uuid.UUID{dst.ID}, moved.ID); !errors.Is(err, domain.ErrCycle) {
		t.Fatalf("mover carpeta dentro de su descendiente => %v, quiero ErrCycle", err)
	}

	// Conflicto: ya hay un a.txt en Destino; mover otro a.txt (desde una subcarpeta)
	// choca con el índice único y aborta el lote (ErrConflict).
	other, err := st.CreateFolder(ctx, root.ID, "Otra", "sub", false)
	if err != nil {
		t.Fatalf("CreateFolder otra: %v", err)
	}
	dupe := seedFile(t, ctx, st, other.ID, "a.txt", 1)
	if _, err := st.BatchMove(ctx, []uuid.UUID{dupe.ID}, nil, dst.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("choque de nombre en el destino => %v, quiero ErrConflict", err)
	}
}

// TestDriveBatchTrashAndZip comprueba la papelera en lote (soft delete de N ficheros
// + subárboles de N carpetas en una operación, conservando blobs/cuota) y la
// recolección de un ZIP (rutas relativas: subárbol prefijado por el nombre de la
// carpeta + ficheros sueltos en la raíz; solo lo accesible y vivo; aislamiento por
// tenant).
func TestDriveBatchTrashAndZip(t *testing.T) {
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
	docs, err := st.CreateFolder(ctx, root.ID, "Docs", "sub", false)
	if err != nil {
		t.Fatalf("CreateFolder Docs: %v", err)
	}
	sub, err := st.CreateFolder(ctx, docs.ID, "sub", "sub", false)
	if err != nil {
		t.Fatalf("CreateFolder sub: %v", err)
	}
	inner := seedFile(t, ctx, st, docs.ID, "a.txt", 10)    // Docs/a.txt
	deep := seedFile(t, ctx, st, sub.ID, "b.txt", 20)      // Docs/sub/b.txt
	loose := seedFile(t, ctx, st, root.ID, "loose.txt", 5) // /loose.txt

	// --- ZIP: carpeta Docs (subárbol) + fichero suelto loose.txt ---
	entries, err := st.CollectZip(ctx, []uuid.UUID{loose.ID}, []uuid.UUID{docs.ID}, 100)
	if err != nil {
		t.Fatalf("CollectZip: %v", err)
	}
	paths := map[string]int64{}
	for _, e := range entries {
		if e.StorageKey == "" {
			t.Fatalf("entrada sin storage_key: %+v", e)
		}
		paths[e.Path] = e.Size
	}
	if len(paths) != 3 {
		t.Fatalf("entradas del zip = %v, quiero 3", paths)
	}
	if paths["Docs/a.txt"] != 10 || paths["Docs/sub/b.txt"] != 20 || paths["loose.txt"] != 5 {
		t.Fatalf("rutas/tamaños del zip inesperados: %v", paths)
	}
	// Un fichero borrado del subárbol NO se incluye (deny-by-default: vivo).
	if _, err := st.DeleteFile(ctx, deep.ID); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	entries, _ = st.CollectZip(ctx, nil, []uuid.UUID{docs.ID}, 100)
	for _, e := range entries {
		if e.Path == "Docs/sub/b.txt" {
			t.Fatalf("un fichero en papelera no debe entrar en el zip: %+v", e)
		}
	}
	// Aislamiento: otro tenant no ve ningún fichero por esos ids.
	if e, _ := st.CollectZip(tenantCtx("otro"), []uuid.UUID{loose.ID}, []uuid.UUID{docs.ID}, 100); len(e) != 0 {
		t.Fatalf("otro tenant no debe recolectar nada: %+v", e)
	}
	// Restaurar deep para la fase de papelera en lote.
	if _, err := st.RestoreFile(ctx, deep.ID); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}

	// --- Papelera en lote: fichero suelto + subárbol de Docs, en una operación ---
	deleted, folderCount, err := st.BatchTrash(ctx, []uuid.UUID{loose.ID}, []uuid.UUID{docs.ID})
	if err != nil {
		t.Fatalf("BatchTrash: %v", err)
	}
	// 3 ficheros borrados (loose + Docs/a.txt + Docs/sub/b.txt) y 2 carpetas (Docs, sub).
	if len(deleted) != 3 || folderCount != 2 {
		t.Fatalf("BatchTrash = %d ficheros / %d carpetas, quiero 3/2", len(deleted), folderCount)
	}
	// Soft delete: la cuota se CONSERVA (blobs vivos).
	if q, _ := st.GetQuota(ctx); q.UsedBytes != 35 {
		t.Fatalf("cuota tras papelera en lote=%d, quiero 35 (blobs conservados)", q.UsedBytes)
	}
	// Los ficheros del subárbol dejan el listado activo.
	if _, err := st.GetFile(ctx, inner.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Docs/a.txt debería estar en papelera: %v", err)
	}
	// La papelera muestra Docs (nivel superior) y loose.txt, no el contenido suelto.
	folders, files, err := st.ListTrash(ctx)
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(folders) != 1 || folders[0].ID != docs.ID {
		t.Fatalf("papelera carpetas=%+v, quiero solo Docs", folders)
	}
	if len(files) != 1 || files[0].ID != loose.ID {
		t.Fatalf("papelera ficheros=%+v, quiero solo loose.txt", files)
	}
}

func TestDriveTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctxA := tenantCtx("tenant-a")
	ctxB := tenantCtx("tenant-b")

	if _, err := st.EnsureQuota(ctxA, 1<<30); err != nil {
		t.Fatalf("A EnsureQuota: %v", err)
	}
	rootA, err := st.EnsureRoot(ctxA)
	if err != nil {
		t.Fatalf("A EnsureRoot: %v", err)
	}
	fileA := seedFile(t, ctxA, st, rootA.ID, "confidencial-a.txt", 10)

	// Tenant B no ve el fichero de A ni por su ID (RLS).
	if _, err := st.GetFile(ctxB, fileA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B ve el fichero de A (err=%v)", err)
	}
	// Tenant B no ve la carpeta raiz de A por su ID.
	if _, err := st.GetFolder(ctxB, rootA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B ve la carpeta de A (err=%v)", err)
	}
	// A conserva lo suyo.
	if _, err := st.GetFile(ctxA, fileA.ID); err != nil {
		t.Fatalf("A perdio su fichero: %v", err)
	}
}

// TestDriveSharePasswordAndResolve comprueba el ciclo de un enlace público con y
// sin contraseña: CreateShare persiste el hash (HasPassword=true), ResolveLink
// (anónimo, sin tenant en ctx) devuelve el share + tenant + hash para verificar,
// y un enlace sin contraseña resuelve con hash vacío.
func TestDriveSharePasswordAndResolve(t *testing.T) {
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
	file := seedFile(t, ctx, st, root.ID, "secreto.txt", 10)

	hash, err := domain.HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Enlace a fichero protegido por contraseña.
	sh, err := st.CreateShare(ctx, store.NewShareRecord{
		FileID: &file.ID, Kind: domain.ShareLink, Permission: domain.PermRead,
		Token: "tok-secreto-123", PasswordHash: hash,
	})
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	if !sh.HasPassword {
		t.Fatal("el share creado con contraseña debe reportar HasPassword=true")
	}

	got, tn, gotHash, err := st.ResolveLink(context.Background(), "tok-secreto-123")
	if err != nil {
		t.Fatalf("ResolveLink: %v", err)
	}
	if tn != "acme" || !got.HasPassword {
		t.Fatalf("ResolveLink protegido: tenant=%q hasPassword=%v", tn, got.HasPassword)
	}
	if gotHash != hash {
		t.Fatal("ResolveLink debe devolver el hash en claro para verificar el prompt")
	}
	if ok, _ := domain.VerifyPassword("hunter2", gotHash); !ok {
		t.Fatal("el hash devuelto no verifica la contraseña original")
	}

	// Enlace a carpeta SIN contraseña: HasPassword=false, hash vacío.
	shf, err := st.CreateShare(ctx, store.NewShareRecord{
		FolderID: &root.ID, Kind: domain.ShareLink, Permission: domain.PermRead, Token: "tok-carpeta-456",
	})
	if err != nil {
		t.Fatalf("CreateShare carpeta: %v", err)
	}
	if shf.HasPassword {
		t.Fatal("carpeta sin contraseña no debe reportar HasPassword")
	}
	if _, _, h2, err := st.ResolveLink(context.Background(), "tok-carpeta-456"); err != nil || h2 != "" {
		t.Fatalf("ResolveLink sin contraseña: hash=%q err=%v", h2, err)
	}

	// Token inexistente => ErrShareExpired (no 500).
	if _, _, _, err := st.ResolveLink(context.Background(), "no-existe"); !errors.Is(err, domain.ErrShareExpired) {
		t.Fatalf("token inexistente => %v, quiero ErrShareExpired", err)
	}
}

// TestDriveSharedWithMe comprueba «Compartido conmigo» + la verja de acceso por
// share kind=user|group: un share dirigido al sub o a un grupo concede acceso con
// su permiso; sin share vigente o con share CADUCADO se deniega (ErrNotFound,
// deny-by-default); el listado resuelve rutas y omite destinos en la papelera.
func TestDriveSharedWithMe(t *testing.T) {
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
	dir, err := st.CreateFolder(ctx, root.ID, "Equipo", "alice", false)
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	file := seedFile(t, ctx, st, dir.ID, "plan.pdf", 100)

	mkShare := func(rec store.NewShareRecord) {
		t.Helper()
		if _, err := st.CreateShare(ctx, rec); err != nil {
			t.Fatalf("CreateShare(%s): %v", rec.Subject, err)
		}
	}
	// alice comparte la CARPETA con bob (user, editor) y el FICHERO con el grupo eng
	// (viewer); a carol le comparte la carpeta pero CADUCADO.
	mkShare(store.NewShareRecord{FolderID: &dir.ID, Kind: domain.ShareUser, Subject: "bob", Permission: domain.PermWrite, CreatedBy: "alice"})
	mkShare(store.NewShareRecord{FileID: &file.ID, Kind: domain.ShareGroup, Subject: "eng", Permission: domain.PermRead, CreatedBy: "alice"})
	past := time.Now().Add(-time.Hour)
	mkShare(store.NewShareRecord{FolderID: &dir.ID, Kind: domain.ShareUser, Subject: "carol", Permission: domain.PermRead, CreatedBy: "alice", ExpiresAt: &past})

	// bob: ve la carpeta compartida (editor, con ruta y dueño).
	entries, err := st.ListSharedWithMe(ctx, "bob", nil)
	if err != nil {
		t.Fatalf("ListSharedWithMe(bob): %v", err)
	}
	if len(entries) != 1 || !entries[0].IsDir || entries[0].ID != dir.ID ||
		entries[0].Permission != domain.PermWrite || entries[0].SharedBy != "alice" || entries[0].Path != "/Equipo" {
		t.Fatalf("bob shared-with-me: %+v", entries)
	}
	// Verja: bob accede a la carpeta como editor; NO al fichero (solo por grupo eng,
	// y bob no está en eng).
	if sh, err := st.ShareForResource(ctx, "bob", nil, dir.ID); err != nil || sh.Permission != domain.PermWrite {
		t.Fatalf("verja bob carpeta: sh=%+v err=%v", sh, err)
	}
	if _, err := st.ShareForResource(ctx, "bob", nil, file.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob no debe acceder al fichero por su sub: %v", err)
	}

	// dave ∈ eng: ve el FICHERO compartido (viewer) por grupo.
	engEntries, err := st.ListSharedWithMe(ctx, "dave", []string{"eng"})
	if err != nil {
		t.Fatalf("ListSharedWithMe(dave): %v", err)
	}
	if len(engEntries) != 1 || engEntries[0].IsDir || engEntries[0].ID != file.ID ||
		engEntries[0].Permission != domain.PermRead || engEntries[0].Path != "/Equipo/plan.pdf" {
		t.Fatalf("dave (eng) shared-with-me: %+v", engEntries)
	}
	if sh, err := st.ShareForResource(ctx, "dave", []string{"eng"}, file.ID); err != nil || sh.Permission != domain.PermRead {
		t.Fatalf("verja dave fichero por grupo: sh=%+v err=%v", sh, err)
	}

	// carol (share CADUCADO): sin acceso.
	if e, _ := st.ListSharedWithMe(ctx, "carol", nil); len(e) != 0 {
		t.Fatalf("carol (caducado) no debe ver nada: %+v", e)
	}
	if _, err := st.ShareForResource(ctx, "carol", nil, dir.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("verja carol caducado => %v, quiero ErrNotFound", err)
	}

	// erin sin shares: deny-by-default.
	if e, _ := st.ListSharedWithMe(ctx, "erin", []string{"otros"}); len(e) != 0 {
		t.Fatalf("erin sin shares: %+v", e)
	}
	if _, err := st.ShareForResource(ctx, "erin", nil, dir.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("verja erin => %v, quiero ErrNotFound", err)
	}

	// ListSharesForSubject: bob tiene 1 share vigente (la carpeta).
	if shares, _ := st.ListSharesForSubject(ctx, "bob", nil); len(shares) != 1 || shares[0].Subject != "bob" {
		t.Fatalf("ListSharesForSubject bob: %+v", shares)
	}

	// Destino en la papelera: se omite del listado (revive al restaurar).
	if _, err := st.DeleteFolder(ctx, dir.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	if e, _ := st.ListSharedWithMe(ctx, "bob", nil); len(e) != 0 {
		t.Fatalf("carpeta en papelera no debe listarse: %+v", e)
	}
	if e, _ := st.ListSharedWithMe(ctx, "dave", []string{"eng"}); len(e) != 0 {
		t.Fatalf("fichero (subárbol borrado) no debe listarse: %+v", e)
	}
}

// TestDriveGetFileByNameAndSearch comprueba el lookup indexado por (carpeta,
// nombre) usado por la navegación pública y la búsqueda por subcadena (índice GIN
// pg_trgm de la migración 0005), incluido el aislamiento por tenant.
func TestDriveGetFileByNameAndSearch(t *testing.T) {
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
	seedFile(t, ctx, st, root.ID, "Factura-2026.pdf", 10)
	seedFile(t, ctx, st, root.ID, "notas.txt", 5)

	// GetFileByName: case-insensitive, por (carpeta, nombre).
	got, err := st.GetFileByName(ctx, root.ID, "factura-2026.pdf")
	if err != nil || got.Name != "Factura-2026.pdf" {
		t.Fatalf("GetFileByName: got=%q err=%v", got.Name, err)
	}
	if _, err := st.GetFileByName(ctx, root.ID, "no-existe.doc"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetFileByName ausente => %v", err)
	}

	// SearchFiles: subcadena case-insensitive (la sirve el GIN pg_trgm).
	res, err := st.SearchFiles(ctx, "factura", 10)
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if len(res) != 1 || res[0].Name != "Factura-2026.pdf" {
		t.Fatalf("búsqueda 'factura' => %+v", res)
	}
	if res[0].Path != "/Factura-2026.pdf" {
		t.Fatalf("ruta en la raíz = %q, quiero /Factura-2026.pdf", res[0].Path)
	}

	// La RUTA es lo que hace útil un resultado: una búsqueda cruza todo el árbol, así
	// que sin ella no se puede abrir ni situar el fichero. Se comprueba anidada, que es
	// donde el resolvedor recursivo puede fallar (en la raíz acierta por casualidad).
	sub, err := st.CreateFolder(ctx, root.ID, "Contabilidad", "u1", false)
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	seedFile(t, ctx, st, sub.ID, "Factura-2027.pdf", 7)
	res, err = st.SearchFiles(ctx, "factura-2027", 10)
	if err != nil {
		t.Fatalf("SearchFiles anidada: %v", err)
	}
	if len(res) != 1 || res[0].Path != "/Contabilidad/Factura-2027.pdf" {
		t.Fatalf("ruta anidada => %+v", res)
	}

	// Aislamiento por tenant: otro tenant no encuentra el fichero por nombre.
	if _, err := st.GetFileByName(tenantCtx("otro"), root.ID, "Factura-2026.pdf"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento GetFileByName roto: %v", err)
	}
}

// TestDriveCopyFileReferencesBlob comprueba la copia server-side de un fichero: la
// nueva fila REFERENCIA EL MISMO blob (misma storage_key/size/checksum, sin
// duplicar bytes), el nombre es «Copia de X» único (segunda copia → «(2)»), y la
// cuota crece por bytes LÓGICOS (política de CreateFile). Aislamiento por tenant.
func TestDriveCopyFileReferencesBlob(t *testing.T) {
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
	src, err := st.CreateFile(ctx, store.NewFileRecord{
		FolderID: root.ID, Name: "informe.pdf", StorageKey: "files/blob-xyz",
		SizeBytes: 100, ContentType: "application/pdf", Checksum: "sha-abc", OwnerSub: "alice",
	})
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}

	cp, err := st.CopyFile(ctx, src.ID, root.ID, "bob")
	if err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	if cp.ID == src.ID {
		t.Fatal("la copia debe ser una fila NUEVA (id distinto)")
	}
	if cp.StorageKey != src.StorageKey {
		t.Fatalf("la copia debe REFERENCIAR el mismo blob: copia=%q origen=%q", cp.StorageKey, src.StorageKey)
	}
	if cp.SizeBytes != 100 || cp.Checksum != "sha-abc" || cp.ContentType != "application/pdf" {
		t.Fatalf("la copia debe heredar tamaño/checksum/tipo: %+v", cp)
	}
	if cp.Name != "Copia de informe.pdf" {
		t.Fatalf("nombre de copia = %q, quiero «Copia de informe.pdf»", cp.Name)
	}
	// Cuota por bytes lógicos: 100 (origen) + 100 (copia) = 200, aunque el blob no
	// se duplique en S3.
	if q, _ := st.GetQuota(ctx); q.UsedBytes != 200 {
		t.Fatalf("cuota tras copiar = %d, quiero 200 (bytes lógicos)", q.UsedBytes)
	}

	// Segunda copia en la misma carpeta: nombre único «Copia de informe.pdf (2)».
	cp2, err := st.CopyFile(ctx, src.ID, root.ID, "bob")
	if err != nil {
		t.Fatalf("segunda CopyFile: %v", err)
	}
	if cp2.Name != "Copia de informe.pdf (2)" {
		t.Fatalf("segunda copia = %q, quiero «Copia de informe.pdf (2)»", cp2.Name)
	}

	// Purga del ORIGEN: su blob lo comparten 2 copias por referencia ⇒ NO es huérfano
	// (no debe borrarse de S3; el servicio conserva los bytes de las copias).
	if _, err := st.DeleteFile(ctx, src.ID); err != nil {
		t.Fatalf("DeleteFile origen: %v", err)
	}
	df, err := st.PurgeFile(ctx, src.ID)
	if err != nil {
		t.Fatalf("PurgeFile origen: %v", err)
	}
	if df.BlobOrphaned {
		t.Fatal("el blob compartido por las copias NO debe marcarse huérfano")
	}

	// Aislamiento: otro tenant no puede copiar el fichero de acme (RLS → NotFound).
	other := tenantCtx("otro")
	if _, err := st.EnsureQuota(other, 1<<30); err != nil {
		t.Fatalf("otro EnsureQuota: %v", err)
	}
	rootOther, _ := st.EnsureRoot(other)
	if _, err := st.CopyFile(other, cp.ID, rootOther.ID, "mallory"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento CopyFile roto: %v (quiero NotFound)", err)
	}
}

// TestDriveCopyFolderRecursive comprueba la copia recursiva de una carpeta: replica
// la estructura (subcarpetas) y cada fichero es una fila NUEVA que referencia su
// MISMO blob; la raíz es «Copia de X» hermana del origen; la cuota crece por los
// bytes lógicos totales del subárbol.
func TestDriveCopyFolderRecursive(t *testing.T) {
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
	// Estructura: /Proyecto (a.txt) / Sub (b.txt)
	proj, err := st.CreateFolder(ctx, root.ID, "Proyecto", "alice", false)
	if err != nil {
		t.Fatalf("CreateFolder Proyecto: %v", err)
	}
	sub, err := st.CreateFolder(ctx, proj.ID, "Sub", "alice", false)
	if err != nil {
		t.Fatalf("CreateFolder Sub: %v", err)
	}
	a := seedFile(t, ctx, st, proj.ID, "a.txt", 10)
	b := seedFile(t, ctx, st, sub.ID, "b.txt", 20)

	usedBefore, _ := st.GetQuota(ctx)

	// Copia hermana (destParent nil = padre del origen = root).
	cp, err := st.CopyFolder(ctx, proj.ID, nil, "bob")
	if err != nil {
		t.Fatalf("CopyFolder: %v", err)
	}
	if cp.Name != "Copia de Proyecto" || cp.ParentID == nil || *cp.ParentID != root.ID {
		t.Fatalf("raíz de la copia: name=%q parent=%v", cp.Name, cp.ParentID)
	}

	// La copia contiene a.txt (referenciando el blob de a) y la subcarpeta Sub/b.txt.
	subs, files, err := st.ListChildren(ctx, cp.ID)
	if err != nil {
		t.Fatalf("ListChildren copia: %v", err)
	}
	if len(files) != 1 || files[0].Name != "a.txt" || files[0].StorageKey != a.StorageKey {
		t.Fatalf("copia raíz: files=%+v (quiero a.txt referenciando %q)", files, a.StorageKey)
	}
	if len(subs) != 1 || subs[0].Name != "Sub" {
		t.Fatalf("copia raíz: subcarpetas=%+v (quiero Sub)", subs)
	}
	_, subFiles, err := st.ListChildren(ctx, subs[0].ID)
	if err != nil {
		t.Fatalf("ListChildren Sub copiada: %v", err)
	}
	if len(subFiles) != 1 || subFiles[0].Name != "b.txt" || subFiles[0].StorageKey != b.StorageKey {
		t.Fatalf("Sub copiada: files=%+v (quiero b.txt referenciando %q)", subFiles, b.StorageKey)
	}

	// Cuota: +30 (10+20) por bytes lógicos, sin duplicar blobs.
	usedAfter, _ := st.GetQuota(ctx)
	if usedAfter.UsedBytes-usedBefore.UsedBytes != 30 {
		t.Fatalf("cuota copia carpeta: delta=%d, quiero 30", usedAfter.UsedBytes-usedBefore.UsedBytes)
	}

	// No se puede copiar una carpeta DENTRO de sí misma (ciclo).
	if _, err := st.CopyFolder(ctx, proj.ID, &proj.ID, "bob"); !errors.Is(err, domain.ErrCycle) {
		t.Fatalf("copiar dentro de sí misma => %v, quiero ErrCycle", err)
	}
}

// TestGetFileByStorageKey ejercita la búsqueda por storage_key que sostiene la
// idempotencia de la finalización de subidas directas: encuentra el fichero vivo
// que referencia la clave, devuelve ErrNotFound si ninguna la referencia y aísla
// por tenant (RLS: la clave de un tenant no es visible desde otro).
func TestGetFileByStorageKey(t *testing.T) {
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
	f := seedFile(t, ctx, st, root.ID, "big.bin", 500) // StorageKey = files/big.bin

	got, err := st.GetFileByStorageKey(ctx, f.StorageKey)
	if err != nil {
		t.Fatalf("GetFileByStorageKey: %v", err)
	}
	if got.ID != f.ID {
		t.Fatalf("devolvió %v, esperaba %v", got.ID, f.ID)
	}

	// Clave inexistente → ErrNotFound (deja que finalize proceda a crear).
	if _, err := st.GetFileByStorageKey(ctx, "files/no-existe"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("clave inexistente => %v, quiero ErrNotFound", err)
	}

	// Aislamiento por tenant: otro tenant NO ve la clave (RLS).
	if _, err := st.GetFileByStorageKey(tenantCtx("other"), f.StorageKey); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant => %v, quiero ErrNotFound (RLS)", err)
	}
}
