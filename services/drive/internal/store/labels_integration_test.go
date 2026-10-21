// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Integración del SNAPSHOT de etiqueta de sensibilidad (mig 0013) contra PostgreSQL
// real (pgtest): persistencia y borrado del snapshot, propagación a las copias
// server-side (copiar no puede ser la forma fácil de quitar una etiqueta), filtro por
// etiqueta en eDiscovery y aislamiento por tenant.
package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

var etiquetaConfidencial = domain.Label{
	ID: "22222222-2222-2222-2222-222222222222", Name: "Confidencial",
	Rank: 30, BlockExternalShare: true,
}

func TestFileLabelSnapshot(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.EnsureQuota(ctx, 1<<20); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	f := seedFile(t, ctx, st, root.ID, "nomina.pdf", 10)
	if f.Label != nil {
		t.Fatalf("un fichero nace SIN etiqueta: %+v", f.Label)
	}

	// Etiquetar guarda el snapshot completo.
	got, err := st.SetFileLabel(ctx, f.ID, etiquetaConfidencial)
	if err != nil {
		t.Fatalf("SetFileLabel: %v", err)
	}
	if got.Label == nil || got.Label.Name != "Confidencial" || got.Label.Rank != 30 || !got.Label.BlockExternalShare {
		t.Fatalf("snapshot mal guardado: %+v", got.Label)
	}
	// Y se relee igual por cualquier camino de lectura de ficheros.
	reread, err := st.GetFile(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if reread.Label == nil || *reread.Label != etiquetaConfidencial {
		t.Fatalf("el snapshot no se relee igual: %+v", reread.Label)
	}

	// Renombrar/mover NO tocan la etiqueta (viaja con el fichero).
	ren, err := st.RenameFile(ctx, f.ID, "nomina-2026.pdf")
	if err != nil {
		t.Fatalf("RenameFile: %v", err)
	}
	if ren.Label == nil || !ren.Label.BlockExternalShare {
		t.Fatalf("renombrar no debe perder la etiqueta: %+v", ren.Label)
	}

	// COPIAR PROPAGA la etiqueta: duplicar no puede ser la forma de quitarla.
	cp, err := st.CopyFile(ctx, f.ID, root.ID, "user-1")
	if err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	if cp.Label == nil || *cp.Label != etiquetaConfidencial {
		t.Fatalf("la copia debe heredar la etiqueta: %+v", cp.Label)
	}

	// Copiar una CARPETA también propaga (mismo motivo, camino en bloque).
	sub, err := st.CreateFolder(ctx, root.ID, "Con etiquetados", "user-1", false)
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	inner := seedFile(t, ctx, st, sub.ID, "secreto.pdf", 10)
	if _, err := st.SetFileLabel(ctx, inner.ID, etiquetaConfidencial); err != nil {
		t.Fatalf("SetFileLabel(inner): %v", err)
	}
	cpFolder, err := st.CopyFolder(ctx, sub.ID, &root.ID, "user-1")
	if err != nil {
		t.Fatalf("CopyFolder: %v", err)
	}
	_, copiedFiles, err := st.ListChildren(ctx, cpFolder.ID)
	if err != nil {
		t.Fatalf("ListChildren(copia): %v", err)
	}
	if len(copiedFiles) != 1 || copiedFiles[0].Label == nil || !copiedFiles[0].Label.BlockExternalShare {
		t.Fatalf("la copia de carpeta debe heredar las etiquetas: %+v", copiedFiles)
	}

	// Desetiquetar (snapshot vacío) limpia las cuatro columnas.
	cleared, err := st.SetFileLabel(ctx, f.ID, domain.Label{})
	if err != nil {
		t.Fatalf("SetFileLabel(vacío): %v", err)
	}
	if cleared.Label != nil {
		t.Fatalf("desetiquetar debe dejarlo sin etiqueta: %+v", cleared.Label)
	}

	// Aislamiento por tenant: un fichero de otra org no existe (no se etiqueta).
	if _, err := st.SetFileLabel(tenantCtx("otra"), f.ID, etiquetaConfidencial); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant: se esperaba ErrNotFound, got %v", err)
	}
	if _, err := st.SetFileLabel(ctx, uuid.New(), etiquetaConfidencial); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("fichero inexistente: se esperaba ErrNotFound, got %v", err)
	}
}

