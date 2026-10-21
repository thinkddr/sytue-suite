// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Vistas de filtro personales (0018): una fila por (documento, persona); nadie lee la de
// otra persona ni desde otra organización, y se van con el documento.
func TestVistasFiltroPorPersonaYOrganizacion(t *testing.T) {
	st := newStore(t)
	acme, otra := tenantCtx("acme"), tenantCtx("otra")
	doc, _, err := st.CreateDocument(acme, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "V", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	ana := json.RawMessage(`{"views":{"H":[{"id":"v1","name":"Ana"}]},"active":{"H":"v1"}}`)
	if err := st.GuardarVistasFiltro(acme, doc.ID, "ana", ana); err != nil {
		t.Fatal(err)
	}

	got, ok, err := st.VistasFiltro(acme, doc.ID, "ana")
	if err != nil || !ok || string(got) == "" {
		t.Fatalf("Ana no recupera lo suyo: %v %v %s", err, ok, got)
	}
	var e struct{ Active map[string]string }
	if err := json.Unmarshal(got, &e); err != nil || e.Active["H"] != "v1" {
		t.Fatalf("estado de Ana: %s", got)
	}
	// El propietario no ve la fila de Ana: la consulta es por su propio sub.
	if _, ok, err := st.VistasFiltro(acme, doc.ID, "owner"); err != nil || ok {
		t.Fatalf("el propietario ve vistas ajenas: %v %v", ok, err)
	}
	// Otra organización no ve nada, ni con el mismo sub y el mismo id de documento (RLS).
	if _, ok, err := st.VistasFiltro(otra, doc.ID, "ana"); err != nil || ok {
		t.Fatalf("otra organización ve vistas ajenas: %v %v", ok, err)
	}
	if err := st.GuardarVistasFiltro(otra, doc.ID, "ana", ana); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra organización escribe en un documento ajeno: %v", err)
	}

	// Guardar otra vez sustituye (último gana).
	if err := st.GuardarVistasFiltro(acme, doc.ID, "ana", json.RawMessage(`{"views":{},"active":{}}`)); err != nil {
		t.Fatal(err)
	}
	got, _, _ = st.VistasFiltro(acme, doc.ID, "ana")
	if string(got) != `{"active": {}, "views": {}}` && string(got) != `{"views": {}, "active": {}}` {
		t.Fatalf("no sustituyó: %s", got)
	}

	// Se van con el documento (ON DELETE CASCADE): se purga desde la papelera.
	if err := st.SoftDeleteDocument(acme, doc.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.PurgarDocumento(acme, doc.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.VistasFiltro(acme, doc.ID, "ana"); ok {
		t.Fatal("las vistas sobreviven al documento")
	}
}
