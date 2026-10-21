// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

func celdaPatch(t *testing.T, opID, hoja, ref, valor string, base int64) domain.Op {
	t.Helper()
	ruta := func(segs ...any) []json.RawMessage {
		out := make([]json.RawMessage, len(segs))
		for i, s := range segs {
			out[i], _ = json.Marshal(s)
		}
		return out
	}
	v, _ := json.Marshal(valor)
	return domain.Op{
		Kind: domain.OpPatch, Key: domain.ClaveHoja, OpID: opID, Base: base,
		Cambios: []domain.Cambio{{Path: ruta("sheets", map[string]string{"id": hoja}, "cells", ref), Value: v}},
	}
}

// Parches de celda: dos personas con la misma base se aplican las dos en orden de
// seq (sin 409), reenviar uno no lo repite y un cliente que escribe el registro
// completo con una base vieja sigue recibiendo su 409.
func TestParchesDeCelda(t *testing.T) {
	st := newStore(t)
	svc := domain.NewService(st)
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindSpreadsheet, Title: "Hoja", OwnerSub: "user-ada",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
		PrincipalKind: domain.PrincipalUser, PrincipalID: "user-bob", Role: domain.RoleEditor, GrantedBy: "user-ada",
	}); err != nil {
		t.Fatal(err)
	}
	ada, bob := domain.Subject{Sub: "user-ada"}, domain.Subject{Sub: "user-bob"}
	libro, _ := json.Marshal(`{"active":0,"sheets":[{"id":"h1","name":"H","cells":{}}]}`)
	if _, seq, err := svc.ApplyOps(ctx, ada, doc.ID, []domain.Op{{
		Kind: domain.OpSet, Key: domain.ClaveHoja, Value: libro, Lamport: 1,
		Dot: domain.Dot{Actor: "ada", Counter: 1},
	}}, nil); err != nil || seq != 1 {
		t.Fatalf("siembra: seq=%d err=%v", seq, err)
	}

	// Los dos con base 1: no hay conflicto, quedan las dos celdas.
	if _, seq, err := svc.ApplyOps(ctx, ada, doc.ID, []domain.Op{celdaPatch(t, "p-ada", "h1", "A1", "de ada", 1)}, nil); err != nil || seq != 2 {
		t.Fatalf("parche de ada: seq=%d err=%v", seq, err)
	}
	base := int64(1)
	if _, seq, err := svc.ApplyOps(ctx, bob, doc.ID, []domain.Op{celdaPatch(t, "p-bob", "h1", "B1", "de bob", 1)}, &base); err != nil || seq != 3 {
		t.Fatalf("parche de bob con base vieja: seq=%d err=%v", seq, err)
	}
	// Idempotencia: el reintento no escribe nada y devuelve la seq vigente.
	out, seq, err := svc.ApplyOps(ctx, bob, doc.ID, []domain.Op{celdaPatch(t, "p-bob", "h1", "B1", "de bob", 1)}, nil)
	if err != nil || seq != 3 || len(out) != 0 {
		t.Fatalf("reintento: seq=%d ops=%d err=%v", seq, len(out), err)
	}
	// Mismo lote con el mismo op_id dos veces: una sola vez.
	if _, seq, err := svc.ApplyOps(ctx, ada, doc.ID, []domain.Op{
		celdaPatch(t, "p-2", "h1", "C1", "x", 3), celdaPatch(t, "p-2", "h1", "C1", "x", 3),
	}, nil); err != nil || seq != 4 {
		t.Fatalf("lote con op_id repetido: seq=%d err=%v", seq, err)
	}

	// Un cliente que escribe el registro completo con base obsoleta: 409 de siempre.
	viejo := int64(1)
	if _, _, err := svc.ApplyOps(ctx, bob, doc.ID, []domain.Op{{
		Kind: domain.OpSet, Key: domain.ClaveHoja, Value: libro, Lamport: 2,
		Dot: domain.Dot{Actor: "bob", Counter: 2},
	}}, &viejo); !errors.Is(err, domain.ErrStaleSeq) {
		t.Fatalf("escritura completa con base vieja = %v, quiero ErrStaleSeq", err)
	}

	// El log: sets ordinarios del registro, con op_id y cambios (auditoría), y el
	// último contiene todo.
	ops, err := st.OpsSince(ctx, doc.ID, 1, 10)
	if err != nil || len(ops) != 3 {
		t.Fatalf("OpsSince: %d ops, err=%v", len(ops), err)
	}
	for _, o := range ops {
		if o.Op.Kind != domain.OpSet || o.Op.OpID == "" || len(o.Op.Cambios) == 0 {
			t.Fatalf("op materializada mal: %+v", o.Op)
		}
	}
	var contenido string
	_ = json.Unmarshal(ops[2].Op.Value, &contenido)
	want := `{"active":0,"sheets":[{"cells":{"A1":"de ada","B1":"de bob","C1":"x"},"id":"h1","name":"H"}]}`
	if contenido != want {
		t.Fatalf("contenido final:\n got  %s\n want %s", contenido, want)
	}

	// Sin contenido previo (otro documento) el parche es inválido.
	otro, _, _ := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "Vacía", OwnerSub: "user-ada"})
	if _, _, err := svc.ApplyOps(ctx, ada, otro.ID, []domain.Op{celdaPatch(t, "p-x", "h1", "A1", "x", 0)}, nil); !errors.Is(err, domain.ErrInvalidOp) {
		t.Fatalf("parche sin contenido = %v, quiero ErrInvalidOp", err)
	}
}

