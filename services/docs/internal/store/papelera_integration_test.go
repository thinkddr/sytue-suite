// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// TestPapeleraSincronizadaConDrive: el estado del fichero de Drive se aplica al
// documento de su dueño nacido de él, y a ningún otro; dos veces es lo mismo que una.
func TestPapeleraSincronizadaConDrive(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")
	const fichero = "3f1d0c6e-0e0a-4c8e-9d58-2c1a4b6f7e90"
	crear := func(owner string, kind domain.Kind) *domain.Document {
		t.Helper()
		d, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
			Kind: kind, Title: "Informe", OwnerSub: owner, SourceRef: "drive:" + fichero,
		})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	suyo := crear("ana", domain.KindDocument)
	deOtra := crear("bea", domain.KindDocument) // lo abrió desde el fichero de Ana
	vivo := func(d *domain.Document) bool {
		_, err := st.GetDocument(ctx, d.ID)
		return err == nil
	}
	sincro := func(estado string) {
		t.Helper()
		if err := st.SincronizarConDrive(ctx, fichero, "ana", domain.KindDocument, estado); err != nil {
			t.Fatal(err)
		}
	}

	for range 2 {
		sincro(store.DrivePapelera)
	}
	if vivo(suyo) || !vivo(deOtra) {
		t.Fatalf("a la papelera: el suyo vivo=%v, el de Bea vivo=%v", vivo(suyo), vivo(deOtra))
	}
	if docs, _ := st.ListarPapelera(ctx, "ana", "", nil, 0); len(docs) != 1 || docs[0].ID != suyo.ID {
		t.Fatalf("papelera de Ana = %v", docs)
	}
	if docs, _ := st.ListarPapelera(ctx, "ana", domain.KindSpreadsheet, nil, 0); len(docs) != 0 {
		t.Fatalf("papelera de hojas de Ana = %v", docs)
	}
	if _, err := st.DocumentoEnPapelera(ctx, suyo.ID, "bea"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("la papelera de Ana vista por Bea: %v", err)
	}
	for range 2 {
		sincro(store.DriveVivo)
	}
	if !vivo(suyo) {
		t.Fatal("restaurado en Drive y sigue en la papelera de Docs")
	}

	// Si entretanto volvió a abrir el fichero, restaurar el viejo chocaría con el nuevo.
	sincro(store.DrivePapelera)
	nuevo := crear("ana", domain.KindDocument)
	if err := st.RestaurarDocumento(ctx, suyo.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("restaurar con otro vivo del mismo fichero = %v", err)
	}
	sincro(store.DriveVivo) // tampoco lo resucita la cola
	if vivo(suyo) {
		t.Fatal("la cola resucitó un segundo documento del mismo fichero")
	}

	// Purgado en Drive: NADA se borra para siempre desde ahí. El vivo va a la
	// papelera y los que ya estaban en ella siguen: los vacía su dueño.
	sincro(store.DrivePurgado)
	if docs, _ := st.ListarPapelera(ctx, "ana", "", nil, 0); len(docs) != 2 {
		t.Fatalf("tras purgar en Drive, la papelera de Ana = %v; quiero los dos", docs)
	}
	// Vaciar va por páginas: con el cursor del primero, sale el segundo.
	pagina, _ := st.ListarPapelera(ctx, "ana", "", nil, 1)
	resto, _ := st.ListarPapelera(ctx, "ana", "",
		&store.CursorPapelera{BorradoEn: *pagina[0].DeletedAt, ID: pagina[0].ID}, 0)
	if len(pagina) != 1 || len(resto) != 1 || resto[0].ID == pagina[0].ID {
		t.Fatalf("páginas %v y %v", pagina, resto)
	}
	for _, d := range []*domain.Document{nuevo, suyo} {
		if err := st.PurgarDocumento(ctx, d.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.PurgarDocumento(ctx, deOtra.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("purgar uno vivo = %v", err)
	}
	if !vivo(deOtra) {
		t.Fatal("el documento de Bea no se toca")
	}
}

// TestPapeleraCaducadaYReconciliacion: el barrido ve lo caducado de TODAS las
// organizaciones (política de la 0014) y solo lo caducado; PurgarCaducado no toca lo
// restaurado mientras tanto; y la reconciliación deja fuera el fichero que es hogar
// de otro documento vivo.
func TestPapeleraCaducadaYReconciliacion(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	if _, err := st.ResolveAppRole(context.Background()); err != nil {
		t.Fatal(err)
	}
	crear := func(org, owner, ref string) *domain.Document {
		t.Helper()
		d, _, err := st.CreateDocument(tenantCtx(org), domain.CreateDocumentParams{
			Kind: domain.KindDocument, Title: "x", OwnerSub: owner, SourceRef: ref,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SoftDeleteDocument(tenantCtx(org), d.ID); err != nil {
			t.Fatal(err)
		}
		return d
	}
	viejoA := crear("acme", "ana", "")
	viejoB := crear("otra", "bea", "drive:11111111-1111-1111-1111-111111111111")
	reciente := crear("acme", "ana", "")
	admin, err := pgx.Connect(context.Background(), h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	envejecer := func(id string) {
		t.Helper()
		if _, err := admin.Exec(context.Background(),
			`UPDATE documents SET deleted_at = now() - interval '40 days' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
	}
	envejecer(viejoA.ID)
	envejecer(viejoB.ID)
	antes := time.Now().Add(-30 * 24 * time.Hour)
	cad, err := st.PapeleraCaducada(context.Background(), antes, nil, 0)
	if err != nil || len(cad) != 2 {
		t.Fatalf("caducados = %+v, %v; quiero los dos de 40 días y no el reciente", cad, err)
	}
	for _, c := range cad {
		if c.Doc.ID == reciente.ID || (c.Doc.ID == viejoB.ID && c.Tenant != "otra") {
			t.Fatalf("caducado inesperado %+v", c)
		}
	}
	// Por páginas: tras el primero, sale el segundo y ninguno más.
	pag, _ := st.PapeleraCaducada(context.Background(), antes, nil, 1)
	resto, _ := st.PapeleraCaducada(context.Background(), antes,
		&store.CursorPapelera{BorradoEn: *pag[0].Doc.DeletedAt, ID: pag[0].Doc.ID}, 0)
	if len(pag) != 1 || len(resto) != 1 || resto[0].Doc.ID == pag[0].Doc.ID {
		t.Fatalf("páginas %v y %v", pag, resto)
	}
	if ok, err := st.SigueCaducado(tenantCtx("otra"), viejoB.ID, antes); err != nil || !ok {
		t.Fatalf("sigue caducado = %v, %v", ok, err)
	}
	// Restaurado entre el barrido y el borrado —y hasta vuelto a tirar, ya sin
	// caducar—: no se toca.
	if err := st.RestaurarDocumento(tenantCtx("acme"), viejoA.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.PurgarCaducado(tenantCtx("acme"), viejoA.ID, antes); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("purgar uno restaurado = %v", err)
	}
	if ok, _ := st.SigueCaducado(tenantCtx("acme"), viejoA.ID, antes); ok {
		t.Fatal("restaurado y sigue caducado")
	}
	if err := st.SoftDeleteDocument(tenantCtx("acme"), viejoA.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.PurgarCaducado(tenantCtx("acme"), viejoA.ID, antes); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("purgar uno tirado otra vez hoy = %v", err)
	}
	if err := st.PurgarCaducado(tenantCtx("otra"), viejoB.ID, antes); err != nil {
		t.Fatal(err)
	}

	// Reconciliación: dos borrados del mismo fichero cuentan una vez; el fichero de
	// un documento vivo no es candidato.
	ref := "drive:22222222-2222-2222-2222-222222222222"
	crear("acme", "ana", ref)
	crear("acme", "ana", ref)
	vivoRef := "drive:33333333-3333-3333-3333-333333333333"
	crear("acme", "ana", vivoRef)
	if _, _, err := st.CreateDocument(tenantCtx("acme"), domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "vivo", OwnerSub: "ana", SourceRef: vivoRef,
	}); err != nil {
		t.Fatal(err)
	}
	docs, err := st.SinReconciliar(tenantCtx("acme"))
	if err != nil || len(docs) != 1 || docs[0].SourceRef != ref {
		t.Fatalf("sin reconciliar = %+v, %v", docs, err)
	}
}
