// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Borrar un comentario o una respuesta: solo su autor, o quien edita o administra el
// documento (docs/security.md#matriz-acl). Un comentarista borra los suyos y nada más.
// Va contra PostgreSQL porque la regla vive en la sentencia del almacén. Los comentarios
// de un fichero de Word los guarda, a su nombre, quien importa: son de esa persona.
func TestBorrarComentariosPorRol(t *testing.T) {
	e := nuevoEscenarioPermisos(t, domain.KindDocument)
	e.conceder("otra", domain.RoleCommenter)

	nuevo := func(autor string, padre *string) string {
		t.Helper()
		c, err := e.st.CreateComment(e.ctx, domain.CreateCommentParams{DocumentID: e.doc.ID, ParentID: padre, AuthorSub: autor, Body: "x"})
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	borrar := func(quien, id string) int {
		rec := httptest.NewRecorder()
		e.hd.deleteComment(rec, e.peticion("DELETE", quien, "", map[string]string{"commentId": id}, "/comments/"+id))
		return rec.Code
	}

	casos := []struct {
		quien, autor string
		quiere       int
	}{
		{"comenta", "comenta", http.StatusNoContent}, // el suyo
		{"comenta", "otra", http.StatusForbidden},    // el de otro comentarista
		{"comenta", "ana", http.StatusForbidden},     // el de un editor
		{"comenta", "owner", http.StatusForbidden},   // el del propietario
		{"lector", "lector", http.StatusForbidden},   // un lector no comenta, tampoco borra
		{"ana", "comenta", http.StatusNoContent},     // un editor modera
		{"ana", "ana", http.StatusNoContent},
		{"owner", "comenta", http.StatusNoContent}, // el propietario modera
		{"owner", "luis", http.StatusNoContent},
		{"intruso", "comenta", http.StatusForbidden}, // sin permiso en el documento
	}
	for _, c := range casos {
		id := nuevo(c.autor, nil)
		if got := borrar(c.quien, id); got != c.quiere {
			t.Errorf("%s borra el comentario de %s: HTTP %d, se esperaba %d", c.quien, c.autor, got, c.quiere)
		}
		// Lo que se negó sigue vivo: ni soft-delete a medias.
		vivos, err := e.st.ListComments(e.ctx, e.doc.ID, true)
		if err != nil {
			t.Fatal(err)
		}
		existe := false
		for _, v := range vivos {
			existe = existe || v.ID == id
		}
		if existe == (c.quiere == http.StatusNoContent) {
			t.Errorf("%s borra el de %s: HTTP %d pero existe=%v", c.quien, c.autor, c.quiere, existe)
		}
	}

	// Las respuestas siguen la misma regla: la de otra persona, solo con rol de moderación.
	raiz := nuevo("owner", nil)
	resp := nuevo("otra", &raiz)
	if got := borrar("comenta", resp); got != http.StatusForbidden {
		t.Errorf("comentarista borra la respuesta de otro: HTTP %d", got)
	}
	if got := borrar("otra", resp); got != http.StatusNoContent {
		t.Errorf("la autora borra su respuesta: HTTP %d", got)
	}

	// Comentarios importados de Word (por el manejador real): su autor es quien importó,
	// raíz y respuesta.
	rec := httptest.NewRecorder()
	e.hd.importComments(rec, e.peticion("POST", "comenta",
		`{"comments":[{"id":"a1","text":"Ana, 1/1: revisar","quote":"hola","anchored":true},{"id":"a2","parent_id":"a1","text":"Luis: hecho"}]}`, nil, "/comments/import"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("importar: HTTP %d %s", rec.Code, rec.Body)
	}
	importados, err := e.st.ListComments(e.ctx, e.doc.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	var importado string
	for _, c := range importados {
		if c.AuthorSub == "comenta" && c.Body == "Ana, 1/1: revisar" {
			importado = c.ID
		}
	}
	if importado == "" {
		t.Fatal("el comentario importado no quedó a nombre de quien importa")
	}
	if got := borrar("otra", importado); got != http.StatusForbidden {
		t.Errorf("otra persona borra el importado: HTTP %d", got)
	}
	if got := borrar("comenta", importado); got != http.StatusNoContent {
		t.Errorf("quien importó lo borra: HTTP %d", got)
	}

	// Ya borrado: 404, no 403 ni 204.
	if got := borrar("owner", importado); got != http.StatusNotFound {
		t.Errorf("borrar dos veces: HTTP %d", got)
	}
}
