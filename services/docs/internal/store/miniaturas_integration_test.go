// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// La miniatura de la portada solo avanza, no mueve el documento en la lista y viaja en
// ella; la de otra organización no se ve, y la de un documento en la papelera tampoco.
func TestMiniaturaDePortada(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")
	sujeto := domain.Subject{Sub: "user-ada", Tenant: "acme"}

	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Acta", OwnerSub: "user-ada",
	})
	if err != nil {
		t.Fatal(err)
	}
	if k, seq, err := st.MiniaturaDe(ctx, doc.ID); err != nil || k != "" || seq != 0 {
		t.Fatalf("un documento nuevo no tiene miniatura: %q %d %v", k, seq, err)
	}
	antes, _ := st.GetDocument(ctx, doc.ID)

	anterior, ok, err := st.GuardarMiniatura(ctx, doc.ID, 5, "docs/x/miniatura/5.png")
	if err != nil || !ok || anterior != "" {
		t.Fatalf("primera miniatura: %q %v %v", anterior, ok, err)
	}
	anterior, ok, err = st.GuardarMiniatura(ctx, doc.ID, 9, "docs/x/miniatura/9.png")
	if err != nil || !ok || anterior != "docs/x/miniatura/5.png" {
		t.Fatalf("la segunda debe sustituir a la primera y devolverla: %q %v %v", anterior, ok, err)
	}
	// Una réplica que se retrasó con una secuencia vieja no pisa la nueva.
	if anterior, ok, err = st.GuardarMiniatura(ctx, doc.ID, 7, "docs/x/miniatura/7.png"); err != nil || ok || anterior != "" {
		t.Fatalf("una miniatura vieja no debe guardarse: %q %v %v", anterior, ok, err)
	}
	if k, seq, _ := st.MiniaturaDe(ctx, doc.ID); k != "docs/x/miniatura/9.png" || seq != 9 {
		t.Fatalf("miniatura vigente %q %d, se esperaba la de la secuencia 9", k, seq)
	}

	despues, _ := st.GetDocument(ctx, doc.ID)
	if !despues.UpdatedAt.Equal(antes.UpdatedAt) {
		t.Error("dibujar la miniatura no es editar: updated_at no debe cambiar")
	}
	lista, err := st.ListDocuments(ctx, sujeto, domain.ListFilter{})
	if err != nil || len(lista) != 1 || lista[0].MiniaturaSeq != 9 {
		t.Fatalf("la lista debe traer la secuencia de la miniatura: %+v %v", lista, err)
	}

	if _, _, err := st.MiniaturaDe(tenantCtx("globex"), doc.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("otra organización no debe ver la miniatura: %v", err)
	}
	if _, ok, _ := st.GuardarMiniatura(tenantCtx("globex"), doc.ID, 99, "x"); ok {
		t.Error("otra organización no debe poder cambiar la miniatura")
	}
	if err := st.SoftDeleteDocument(ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.MiniaturaDe(ctx, doc.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("la miniatura de un documento en la papelera no se sirve: %v", err)
	}
}
