// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// La auditoría agrupa, pagina sin perder ni repetir operaciones y no deja salir el
// valor de lo escrito, solo el nombre de la clave.
func TestAuditoriaOps(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Acta", OwnerSub: "user-ada"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
		PrincipalKind: domain.PrincipalUser, PrincipalID: "user-bob", Role: domain.RoleEditor, GrantedBy: "user-ada",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.IniciarProteccion(ctx, doc.ID, domain.EstadoProteccion{Claves: map[string]domain.ClaveHash{}}); err != nil {
		t.Fatal(err)
	}
	const secreto = "texto-confidencial-9c1e"
	ada, bob := domain.Subject{Sub: "user-ada"}, domain.Subject{Sub: "user-bob"}
	escribe := func(quien domain.Subject, op domain.Op) {
		t.Helper()
		if _, _, err := st.AppendOps(ctx, doc.ID, quien, []domain.Op{op}, nil); err != nil {
			t.Fatal(err)
		}
	}
	// 3 inserciones de Ada (la secuencia por carácter) y 2 `set` de Bob con un valor sensible.
	for i, c := range []string{"h", "o", "l"} {
		escribe(ada, insertOp(nil, "ada", uint64(i+1), c, uint64(i+1)))
	}
	for i := 0; i < 2; i++ {
		n := uint64(10 + i)
		escribe(bob, domain.Op{Kind: domain.OpSet, Dot: domain.Dot{Actor: "bob", Counter: n}, Key: "doc:content", Value: json.RawMessage(`"` + secreto + `"`), Lamport: n})
	}

	// Una página con todo: las cuentas por (autor, tipo) suman lo escrito. No se exige un
	// número de grupos exacto porque un cambio de minuto en mitad del test los parte.
	pag, err := st.AuditoriaOps(ctx, doc.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if pag.SiguienteAntesDe != 0 {
		t.Fatalf("página que cabe entera con siguiente = %d", pag.SiguienteAntesDe)
	}
	suma := map[string]int{}
	for _, e := range pag.Entradas {
		suma[e.AuthorSub+"/"+string(e.Kind)+"/"+e.Key] += e.Cuantas
	}
	if len(suma) != 2 || suma["user-ada/insert/"] != 3 || suma["user-bob/set/doc:content"] != 2 {
		t.Fatalf("resumen = %v", suma)
	}
	if raw, _ := json.Marshal(pag); strings.Contains(string(raw), secreto) {
		t.Fatalf("la auditoría dejó salir el contenido: %s", raw)
	}
	if pag.Entradas[0].SeqHasta != 5 {
		t.Fatalf("la más reciente primero: %+v", pag.Entradas[0])
	}

	// Paginada de 2 en 2: recorre las 5 operaciones una vez cada una.
	vistas, antes := 0, int64(0)
	for pagina := 0; pagina < 10; pagina++ {
		p, err := st.AuditoriaOps(ctx, doc.ID, antes, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range p.Entradas {
			vistas += e.Cuantas
		}
		if p.SiguienteAntesDe == 0 {
			break
		}
		antes = p.SiguienteAntesDe
	}
	if vistas != 5 {
		t.Fatalf("paginando se vieron %d operaciones, quiero 5", vistas)
	}

	// Otro documento no se mezcla.
	otro, _, _ := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Otro", OwnerSub: "user-ada"})
	if p, err := st.AuditoriaOps(ctx, otro.ID, 0, 100); err != nil || len(p.Entradas) != 0 {
		t.Fatalf("documento vacío: %+v, %v", p, err)
	}
}

// Los parches dicen DÓNDE tocaron (hoja/celda), sin el valor escrito.
func TestAuditoriaDeParches(t *testing.T) {
	st := newStore(t)
	svc := domain.NewService(st)
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindSpreadsheet, Title: "Hoja", OwnerSub: "user-ada"})
	if err != nil {
		t.Fatal(err)
	}
	ada := domain.Subject{Sub: "user-ada"}
	libro, _ := json.Marshal(`{"active":0,"sheets":[{"id":"h1","name":"H","cells":{}}]}`)
	if _, _, err := svc.ApplyOps(ctx, ada, doc.ID, []domain.Op{{
		Kind: domain.OpSet, Key: domain.ClaveHoja, Value: libro, Lamport: 1, Dot: domain.Dot{Actor: "ada", Counter: 1},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	const secreto = "sueldo-confidencial-77"
	for i, ref := range []string{"B2", "B3", "B2"} {
		if _, _, err := svc.ApplyOps(ctx, ada, doc.ID, []domain.Op{celdaPatch(t, "p"+string(rune('a'+i)), "h1", ref, secreto+string(rune('a'+i)), 0)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	pag, err := st.AuditoriaOps(ctx, doc.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	total, rutas := 0, map[string]bool{}
	for _, e := range pag.Entradas {
		total += e.Cuantas
		for _, r := range e.Rutas {
			rutas[r] = true
		}
	}
	if total != 4 || len(rutas) != 2 || !rutas["sheets/h1/cells/B2"] || !rutas["sheets/h1/cells/B3"] {
		t.Fatalf("total=%d rutas=%v entradas=%+v", total, rutas, pag.Entradas)
	}
	if raw, _ := json.Marshal(pag); strings.Contains(string(raw), secreto) {
		t.Fatalf("la auditoría dejó salir el valor de una celda: %s", raw)
	}
}
