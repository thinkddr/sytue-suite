// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// La protección de una presentación se cumple por la misma puerta que POST /ops y el
// WebSocket, con la identidad de la sesión y la contraseña solo como hash.
func TestProteccionDeDiapasCumplidaEnElServidor(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	svc := domain.NewService(st)
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindPresentation, Title: "Plan", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"editor", "lector"} {
		rol := domain.RoleEditor
		if sub == "lector" {
			rol = domain.RoleViewer
		}
		if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
			PrincipalKind: domain.PrincipalUser, PrincipalID: sub, Role: rol, GrantedBy: "owner",
		}); err != nil {
			t.Fatal(err)
		}
	}
	var n uint64
	escribe := func(sub, modelo string, p domain.Proteccion) error {
		n++
		v, _ := json.Marshal(modelo)
		_, _, err := svc.ApplyOps(domain.ConProteccion(ctx, &p), domain.Subject{Sub: sub}, doc.ID, []domain.Op{{
			Kind: domain.OpSet, Key: "doc:slides", Value: v, Dot: domain.Dot{Actor: sub, Counter: n}, Lamport: n,
		}}, nil)
		return err
	}
	modelo := func(texto, protect string) string {
		p := ""
		if protect != "" {
			p = `,"protect":` + protect
		}
		return fmt.Sprintf(`{"slides":[{"id":"s1","boxes":[{"id":"b1","kind":"text","content":%q}]},{"id":"s2","boxes":[]}]%s}`, texto, p)
	}
	const secreto = "s3creto-diapos-5d1e"

	if err := escribe("editor", modelo("hola", ""), domain.Proteccion{}); err != nil {
		t.Fatalf("escritura normal: %v", err)
	}
	// Un editor protege con contraseña: el valor registrado no la lleva y la tabla solo su hash.
	if err := escribe("editor", modelo("hola", `{"slides":["s1"],"password":"`+secreto+`"}`), domain.Proteccion{Nuevas: map[string]string{"diapos": secreto}}); err != nil {
		t.Fatalf("proteger: %v", err)
	}
	cuenta := func(q string, args ...any) (c int) {
		t.Helper()
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { return tx.QueryRow(ctx, q, args...).Scan(&c) }); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if c := cuenta(`SELECT count(*) FROM doc_operations WHERE document_id = $1 AND op::text LIKE '%' || $2 || '%'`, doc.ID, secreto); c != 0 {
		t.Fatalf("la contraseña quedó en el registro de operaciones (%d)", c)
	}
	if c := cuenta(`SELECT count(*) FROM doc_proteccion_claves WHERE document_id = $1 AND ambito = 'diapos' AND algoritmo = 'SHA-512' AND vueltas = 100000 AND length(hash) = 64`, doc.ID); c != 1 {
		t.Fatal("no hay el hash de la contraseña")
	}
	if c := cuenta(`SELECT count(*) FROM doc_proteccion WHERE document_id = $1 AND modelo LIKE '%' || $2 || '%'`, doc.ID, secreto); c != 0 {
		t.Fatal("la contraseña quedó en el estado de protección")
	}
	loaded, err := st.LoadState(ctx, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(loaded.State)
	if strings.Contains(string(raw), secreto) || !strings.Contains(string(raw), `hasPassword`) {
		t.Fatalf("el estado servido: %s", raw)
	}

	// Lo bloqueado: no sin contraseña, no con una mala, sí con la buena y el propietario sí.
	cambiada := modelo("TRUCADA", `{"slides":["s1"],"hasPassword":true}`)
	if err := escribe("editor", cambiada, domain.Proteccion{}); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("un editor cambió una diapositiva bloqueada: %v", err)
	}
	if err := escribe("editor", cambiada, domain.Proteccion{Clave: "mala"}); !errors.Is(err, domain.ErrClaveProteccion) {
		t.Fatalf("con una contraseña mala: %v", err)
	}
	if err := escribe("lector", cambiada, domain.Proteccion{Clave: secreto}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("un lector con la contraseña: %v", err)
	}
	if err := escribe("editor", cambiada, domain.Proteccion{Clave: secreto}); err != nil {
		t.Fatalf("con la contraseña: %v", err)
	}
	if err := escribe("owner", modelo("del dueño", `{"slides":["s1"],"hasPassword":true}`), domain.Proteccion{}); err != nil {
		t.Fatalf("el propietario: %v", err)
	}
	// Quitar la protección: el propietario; la contraseña se borra con ella.
	if err := escribe("editor", modelo("del dueño", ""), domain.Proteccion{}); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("un editor quitó la protección: %v", err)
	}
	if err := escribe("owner", modelo("del dueño", ""), domain.Proteccion{}); err != nil {
		t.Fatalf("el propietario quita la protección: %v", err)
	}
	if c := cuenta(`SELECT count(*) FROM doc_proteccion_claves WHERE document_id = $1`, doc.ID); c != 0 {
		t.Fatal("la contraseña sobrevivió a la protección")
	}
	if err := escribe("editor", modelo("libre otra vez", ""), domain.Proteccion{}); err != nil {
		t.Fatalf("sin protección escribe cualquier editor: %v", err)
	}
}

// Una caja de medio con un origen o un identificador que los editores no admiten no se
// guarda, ni por la puerta de edición ni como contenido inicial de un documento.
func TestMediosDeUnaPresentacionSeValidanAlGuardar(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	svc := domain.NewService(st)
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindPresentation, Title: "Plan", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	set := func(n uint64, caja string) domain.Op {
		v, _ := json.Marshal(`{"slides":[{"id":"s1","boxes":[` + caja + `]}]}`)
		return domain.Op{Kind: domain.OpSet, Key: "doc:slides", Value: v, Dot: domain.Dot{Actor: "owner", Counter: n}, Lamport: n}
	}
	const (
		buena = `{"id":"s1b1","kind":"media","src":"https://media.example.eu/v.mp4"}`
		mala  = `{"id":"../b1","kind":"media","src":"data:text/plain;base64,AAAA"}`
	)
	owner := domain.Subject{Sub: "owner"}
	if _, _, err := svc.ApplyOps(ctx, owner, doc.ID, []domain.Op{set(1, buena)}, nil); err != nil {
		t.Fatalf("una caja de medio válida no se guarda: %v", err)
	}
	if _, _, err := svc.ApplyOps(ctx, owner, doc.ID, []domain.Op{set(2, mala)}, nil); !errors.Is(err, domain.ErrInvalidOp) {
		t.Errorf("al editar: salió %v, esperaba ErrInvalidOp", err)
	}
	if d, err := st.GetDocument(ctx, doc.ID); err != nil || d.Seq != 1 {
		t.Errorf("el documento avanzó con la escritura rechazada: %+v (err %v)", d, err)
	}
	_, _, err = st.CreateDocument(ctx, domain.CreateDocumentParams{
		Kind: domain.KindPresentation, Title: "Otra", OwnerSub: "owner", Seed: []domain.Op{set(1, mala)},
	})
	if !errors.Is(err, domain.ErrInvalidOp) {
		t.Errorf("al crear con contenido: salió %v, esperaba ErrInvalidOp", err)
	}
}
