// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// Contraseña con un texto que no aparece en ninguna otra parte, para buscarla.
const secretoUnico = "s3creto-unico-7f3a"

type modeloT = map[string]any

func valorHoja(t *testing.T, m modeloT) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := json.Marshal(string(b))
	return v
}

func hojaH(celdas, extra modeloT) modeloT {
	h := modeloT{"name": "H", "cells": celdas}
	for k, v := range extra {
		h[k] = v
	}
	return modeloT{"active": 0, "sheets": []any{h}}
}

type banco struct {
	t      *testing.T
	ctx    context.Context
	st     *store.Store
	svc    *domain.Service
	docID  string
	cuenta uint64
}

// escribe aplica un set de doc:sheet por la puerta que usan POST /ops y el WebSocket
// (Service.ApplyOps).
func (b *banco) escribe(quien string, m modeloT, p domain.Proteccion) error {
	b.t.Helper()
	b.cuenta++
	op := domain.Op{
		Kind: domain.OpSet, Key: "doc:sheet", Value: valorHoja(b.t, m),
		Dot: domain.Dot{Actor: quien, Counter: b.cuenta}, Lamport: b.cuenta,
	}
	_, _, err := b.svc.ApplyOps(domain.ConProteccion(b.ctx, &p), domain.Subject{Sub: quien}, b.docID, []domain.Op{op}, nil)
	return err
}