// La protección de hoja se cumple igual con parches: se materializan en el mismo `set` que
// valida el servidor, así que un parche no es una puerta lateral.
func TestParchesRespetanLaProteccion(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	b := &banco{t: t, ctx: tenantCtx("acme"), st: st, svc: domain.NewService(st)}
	doc, _, err := st.CreateDocument(b.ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "Cuentas", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	b.docID = doc.ID
	if err := st.UpsertPermission(b.ctx, doc.ID, domain.Permission{
		PrincipalKind: domain.PrincipalUser, PrincipalID: "bruto", Role: domain.RoleEditor, GrantedBy: "owner",
	}); err != nil {
		t.Fatal(err)
	}
	celdas := modeloT{"A1": "10", "B1": "libre"}
	hoja := modeloT{
		"id": "h1", "name": "H", "cells": celdas,
		"protect": modeloT{"formatCells": true},
		"styles":  modeloT{"B1": modeloT{"locked": false}},
	}
	if err := b.escribe("owner", modeloT{"active": 0, "sheets": []any{hoja}}, domain.Proteccion{Nuevas: map[string]string{"hoja:H": secretoUnico}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	bruto := domain.Subject{Sub: "bruto"}
	parche := func(opID, ref string) []domain.Op { return []domain.Op{celdaPatch(t, opID, "h1", ref, "x", 0)} }

	// Una celda bloqueada: rechazada y sin rastro. Una desbloqueada: pasa.
	antes, _ := st.GetDocument(b.ctx, doc.ID)
	if _, _, err := b.svc.ApplyOps(b.ctx, bruto, doc.ID, parche("p1", "A1"), nil); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("parche sobre celda bloqueada = %v, quiero ErrProtegido", err)
	}
	if d, _ := st.GetDocument(b.ctx, doc.ID); d.Seq != antes.Seq {
		t.Fatalf("el rechazo escribió: seq %d -> %d", antes.Seq, d.Seq)
	}
	if _, _, err := b.svc.ApplyOps(b.ctx, bruto, doc.ID, parche("p2", "B1"), nil); err != nil {
		t.Fatalf("parche sobre celda libre: %v", err)
	}
	// Quitar la protección con un parche tampoco: el servidor ve el modelo resultante.
	quitar := domain.Op{Kind: domain.OpPatch, Key: domain.ClaveHoja, OpID: "p3", Cambios: []domain.Cambio{{
		Path: []json.RawMessage{json.RawMessage(`"sheets"`), json.RawMessage(`{"id":"h1"}`), json.RawMessage(`"protect"`)}, Del: true,
	}}}
	if _, _, err := b.svc.ApplyOps(b.ctx, bruto, doc.ID, []domain.Op{quitar}, nil); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("quitar la protección con un parche = %v, quiero ErrProtegido", err)
	}
}
