// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// Las secciones protegidas se cumplen por la misma puerta que POST /ops y el WebSocket,
// con la identidad de la sesión y contra el último contenido efectivo del documento.
func TestSeccionesProtegidasEnElServidor(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	svc := domain.NewService(st)
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Contrato", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"ana", "otra"} {
		if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
			PrincipalKind: domain.PrincipalUser, PrincipalID: sub, Role: domain.RoleEditor, GrantedBy: "owner",
		}); err != nil {
			t.Fatal(err)
		}
	}
	var n uint64
	escribe := func(sub, contenido string) error {
		n++
		v, _ := json.Marshal(contenido)
		_, _, err := svc.ApplyOps(ctx, domain.Subject{Sub: sub}, doc.ID, []domain.Op{{
			Kind: domain.OpSet, Key: "doc:html", Value: v, Dot: domain.Dot{Actor: sub, Counter: n}, Lamport: n,
		}}, nil)
		return err
	}
	pagina := func(clausula, libre string) string {
		return "<p>" + libre + "</p><div data-sytue-seccion=\"clausula\" data-sytue-puede=\"ana\">" + clausula + "</div>"
	}

	// Sin secciones, cualquiera escribe; un editor no puede crearlas.
	if err := escribe("otra", "<p>borrador</p>"); err != nil {
		t.Fatalf("escritura normal: %v", err)
	}
	if err := escribe("otra", pagina("cláusula", "borrador")); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("un editor creó una sección: %v", err)
	}
	// El propietario la crea.
	if err := escribe("owner", pagina("cláusula", "borrador")); err != nil {
		t.Fatalf("el propietario crea la sección: %v", err)
	}
	// Fuera de la sección, cualquiera; dentro, solo ana.
	if err := escribe("otra", pagina("cláusula", "texto nuevo")); err != nil {
		t.Fatalf("editar fuera de la sección: %v", err)
	}
	if err := escribe("otra", pagina("cambiada", "texto nuevo")); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("un editor cambió una sección ajena: %v", err)
	}
	if err := escribe("ana", pagina("cambiada por ana", "texto nuevo")); err != nil {
		t.Fatalf("ana edita su sección: %v", err)
	}
	// Un guardado denegado no dejó rastro: el contenido vigente es el de ana.
	// (la prueba posterior solo pasa si el estado de protección es el de ana)
	if err := escribe("otra", pagina("cambiada por ana", "otro texto")); err != nil {
		t.Fatalf("tras la edición de ana, el contenido vigente es el suyo: %v", err)
	}
	// Quitar la sección, solo el propietario.
	if err := escribe("ana", "<p>nada</p>"); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("ana quitó la sección: %v", err)
	}
	if err := escribe("owner", "<p>nada</p>"); err != nil {
		t.Fatalf("el propietario quita la sección: %v", err)
	}
}

