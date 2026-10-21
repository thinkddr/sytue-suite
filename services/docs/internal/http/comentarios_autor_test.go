// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// El autor de un comentario sale con su nombre del directorio, no con el principio de
// su `sub` (medido en producción: «52135c44»). Lo resuelve el servicio con la sesión de
// quien lee, así que la web y Fyne lo reciben igual. Quien no se resuelve, sin nombre.
func TestLosComentariosLlevanElNombreDelAutor(t *testing.T) {
	pedidos := 0
	id := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidos++
		var q struct {
			UserID string `json:"user_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if r.URL.Path != "/api/v1/directory:resolve" || r.Header.Get("Authorization") != "Bearer tk" || q.UserID != "52135c44-ana" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"user_id":"52135c44-ana","email":"ana@acme.test","name":"Ana Pérez"}`))
	}))
	defer id.Close()
	h := newHandler(&fakeStore{doc: ownedDoc(), comments: []domain.Comment{
		{ID: "c1", AuthorSub: "52135c44-ana"}, {ID: "c2", AuthorSub: "52135c44-ana"}, {ID: "c3", AuthorSub: "de-fuera"},
	}})
	h.idBaseURL = id.URL

	rq := req(t, "GET", "/v1/documents/d1/comments", ownerClaims(), map[string]string{"id": "d1"}, "")
	rq.Header.Set("Authorization", "Bearer tk")
	rec := httptest.NewRecorder()
	h.listComments(rec, rq)
	var out struct {
		Comments []domain.Comment `json:"comments"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil || len(out.Comments) != 3 {
		t.Fatalf("listado: %d %v", rec.Code, err)
	}
	if out.Comments[0].AuthorName != "Ana Pérez" || out.Comments[1].AuthorName != "Ana Pérez" || out.Comments[2].AuthorName != "" {
		t.Errorf("nombres: %q %q %q", out.Comments[0].AuthorName, out.Comments[1].AuthorName, out.Comments[2].AuthorName)
	}
	if pedidos != 2 {
		t.Errorf("%d peticiones al directorio; una por autor distinto son 2", pedidos)
	}
}

// Los comentarios de un fichero importado los guarda quien importa, raíces antes que
// respuestas, y el servicio dice cuáles con ancla quedaron guardados (el cliente quita del
// HTML los marks de los demás).
func TestImportarComentariosGuardaRaicesYRespuestasYDiceLosAnclados(t *testing.T) {
	h := newHandler(&fakeStore{doc: ownedDoc()})
	cuerpo := `{"comments":[
		{"id":"bb","parent_id":"aa","text":"Marta, 2026-09-30: Hecho","resolved":true},
		{"id":"aa","text":"Lucía, 2026-09-30: Revisar","quote":"1200","anchored":true},
		{"id":"cc","text":"Ana: suelto","anchored":false},
		{"id":"<x>","text":"mal id","anchored":true},
		{"id":"dd","parent_id":"nadie","text":"huérfana"}]}`
	rq := req(t, "POST", "/v1/documents/d1/comments/import", ownerClaims(), map[string]string{"id": "d1"}, cuerpo)
	rec := httptest.NewRecorder()
	h.importComments(rec, rq)
	var out struct {
		Saved    int      `json:"saved"`
		Anchored []string `json:"anchored"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("respuesta: %d %v", rec.Code, err)
	}
	if out.Saved != 3 || len(out.Anchored) != 1 || out.Anchored[0] != "aa" {
		t.Errorf("guardados %d, anclados %v; quería 3 y [aa]", out.Saved, out.Anchored)
	}
}