// TestLabelTravelsInFlatListings fija la incoherencia que arregla esta ola: las
// proyecciones planas de «Recientes», «Favoritos» y «Compartido conmigo» traen el
// snapshot de etiqueta EN LA MISMA consulta (sin N+1), como el listado principal,
// para que el cliente pueda avisar del bloqueo de compartición antes de intentarlo.
// Un fichero sin etiquetar (y cualquier carpeta) sigue llegando sin etiqueta.
func TestLabelTravelsInFlatListings(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.EnsureQuota(ctx, 1<<20); err != nil {
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
	etiquetado := seedFile(t, ctx, st, dir.ID, "nomina.pdf", 10)
	suelto := seedFile(t, ctx, st, dir.ID, "notas.txt", 10)
	if _, err := st.SetFileLabel(ctx, etiquetado.ID, etiquetaConfidencial); err != nil {
		t.Fatalf("SetFileLabel: %v", err)
	}

	// Cada vista se comprueba por ID (el orden de cada listado es cosa suya).
	check := func(vista string, entries []store.Entry) {
		t.Helper()
		byID := make(map[uuid.UUID]store.Entry, len(entries))
		for _, e := range entries {
			byID[e.ID] = e
		}
		got, ok := byID[etiquetado.ID]
		if !ok {
			t.Fatalf("%s: falta el fichero etiquetado (%d entradas)", vista, len(entries))
		}
		if got.Label == nil || *got.Label != etiquetaConfidencial {
			t.Fatalf("%s: la etiqueta no viaja en la proyección: %+v", vista, got.Label)
		}
		if sin, ok := byID[suelto.ID]; ok && sin.Label != nil {
			t.Fatalf("%s: un fichero sin etiquetar no debe traer etiqueta: %+v", vista, sin.Label)
		}
		if carpeta, ok := byID[dir.ID]; ok && carpeta.Label != nil {
			t.Fatalf("%s: una carpeta nunca lleva etiqueta: %+v", vista, carpeta.Label)
		}
	}

	recientes, _, err := st.ListRecent(ctx, store.RecentCursor{}, 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	check("recientes", recientes)

	for _, id := range []uuid.UUID{etiquetado.ID, suelto.ID} {
		if err := st.SetFavorite(ctx, "bob", &id, nil, true); err != nil {
			t.Fatalf("SetFavorite(%s): %v", id, err)
		}
	}
	if err := st.SetFavorite(ctx, "bob", nil, &dir.ID, true); err != nil {
		t.Fatalf("SetFavorite(carpeta): %v", err)
	}
	favs, err := st.ListFavorites(ctx, "bob")
	if err != nil {
		t.Fatalf("ListFavorites: %v", err)
	}
	check("favoritos", favs)

	for _, id := range []uuid.UUID{etiquetado.ID, suelto.ID} {
		if _, err := st.CreateShare(ctx, store.NewShareRecord{
			FileID: &id, Kind: domain.ShareUser, Subject: "bob",
			Permission: domain.PermRead, CreatedBy: "alice",
		}); err != nil {
			t.Fatalf("CreateShare(%s): %v", id, err)
		}
	}
	compartidos, err := st.ListSharedWithMe(ctx, "bob", nil)
	if err != nil {
		t.Fatalf("ListSharedWithMe: %v", err)
	}
	plano := make([]store.Entry, 0, len(compartidos))
	for _, e := range compartidos {
		plano = append(plano, e.Entry)
	}
	check("compartido conmigo", plano)
}

// eDiscovery filtra por etiqueta contra el SNAPSHOT: encuentra lo que estaba
// clasificado como X, aunque la taxonomía haya cambiado desde entonces (Drive ni
// siquiera la consulta aquí).
func TestEDiscoveryFilterByLabel(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.EnsureQuota(ctx, 1<<20); err != nil {
		t.Fatalf("EnsureQuota: %v", err)
	}
	root, err := st.EnsureRoot(ctx)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	const owner = "user-ada"
	etiquetado := seedOwnedFile(t, ctx, st, root.ID, "confidencial.pdf", owner, 10)
	seedOwnedFile(t, ctx, st, root.ID, "normal.pdf", owner, 10)
	if _, err := st.SetFileLabel(ctx, etiquetado.ID, etiquetaConfidencial); err != nil {
		t.Fatalf("SetFileLabel: %v", err)
	}

	// Sin filtro: los dos.
	all, err := st.EDiscoverySearch(ctx, owner, store.EDiscoveryFilter{})
	if err != nil {
		t.Fatalf("EDiscoverySearch: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("sin filtro esperaba 2, got %d", len(all))
	}

	// Filtrando por etiqueta: solo el clasificado, con su snapshot en el resultado.
	hits, err := st.EDiscoverySearch(ctx, owner, store.EDiscoveryFilter{LabelID: etiquetaConfidencial.ID})
	if err != nil {
		t.Fatalf("EDiscoverySearch(label): %v", err)
	}
	if len(hits) != 1 || hits[0].File.ID != etiquetado.ID {
		t.Fatalf("filtro por etiqueta: %+v", hits)
	}
	if hits[0].File.Label == nil || hits[0].File.Label.Name != "Confidencial" {
		t.Fatalf("el resultado debe traer el snapshot: %+v", hits[0].File.Label)
	}

	// Una etiqueta que nadie usó no devuelve nada.
	none, err := st.EDiscoverySearch(ctx, owner, store.EDiscoveryFilter{LabelID: uuid.NewString()})
	if err != nil {
		t.Fatalf("EDiscoverySearch(otra label): %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("etiqueta sin uso debería no devolver nada, got %d", len(none))
	}
}
