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

// Un script NUNCA es una vía para saltarse la protección: tener una ejecución autorizada
// (script aprobado, cuota reservada, rol de editor) no cambia en nada lo que `/ops` deja
// escribir. Lo que el script despacha acaba en la misma puerta (Service.ApplyOps), que
// reautoriza ACL y protección en cada operación.
func TestUnScriptAutorizadoNoSaltaLaProteccionNiLaACL(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	b := &banco{t: t, ctx: tenantCtx("acme"), st: st, svc: domain.NewService(st)}
	doc, _, err := st.CreateDocument(b.ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "Cuentas", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	b.docID = doc.ID
	for quien, rol := range map[string]domain.Role{"editor": domain.RoleEditor, "lector": domain.RoleViewer} {
		if err := st.UpsertPermission(b.ctx, doc.ID, domain.Permission{
			PrincipalKind: domain.PrincipalUser, PrincipalID: quien, Role: rol, GrantedBy: "owner",
		}); err != nil {
			t.Fatal(err)
		}
	}
	celdas := modeloT{"A1": "10", "A2": "20"}
	protegida := hojaH(celdas, modeloT{"protect": modeloT{"formatCells": true}})
	if err := b.escribe("owner", protegida, domain.Proteccion{Nuevas: map[string]string{"hoja:H": secretoUnico}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}

	// Un script de la organización, aprobado, con manifiesto que escribe.
	sc, v, err := st.CrearScript(b.ctx, domain.Script{Name: "Pisa", Kind: domain.KindSpreadsheet, Scope: domain.AmbitoOrg, AuthorSub: "owner"},
		"await sytue.v1.run('bold')", []byte(`{"v":1,"commands":["bold"],"read":[],"write":true,"scope":{},"max_ops":1000,"max_ms":5000,"unattended":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ProponerVersionScript(b.ctx, sc.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.RevisarVersionScript(b.ctx, sc.ID, 1, v.CodeHash, true, "owner"); err != nil {
		t.Fatal(err)
	}
	for _, quien := range []string{"editor", "lector"} {
		if _, err := st.ReservarEjecucion(b.ctx, domain.EjecucionScript{ScriptID: sc.ID, Version: 1, CodeHash: v.CodeHash, DocumentID: doc.ID, Sub: quien}, 5000); err != nil {
			t.Fatalf("la ejecución de %s no se autorizó: %v", quien, err)
		}
	}

	pisada := hojaH(modeloT{"A1": "999", "A2": "20"}, modeloT{"protect": modeloT{"formatCells": true}})
	// El editor con la ejecución autorizada sigue sin poder cambiar una celda bloqueada…
	if err := b.escribe("editor", pisada, domain.Proteccion{}); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("el editor con un script autorizado escribió en una hoja protegida: %v", err)
	}
	// …y el lector, con la suya, no puede escribir nada.
	if err := b.escribe("lector", pisada, domain.Proteccion{}); err == nil {
		t.Fatal("un lector con un script autorizado escribió")
	}
}
