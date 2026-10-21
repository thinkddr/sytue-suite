// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// Los rangos editables por persona o grupo se cumplen con la identidad de la sesión
// (Subject), por la misma puerta que POST /ops y el WebSocket.
func TestProteccionRangosEditablesPorPersonaYGrupoEnElServidor(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	b := &banco{t: t, ctx: tenantCtx("acme"), st: st, svc: domain.NewService(st)}
	doc, _, err := st.CreateDocument(b.ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "Reparto", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	b.docID = doc.ID
	for _, p := range []domain.Permission{
		{PrincipalKind: domain.PrincipalUser, PrincipalID: "ana", Role: domain.RoleEditor, GrantedBy: "owner"},
		{PrincipalKind: domain.PrincipalUser, PrincipalID: "luis", Role: domain.RoleEditor, GrantedBy: "owner"},
		{PrincipalKind: domain.PrincipalGroup, PrincipalID: "conta", Role: domain.RoleEditor, GrantedBy: "owner"},
	} {
		if err := st.UpsertPermission(b.ctx, doc.ID, p); err != nil {
			t.Fatal(err)
		}
	}
	celdas := func(b2, a1 string) modeloT { return modeloT{"A1": a1, "B2": b2, "C3": "fijo"} }
	rangos := []any{
		modeloT{"ref": "B2", "users": []any{"ana"}},
		modeloT{"ref": "A1", "groups": []any{"conta"}},
	}
	protegida := func(b2, a1 string) modeloT {
		return hojaH(celdas(b2, a1), modeloT{"protect": modeloT{"hasPassword": true, "ranges": rangos}})
	}
	if err := b.escribe("owner", hojaH(celdas("0", "0"), modeloT{"protect": modeloT{"ranges": rangos}}), domain.Proteccion{Nuevas: map[string]string{"hoja:H": "k"}}); err != nil {
		t.Fatalf("proteger con rangos: %v", err)
	}

	escribeComo := func(sub string, grupos []string, m modeloT) error {
		b.cuenta++
		op := domain.Op{
			Kind: domain.OpSet, Key: "doc:sheet", Value: valorHoja(t, m),
			Dot: domain.Dot{Actor: sub, Counter: b.cuenta}, Lamport: b.cuenta,
		}
		_, _, err := b.svc.ApplyOps(b.ctx, domain.Subject{Sub: sub, Groups: grupos}, doc.ID, []domain.Op{op}, nil)
		return err
	}
	if err := escribeComo("ana", nil, protegida("de ana", "0")); err != nil {
		t.Fatalf("ana en su rango: %v", err)
	}
	if err := escribeComo("eva", []string{"conta"}, protegida("de ana", "de eva")); err != nil {
		t.Fatalf("el grupo en su rango: %v", err)
	}
	for nombre, c := range map[string]struct {
		sub    string
		grupos []string
		m      modeloT
	}{
		"ana en el rango del grupo":  {"ana", nil, protegida("de ana", "otra vez")},
		"luis en el rango de ana":    {"luis", nil, protegida("de luis", "de eva")},
		"grupo en el rango de ana":   {"eva", []string{"conta"}, protegida("de eva", "de eva")},
		"ana en una celda no suya":   {"ana", nil, hojaH(modeloT{"A1": "de eva", "B2": "de ana", "C3": "x"}, modeloT{"protect": modeloT{"hasPassword": true, "ranges": rangos}})},
		"un grupo que no es el suyo": {"luis", []string{"otro"}, protegida("de ana", "de luis")},
	} {
		if err := escribeComo(c.sub, c.grupos, c.m); !errors.Is(err, domain.ErrProtegido) {
			t.Fatalf("%s: esperaba ErrProtegido, salió %v", nombre, err)
		}
	}
}
