// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Un documento que nace de una plantilla nace CON su contenido, en la misma
// transacción: no hay instante en que exista vacío. Y si el documento ya existía
// (mismo fichero de origen), la semilla no se vuelve a escribir encima.
func TestCrearConSemilla(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")

	op, err := domain.ContentOp(domain.KindDocument, "<h1>Acta</h1>")
	if err != nil {
		t.Fatal(err)
	}
	doc, creado, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Acta", OwnerSub: "user-ada",
		SourceRef: "drive:plantilla", Seed: []domain.Op{op},
	})
	if err != nil || !creado {
		t.Fatalf("CreateDocument: %v (creado=%v)", err, creado)
	}
	if doc.Seq != 1 {
		t.Fatalf("seq del documento recién sembrado = %d, quiero 1", doc.Seq)
	}
	cargado, err := st.LoadState(ctx, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := cargado.State.Materialize().Registers["doc:html"]; string(got) != `"<h1>Acta</h1>"` {
		t.Fatalf("contenido sembrado = %s", got)
	}

	// Segunda vez con el mismo origen: el mismo documento y ni una op más.
	otra, _ := domain.ContentOp(domain.KindDocument, "<p>pisaría</p>")
	mismo, creado, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Acta", OwnerSub: "user-ada",
		SourceRef: "drive:plantilla", Seed: []domain.Op{otra},
	})
	if err != nil || creado || mismo.ID != doc.ID {
		t.Fatalf("idempotencia: %v creado=%v id=%s", err, creado, mismo.ID)
	}
	if ops, _ := st.OpsSince(ctx, doc.ID, 0, 0); len(ops) != 1 {
		t.Fatalf("ops tras recrear = %d, quiero 1", len(ops))
	}

	// Una semilla inválida no deja documento.
	_, _, err = st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindDocument, Title: "Roto", OwnerSub: "user-ada",
		Seed: []domain.Op{{Kind: domain.OpSet}},
	})
	if err == nil {
		t.Fatal("semilla sin clave aceptada")
	}
}