// Las operaciones por carácter de la secuencia RGA pasan por las mismas secciones: teclear
// dentro de una sección ajena, partirla o saltarla con un `set` limpio de doc:html no
// llega a la secuencia que ven los co-editores (ROADMAP-OFFICE permisos-1).
func TestSeccionesPorCaracterEnElServidor(t *testing.T) {
	h := harnessComoEnProduccion(t)
	st := store.New(h.Pool)
	svc := domain.NewService(st)
	ctx := tenantCtx("acme")
	doc, _, err := st.CreateDocument(ctx, domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "Contrato", OwnerSub: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"ana", "otra"} {
		if err := st.UpsertPermission(ctx, doc.ID, domain.Permission{
			PrincipalKind: domain.PrincipalUser, PrincipalID: sub, Role: domain.RoleEditor, GrantedBy: "owner",
		}); err != nil {
			t.Fatal(err)
		}
	}
	const pagina = `<p>intro</p><div data-sytue-seccion="clausula" data-sytue-puede="ana">cláusula</div>`
	var n uint64
	// chars: un elemento por carácter a continuación de `tras`, como el editor.
	chars := func(actor string, tras domain.Dot, s string) ([]domain.Op, []domain.Dot) {
		ops := make([]domain.Op, 0, len(s))
		dots := make([]domain.Dot, 0, len(s))
		for _, r := range s {
			n++
			v, _ := json.Marshal(string(r))
			after := tras
			d := domain.Dot{Actor: actor, Counter: n}
			ops = append(ops, domain.Op{Kind: domain.OpInsert, Dot: d, After: &after, Value: v, Lamport: n})
			dots, tras = append(dots, d), d
		}
		return ops, dots
	}
	setHTML := func(actor, contenido string) domain.Op {
		n++
		v, _ := json.Marshal(contenido)
		return domain.Op{Kind: domain.OpSet, Key: "doc:html", Value: v, Dot: domain.Dot{Actor: actor, Counter: n}, Lamport: 1000 + n}
	}
	aplica := func(sub string, ops ...domain.Op) error {
		_, _, err := svc.ApplyOps(ctx, domain.Subject{Sub: sub}, doc.ID, ops, nil)
		return err
	}
	texto := func() string {
		loaded, err := st.LoadState(ctx, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		return loaded.State.PlainText()
	}
	copiaNula := func() bool {
		var nula bool
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT secuencia IS NULL FROM doc_proteccion WHERE document_id = $1`, doc.ID).Scan(&nula)
		}); err != nil {
			t.Fatal(err)
		}
		return nula
	}

	// El propietario siembra el documento con secciones: el texto va a la secuencia y al registro.
	ops, dots := chars("owner", domain.Dot{}, pagina)
	if err := aplica("owner", append(ops, setHTML("owner", pagina))...); err != nil {
		t.Fatalf("el propietario siembra: %v", err)
	}
	if !copiaNula() {
		t.Fatal("el propietario no se valida, así que la copia no se establece todavía")
	}
	if texto() != pagina {
		t.Fatalf("la secuencia no es el HTML: %q", texto())
	}
	finIntro := dots[len("<p>intro")-1]
	dentro := dots[strings.Index(pagina, "cláusula")+3]

	// Fuera de las secciones, quien no figura teclea (esto reconstruye la copia y reintenta).
	ops, _ = chars("otra", finIntro, " nuevo")
	if err := aplica("otra", ops...); err != nil {
		t.Fatalf("teclear fuera de las secciones: %v", err)
	}
	if copiaNula() {
		t.Fatal("la escritura de un no propietario debía establecer la copia")
	}
	if !strings.Contains(texto(), "<p>intro nuevo</p>") {
		t.Fatalf("lo tecleado no llegó: %q", texto())
	}
	vigente := texto()

	// Dentro de una sección ajena: no, ni limpio ni con un `set` idéntico delante.
	ops, _ = chars("otra", dentro, "TRUCADO")
	if err := aplica("otra", ops...); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("un editor tecleó en una sección ajena: %v", err)
	}
	ops, _ = chars("otra", dentro, "TRUCADO")
	if err := aplica("otra", append([]domain.Op{setHTML("otra", strings.Replace(pagina, "<p>intro</p>", "<p>intro nuevo</p>", 1))}, ops...)...); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("un set limpio de doc:html tapó ops sucias: %v", err)
	}
	// Partirla.
	ops, _ = chars("otra", dentro, "</div><div>")
	if err := aplica("otra", ops...); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("un editor partió una sección: %v", err)
	}
	// Borrarle texto.
	d := dentro
	n++
	if err := aplica("otra", domain.Op{Kind: domain.OpDelete, Target: &d, Dot: domain.Dot{Actor: "otra", Counter: n}, Lamport: n}); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("un editor borró texto de una sección ajena: %v", err)
	}
	if texto() != vigente {
		t.Fatalf("una escritura denegada dejó rastro en la secuencia: %q", texto())
	}

	// La persona con permiso sí, y su cambio es el nuevo punto de partida.
	ops, _ = chars("ana", dentro, " (ana)")
	if err := aplica("ana", ops...); err != nil {
		t.Fatalf("ana edita su sección: %v", err)
	}
	if !strings.Contains(texto(), "(ana)") {
		t.Fatalf("lo de ana no llegó: %q", texto())
	}

	// El propietario quita las secciones: la copia deja de existir; al volver a ponerlas se
	// reconstruye con lo que haya.
	if err := aplica("owner", setHTML("owner", "<p>nada</p>")); err != nil {
		t.Fatalf("el propietario quita las secciones: %v", err)
	}
	if !copiaNula() {
		t.Fatal("sin secciones la copia debía descartarse")
	}
	ops, _ = chars("otra", finIntro, " libre")
	if err := aplica("otra", ops...); err != nil {
		t.Fatalf("sin secciones se teclea donde sea: %v", err)
	}
	if err := aplica("owner", setHTML("owner", pagina)); err != nil {
		t.Fatalf("el propietario vuelve a poner secciones: %v", err)
	}
	ops, _ = chars("otra", dentro, "OTRA VEZ")
	if err := aplica("otra", ops...); !errors.Is(err, domain.ErrProtegido) {
		t.Fatalf("tras reponer las secciones, la copia reconstruida protege: %v", err)
	}
}