func TestProteccionDeHojaCumplidaEnElServidor(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	b := &banco{t: t, ctx: tenantCtx("acme"), st: st, svc: domain.NewService(st)}
	doc, _, err := st.CreateDocument(b.ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "Cuentas", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	b.docID = doc.ID
	for _, quien := range []string{"editor", "bruto"} {
		if err := st.UpsertPermission(b.ctx, doc.ID, domain.Permission{
			PrincipalKind: domain.PrincipalUser, PrincipalID: quien, Role: domain.RoleEditor, GrantedBy: "owner",
		}); err != nil {
			t.Fatal(err)
		}
	}
	cuenta := func(q string, args ...any) (n int) {
		t.Helper()
		if err := h.Pool.InTenantTx(b.ctx, func(tx pgx.Tx) error { return tx.QueryRow(b.ctx, q, args...).Scan(&n) }); err != nil {
			t.Fatal(err)
		}
		return n
	}
	seq := func() int64 {
		d, err := st.GetDocument(b.ctx, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		return d.Seq
	}

	celdas := modeloT{"A1": "10", "A2": "20", "B1": "libre"}
	libre := modeloT{"B1": modeloT{"locked": false}}
	sinProteger := hojaH(celdas, nil)
	protegida := hojaH(celdas, modeloT{"protect": modeloT{"formatCells": true}, "styles": libre})
	protegidaYa := hojaH(celdas, modeloT{"protect": modeloT{"hasPassword": true, "formatCells": true}, "styles": libre})

	// 1. Un editor escribe y luego protege con contraseña.
	if err := b.escribe("editor", sinProteger, domain.Proteccion{}); err != nil {
		t.Fatalf("escritura normal: %v", err)
	}
	v1, err := b.svc.Snapshot(b.ctx, domain.Subject{Sub: "owner"}, doc.ID, nil, false, 1<<20, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.escribe("editor", protegida, domain.Proteccion{Nuevas: map[string]string{"hoja:H": secretoUnico}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}

	// 2. La contraseña no está en ningún sitio: ni en las operaciones, ni en el estado
	// guardado, ni en las instantáneas; y solo existe su hash.
	for q, nombre := range map[string]string{
		`SELECT count(*) FROM doc_operations WHERE op::text LIKE '%` + secretoUnico + `%'`:   "operaciones",
		`SELECT count(*) FROM doc_proteccion WHERE modelo LIKE '%` + secretoUnico + `%'`:     "estado de protección",
		`SELECT count(*) FROM doc_snapshots WHERE state::text LIKE '%` + secretoUnico + `%'`: "instantáneas",
	} {
		if n := cuenta(q); n != 0 {
			t.Fatalf("la contraseña en claro está en %s", nombre)
		}
	}
	if n := cuenta(`SELECT count(*) FROM doc_proteccion_claves WHERE ambito = 'hoja:H' AND algoritmo = 'SHA-512' AND vueltas = 100000`); n != 1 {
		t.Fatalf("hash guardado: %d", n)
	}

	// 3. Quien solo lee ve que hay contraseña, no cuál: estado y operaciones servidos.
	_ = st.UpsertPermission(b.ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: "lector", Role: domain.RoleViewer, GrantedBy: "owner"})
	estado, _, err := b.svc.LoadFull(b.ctx, domain.Subject{Sub: "lector"}, doc.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if raw := string(estado.Registers["doc:sheet"].Value); strings.Contains(raw, secretoUnico) || !strings.Contains(raw, "hasPassword") {
		t.Fatalf("estado servido: %s", raw)
	}

	// 4. Bypass por la puerta de escritura (POST /ops y WS): sin efecto alguno.
	antes := seq()
	opsAntes := cuenta(`SELECT count(*) FROM doc_operations`)
	eventosAntes := cuenta(`SELECT count(*) FROM events_outbox WHERE type = $1`, store.EventDocUpdated)
	intentos := map[string]modeloT{
		"celda bloqueada":   hojaH(modeloT{"A1": "hackeada", "A2": "20", "B1": "libre"}, modeloT{"protect": modeloT{"hasPassword": true, "formatCells": true}, "styles": libre}),
		"quitar protección": sinProteger,
		"desbloquear celda": hojaH(celdas, modeloT{"protect": modeloT{"hasPassword": true, "formatCells": true}, "styles": modeloT{"A1": modeloT{"locked": false}, "B1": modeloT{"locked": false}}}),
	}
	for nombre, m := range intentos {
		if err := b.escribe("editor", m, domain.Proteccion{}); !errors.Is(err, domain.ErrProtegido) {
			t.Fatalf("%s: %v", nombre, err)
		}
	}
	if err := b.escribe("editor", sinProteger, domain.Proteccion{Clave: "mala"}); !errors.Is(err, domain.ErrClaveProteccion) {
		t.Fatalf("contraseña mala: %v", err)
	}
	// Lote con varias ops: la mala invalida TODO el lote, también lo legítimo.
	b.cuenta++
	lote := []domain.Op{
		{Kind: domain.OpSet, Key: "doc:sheet", Value: valorHoja(t, hojaH(modeloT{"A1": "10", "A2": "20", "B1": "ok"}, modeloT{"protect": modeloT{"hasPassword": true, "formatCells": true}, "styles": libre})), Dot: domain.Dot{Actor: "editor", Counter: b.cuenta}, Lamport: b.cuenta},
		{Kind: domain.OpSet, Key: "doc:sheet", Value: valorHoja(t, sinProteger), Dot: domain.Dot{Actor: "editor", Counter: b.cuenta + 1}, Lamport: b.cuenta + 1},
	}
	b.cuenta++
	if _, _, err := b.svc.ApplyOps(b.ctx, domain.Subject{Sub: "editor"}, doc.ID, lote, nil); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("lote con una op ilegítima: %v", err)
	}
	if seq() != antes || cuenta(`SELECT count(*) FROM doc_operations`) != opsAntes ||
		cuenta(`SELECT count(*) FROM events_outbox WHERE type = $1`, store.EventDocUpdated) != eventosAntes {
		t.Fatal("una escritura denegada dejó rastro (seq, operaciones o eventos)")
	}

	// 5. Una celda desbloqueada sí se edita sin contraseña.
	if err := b.escribe("editor", hojaH(modeloT{"A1": "10", "A2": "20", "B1": "editada"}, modeloT{"protect": modeloT{"hasPassword": true, "formatCells": true}, "styles": libre}), domain.Proteccion{}); err != nil {
		t.Fatalf("editar una celda desbloqueada: %v", err)
	}

	// 5b. La base_seq de la coedición convive con la protección: un lote con una base
	// vieja es obsoleto (409) aunque sea legítimo, y con la base al día, lo rechazado
	// por protección antes no dejó nada que estorbe (la seq de `antes` no avanzó).
	obsoleta := antes
	b.cuenta++
	editada := domain.Op{
		Kind: domain.OpSet, Key: "doc:sheet", Dot: domain.Dot{Actor: "editor", Counter: b.cuenta}, Lamport: b.cuenta,
		Value: valorHoja(t, hojaH(modeloT{"A1": "10", "A2": "20", "B1": "otra"}, modeloT{"protect": modeloT{"hasPassword": true, "formatCells": true}, "styles": libre})),
	}
	if _, _, err := b.svc.ApplyOps(b.ctx, domain.Subject{Sub: "editor"}, doc.ID, []domain.Op{editada}, &obsoleta); !errors.Is(err, domain.ErrStaleSeq) {
		t.Fatalf("base vieja sobre una hoja protegida: %v", err)
	}
	vigente := seq()
	if _, _, err := b.svc.ApplyOps(b.ctx, domain.Subject{Sub: "editor"}, doc.ID, []domain.Op{editada}, &vigente); err != nil {
		t.Fatalf("base al día sobre una hoja protegida: %v", err)
	}

	// 6. Restaurar una versión sin protección sobre la hoja protegida: lo mismo que
	// quitar la protección a mano.
	if _, err := b.svc.RestoreVersion(b.ctx, domain.Subject{Sub: "editor"}, doc.ID, v1.ID, nil); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("restaurar sobre una hoja protegida: %v", err)
	}
	if _, err := b.svc.RestoreVersion(domain.ConProteccion(b.ctx, &domain.Proteccion{Clave: "mala"}), domain.Subject{Sub: "editor"}, doc.ID, v1.ID, nil); !errors.Is(err, domain.ErrClaveProteccion) {
		t.Fatalf("restaurar con mala contraseña: %v", err)
	}

	// 7. Con la contraseña, el editor puede; el propietario no la necesita.
	res, err := b.svc.RestoreVersion(domain.ConProteccion(b.ctx, &domain.Proteccion{Clave: secretoUnico}), domain.Subject{Sub: "editor"}, doc.ID, v1.ID, nil)
	if err != nil || !res.Changed {
		t.Fatalf("restaurar con la contraseña: %+v, %v", res, err)
	}
	b.cuenta += 10 // la restauración usa el reloj más alto que ha visto el documento
	if n := cuenta(`SELECT count(*) FROM doc_proteccion_claves`); n != 0 {
		t.Fatalf("quedó el hash de una protección que ya no existe: %d", n)
	}
	if err := b.escribe("editor", protegidaYa, domain.Proteccion{Nuevas: map[string]string{"hoja:H": "otra"}}); err != nil {
		t.Fatal(err)
	}
	if err := b.escribe("owner", sinProteger, domain.Proteccion{}); err != nil {
		t.Fatalf("el propietario quita la protección sin contraseña: %v", err)
	}

	// 8. Tanteo de contraseñas: tras ocho fallos ni la buena sirve (a esa persona).
	if err := b.escribe("editor", protegidaYa, domain.Proteccion{Nuevas: map[string]string{"hoja:H": secretoUnico}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		_ = b.escribe("bruto", sinProteger, domain.Proteccion{Clave: "intento"})
	}
	if err := b.escribe("bruto", sinProteger, domain.Proteccion{Clave: secretoUnico}); !errors.Is(err, domain.ErrClaveProteccion) {
		t.Fatalf("tras el tanteo, la buena contraseña debía esperar: %v", err)
	}
	if err := b.escribe("editor", sinProteger, domain.Proteccion{Clave: secretoUnico}); err != nil {
		t.Fatalf("otra persona no se ve afectada: %v", err)
	}
}

// Un documento protegido ANTES de esta versión lleva la contraseña en claro en su
// historial. No se pierde la protección, no sale por ninguna lectura y, en cuanto se
// escribe, el modelo vigente deja de tenerla.
func TestProteccionMigraLaContrasenaEnClaroDeUnDocumentoExistente(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	svc := domain.NewService(st)
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "Antigua", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: "editor", Role: domain.RoleEditor, GrantedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	legado := hojaH(modeloT{"A1": "10", "B1": "libre"}, modeloT{"protect": modeloT{"password": "heredada-9c1"}, "styles": modeloT{"B1": modeloT{"locked": false}}})
	op, _ := json.Marshal(domain.Op{Kind: domain.OpSet, Key: "doc:sheet", Value: valorHoja(t, legado), Dot: domain.Dot{Actor: "viejo", Counter: 1}, Lamport: 5})
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO doc_operations (document_id, seq, tenant, actor, author_sub, lamport, op) VALUES ($1, 1, 'acme', 'viejo', 'owner', 5, $2)`, doc.ID, json.RawMessage(op)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE documents SET seq = 1 WHERE id = $1`, doc.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Antes de que nadie escriba: la lectura ya no la sirve.
	estado, _, err := svc.LoadFull(ctx, domain.Subject{Sub: "editor"}, doc.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(estado.Registers["doc:sheet"].Value), "heredada-9c1") {
		t.Fatal("el estado servido lleva la contraseña heredada")
	}
	ops, err := st.OpsSince(ctx, doc.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(domain.RedactarOps(ops)[0].Op.Value), "heredada-9c1") {
		t.Fatal("las operaciones servidas llevan la contraseña heredada")
	}

	// La protección sigue: sin la contraseña no se quita, con ella sí.
	desprotegida := hojaH(modeloT{"A1": "10", "B1": "libre"}, modeloT{"styles": modeloT{"B1": modeloT{"locked": false}}})
	write := func(m modeloT, lamport uint64, p domain.Proteccion) error {
		op := domain.Op{Kind: domain.OpSet, Key: "doc:sheet", Value: valorHoja(t, m), Dot: domain.Dot{Actor: "editor", Counter: lamport}, Lamport: lamport}
		_, _, err := svc.ApplyOps(domain.ConProteccion(ctx, &p), domain.Subject{Sub: "editor"}, doc.ID, []domain.Op{op}, nil)
		return err
	}
	if err := write(desprotegida, 6, domain.Proteccion{}); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("la migración perdió la protección: %v", err)
	}
	var modelo string
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT modelo FROM doc_proteccion WHERE document_id = $1`, doc.ID).Scan(&modelo)
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(modelo, "heredada-9c1") || !strings.Contains(modelo, "hasPassword") {
		t.Fatalf("estado migrado: %s", modelo)
	}
	if err := write(desprotegida, 7, domain.Proteccion{Clave: "heredada-9c1"}); err != nil {
		t.Fatalf("la contraseña heredada ya no abre: %v", err)
	}
}

// El límite de intentos de contraseña cuenta también la que viene DENTRO del modelo (el
// camino de los clientes anteriores) y la que se pone como nueva: agotados, tampoco vale
// la buena. Lo que no pide contraseña se sigue escribiendo, y a otra persona no le afecta.
func TestLimiteDeIntentosDeClavePorCualquierVia(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	b := &banco{t: t, ctx: tenantCtx("acme"), st: st, svc: domain.NewService(st)}
	doc, _, err := st.CreateDocument(b.ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "Cuentas", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	b.docID = doc.ID
	for _, quien := range []string{"ana", "luis"} {
		if err := st.UpsertPermission(b.ctx, doc.ID, domain.Permission{
			PrincipalKind: domain.PrincipalUser, PrincipalID: quien, Role: domain.RoleEditor, GrantedBy: "owner",
		}); err != nil {
			t.Fatal(err)
		}
	}
	const buena = "la-buena-91c2"
	libre := modeloT{"B1": modeloT{"locked": false}}
	protegida := func(celdas, protect modeloT) modeloT {
		return hojaH(celdas, modeloT{"protect": protect, "styles": libre})
	}
	celdas := modeloT{"A1": "10", "B1": "libre"}
	if err := b.escribe("owner", protegida(celdas, modeloT{}), domain.Proteccion{Nuevas: map[string]string{"hoja:H": buena}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	// Una celda bloqueada cambiada, con la contraseña dentro del modelo.
	conClave := func(clave string) modeloT {
		return protegida(modeloT{"A1": "cambiada", "B1": "libre"}, modeloT{"hasPassword": true, "password": clave})
	}

	for i := range 8 {
		if err := b.escribe("ana", conClave(fmt.Sprintf("mala-%d", i)), domain.Proteccion{}); !errors.Is(err, domain.ErrClaveProteccion) {
			t.Fatalf("intento %d dentro del modelo: salió %v, esperaba ErrClaveProteccion", i, err)
		}
	}
	if err := b.escribe("ana", conClave(buena), domain.Proteccion{}); !errors.Is(err, domain.ErrClaveProteccion) {
		t.Errorf("agotados los intentos, la buena dentro del modelo: salió %v", err)
	}
	if err := b.escribe("ana", protegida(celdas, modeloT{"hasPassword": true}), domain.Proteccion{Nuevas: map[string]string{"hoja:H": buena}}); !errors.Is(err, domain.ErrClaveProteccion) {
		t.Errorf("agotados los intentos, la buena como contraseña nueva: salió %v", err)
	}
	if err := b.escribe("ana", protegida(modeloT{"A1": "10", "B1": "otra cosa"}, modeloT{"hasPassword": true}), domain.Proteccion{}); err != nil {
		t.Errorf("agotados los intentos, una celda desbloqueada: %v", err)
	}
	if err := b.escribe("luis", conClave(buena), domain.Proteccion{}); err != nil {
		t.Errorf("otra persona con la buena: %v", err)
	}
}
