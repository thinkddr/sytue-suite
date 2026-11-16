// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
)

func comentar(t *testing.T, claims *authn.Claims, ancla string) int {
	t.Helper()
	cuerpo := `{"body":"mira esto"`
	if ancla != "" {
		cuerpo += `,"anchor":` + ancla
	}
	cuerpo += `}`
	h := newHandler(&fakeStore{doc: ownedDoc()})
	rec := httptest.NewRecorder()
	h.createComment(rec, req(t, "POST", "/v1/documents/d1/comments", claims, map[string]string{"id": "d1"}, cuerpo))
	return rec.Code
}

// El ancla de un comentario sobre un dibujo de Docus es `{dibujo, nodo | x,y, text}`: se acepta
// bien formada y se rechaza todo lo demás ANTES de guardarla, porque se devuelve a cada lector.
func TestElAnclaDeUnDibujoSeValida(t *testing.T) {
	grande := `{"dibujo":"d1","text":"` + strings.Repeat("x", 5000) + `"}`
	casos := []struct {
		nombre, ancla string
		quiero        int
	}{
		{"sin ancla", ``, http.StatusCreated},
		{"nula", `null`, http.StatusCreated},
		{"de texto (marca)", `{"mark":"m1","text":"hola"}`, http.StatusCreated},
		{"a una forma", `{"dibujo":"dib-1","nodo":"n1a2b","text":"Rectángulo 1"}`, http.StatusCreated},
		{"a una coordenada", `{"dibujo":"dib-1","x":120.5,"y":-3,"text":"punto"}`, http.StatusCreated},
		{"no es un objeto", `"dib-1"`, http.StatusBadRequest},
		{"id con markup", `{"dibujo":"<img onerror=1>"}`, http.StatusBadRequest},
		{"dibujo no es texto", `{"dibujo":7}`, http.StatusBadRequest},
		{"nodo con markup", `{"dibujo":"d","nodo":"a\"b"}`, http.StatusBadRequest},
		{"nodo y punto a la vez", `{"dibujo":"d","nodo":"n1","x":1,"y":2}`, http.StatusBadRequest},
		{"punto a medias", `{"dibujo":"d","x":1}`, http.StatusBadRequest},
		{"coordenada absurda", `{"dibujo":"d","x":1e300,"y":0}`, http.StatusBadRequest},
		{"coordenada no numérica", `{"dibujo":"d","x":"1","y":0}`, http.StatusBadRequest},
		{"texto largo", `{"dibujo":"d","nodo":"n","text":"` + strings.Repeat("ñ", 301) + `"}`, http.StatusBadRequest},
		{"demasiado grande", grande, http.StatusBadRequest},
	}
	for _, c := range casos {
		if got := comentar(t, ownerClaims(), c.ancla); got != c.quiero {
			t.Errorf("%s: %d, quería %d", c.nombre, got, c.quiero)
		}
	}
}

// Comentar sobre un dibujo exige el mismo permiso que cualquier comentario: quien no tiene
// acceso al documento no lo consigue por llevar un ancla válida (el aislamiento por tenant
// lo cubre matriz_acl_test.go).
func TestElAnclaDeUnDibujoNoSaltaElPermiso(t *testing.T) {
	ajeno := &authn.Claims{Subject: "intruso", Tenant: "acme"}
	if got := comentar(t, ajeno, `{"dibujo":"dib-1","nodo":"n1"}`); got == http.StatusCreated {
		t.Fatalf("un usuario sin acceso comentó (%d)", got)
	}
}

// El ancla de un comentario de una celda de Calcus es `{hoja, fila, col, text}`
// (`sheets/anclaCelda.ts`): se acepta bien formada y se rechaza lo demás antes de guardarla.
func TestElAnclaDeUnaCeldaSeValida(t *testing.T) {
	casos := []struct {
		nombre, ancla string
		quiero        int
	}{
		{"a B4", `{"hoja":"9b2f-hoja","fila":"t3","col":"t1","text":"B4"}`, http.StatusCreated},
		{"fila insertada", `{"hoja":"h:Hoja 1","fila":"n1a2b3c4d","col":"t0","text":"A5"}`, http.StatusCreated},
		{"sin texto", `{"hoja":"h1","fila":"t0","col":"t0"}`, http.StatusCreated},
		{"hoja vacía", `{"hoja":"","fila":"t0","col":"t0"}`, http.StatusBadRequest},
		{"hoja no es texto", `{"hoja":3,"fila":"t0","col":"t0"}`, http.StatusBadRequest},
		{"sin fila", `{"hoja":"h1","col":"t0"}`, http.StatusBadRequest},
		{"columna con markup", `{"hoja":"h1","fila":"t0","col":"<b>"}`, http.StatusBadRequest},
		{"con marca a la vez", `{"hoja":"h1","fila":"t0","col":"t0","mark":"m1"}`, http.StatusBadRequest},
		{"texto largo", `{"hoja":"h1","fila":"t0","col":"t0","text":"` + strings.Repeat("ñ", 301) + `"}`, http.StatusBadRequest},
		{"hoja larga", `{"hoja":"` + strings.Repeat("h", 301) + `","fila":"t0","col":"t0"}`, http.StatusBadRequest},
	}
	for _, c := range casos {
		if got := comentar(t, ownerClaims(), c.ancla); got != c.quiero {
			t.Errorf("%s: %d, quería %d", c.nombre, got, c.quiero)
		}
	}
}
