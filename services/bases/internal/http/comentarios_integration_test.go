// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package baseshttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
)

// pega son el Hub y el Sytue ID de mentira: el Hub apunta los avisos que recibe y el
// directorio resuelve `<quien>@acme.es` ↔ `<quien>` SOLO a sesiones de acme.
type pega struct {
	mu     sync.Mutex
	avisos []hubnotify.Notification
	// consultas cuenta las preguntas al directorio: quien no puede, no llega a hacerlas.
	consultas int
}

func (p *pega) montar(t *testing.T) func(*Deps) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/notifications", func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		if r.Header.Get("Authorization") != "Bearer secreto" || json.NewDecoder(r.Body).Decode(&n) != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		p.mu.Lock()
		p.avisos = append(p.avisos, n)
		p.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/v1/directory:resolve", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			UserID string `json:"user_id"`
			Email  string `json:"email"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		p.mu.Lock()
		p.consultas++
		p.mu.Unlock()
		// gus (globex) no resuelve a nadie de acme: la verja del directorio es la organización.
		if strings.HasSuffix(r.Header.Get("Authorization"), "gus") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		quien := in.UserID
		if in.Email != "" {
			var ok bool
			if quien, ok = strings.CutSuffix(in.Email, "@acme.es"); !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}
		// Solo la gente de acme: gus y hal son de globex y el directorio de acme no los conoce.
		if !slices.Contains([]string{"ana", "bea", "cleo", "dani"}, quien) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"user_id": quien, "email": quien + "@acme.es", "name": "N. " + quien})
	})
	// people:resolve (ADR 0040): cualquiera de cualquier organización, por @usuario o
	// dirección exactos; da el identificador y el nombre, nunca el correo ni la organización.
	mux.HandleFunc("/api/v1/people:resolve", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		p.mu.Lock()
		p.consultas++
		p.mu.Unlock()
		quien, _, _ := strings.Cut(strings.TrimPrefix(in.Query, "@"), "@")
		if !slices.Contains([]string{"ana", "bea", "gus", "hal"}, quien) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"user_id": quien, "name": "N. " + quien, "username": quien})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return func(d *Deps) {
		d.Hub = hubnotify.New(srv.URL, "secreto")
		d.Cfg.IDBaseURL = srv.URL
		d.Cfg.PublicURL = "https://bases.sytue.eu"
	}
}

// tomar deja que lleguen los avisos (van en segundo plano), los devuelve y los olvida.
func (p *pega) tomar(t *testing.T, cuantos int) []hubnotify.Notification {
	t.Helper()
	limite := time.Now().Add(3 * time.Second)
	for {
		p.mu.Lock()
		n := len(p.avisos)
		p.mu.Unlock()
		if n >= cuantos || time.Now().After(limite) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Un margen para que un aviso DE MÁS (el fallo que se busca) llegue a verse.
	time.Sleep(150 * time.Millisecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.avisos
	p.avisos = nil
	return out
}

// esperar es tomar como «tipo→destinatario», ordenados.
func (p *pega) esperar(t *testing.T, cuantos int) []string {
	t.Helper()
	as := p.tomar(t, cuantos)
	out := make([]string, 0, len(as))
	for _, n := range as {
		out = append(out, n.Type+"→"+n.Subject)
	}
	slices.Sort(out)
	return out
}

func TestComentariosPorHTTP(t *testing.T) {
	p := &pega{}
	srv := servidorCon(t, p.montar(t))
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Clientes"}`))
	tabla := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"Contactos"}`))
	reg := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/records", `{"data":{}}`))
	ruta := "/records/" + reg + "/comments"
	conceder := func(quien, rol string) {
		t.Helper()
		if c := llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/permissions", `{"principal_kind":"user","principal_id":"`+quien+`","role":"`+rol+`"}`).Code; c != 201 {
			t.Fatalf("conceder %s a %s: %d", rol, quien, c)
		}
	}

	// Sin acceso y otra organización: 404, también con el id de un comentario real.
	raiz := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", ruta, `{"body":"primero"}`))
	for _, q := range [][2]string{{"bea", "acme"}, {"gus", "globex"}} {
		if c := llamar(t, srv, q[0], q[1], "GET", ruta, "").Code; c != 404 {
			t.Errorf("%s lista sin acceso: %d", q[0], c)
		}
		if c := llamar(t, srv, q[0], q[1], "POST", ruta, `{"body":"x"}`).Code; c != 404 {
			t.Errorf("%s comenta sin acceso: %d", q[0], c)
		}
		if c := llamar(t, srv, q[0], q[1], "DELETE", "/comments/"+raiz, "").Code; c != 404 {
			t.Errorf("%s borra sin acceso: %d", q[0], c)
		}
	}

	// Lector: lee (con el nombre del autor), no comenta ni borra. Comentarista: comenta.
	conceder("bea", "viewer")
	w := llamar(t, srv, "bea", "acme", "GET", ruta, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"author_name":"N. ana"`) || !strings.Contains(w.Body.String(), `"can_delete":false`) {
		t.Errorf("lector lista: %d %s", w.Code, w.Body)
	}
	if c := llamar(t, srv, "bea", "acme", "POST", ruta, `{"body":"x"}`).Code; c != 403 {
		t.Errorf("lector comenta: %d", c)
	}
	if c := llamar(t, srv, "bea", "acme", "DELETE", "/comments/"+raiz, "").Code; c != 403 {
		t.Errorf("lector borra: %d", c)
	}
	conceder("bea", "commenter")
	p.esperar(t, 0)

	// Entradas hostiles.
	for cuerpo, quiero := range map[string]int{
		`{"body":""}`: 422,
		`{"body":"` + strings.Repeat("a", 2001) + `"}`: 422,
		`{"body":"x","parent_id":"no-es-uuid"}`:        422,
		`{"body":"x","parent_id":"` + reg + `"}`:       422, // un id real que no es un comentario
		`{"body":"x","record_id":"` + reg + `"}`:       400, // campo desconocido
	} {
		if c := llamar(t, srv, "bea", "acme", "POST", ruta, cuerpo).Code; c != quiero && (quiero != 400 || c != 422) {
			t.Errorf("cuerpo %.40s: %d, esperado %d", cuerpo, c, quiero)
		}
	}
	if c := llamar(t, srv, "bea", "acme", "GET", "/records/basura/comments", "").Code; c != 404 {
		t.Errorf("id basura: %d", c)
	}

	// Avisos. bea responde y menciona a cleo (sin acceso), a dani (con acceso) y a alguien
	// de fuera: la propietaria recibe el comentario, dani la mención, cleo y el de fuera NADA.
	conceder("dani", "viewer")
	w = llamar(t, srv, "bea", "acme", "POST", ruta, `{"parent_id":"`+raiz+`","body":"mira @cleo@acme.es y @dani@acme.es y @eva@otra.es"}`)
	if w.Code != 201 {
		t.Fatalf("bea responde: %d %s", w.Code, w.Body)
	}
	deBea := idDeRespuesta(t, w)
	if got := p.esperar(t, 2); !slices.Equal(got, []string{"bases.comment→ana", "bases.mention→dani"}) {
		t.Errorf("avisos de la respuesta de bea: %v", got)
	}
	// Mencionarse a una misma no avisa; mencionar a la propietaria le da la mención, no las dos.
	llamar(t, srv, "bea", "acme", "POST", ruta, `{"body":"@bea@acme.es y @ana@acme.es"}`)
	if got := p.esperar(t, 1); !slices.Equal(got, []string{"bases.mention→ana"}) {
		t.Errorf("avisos de la mención a la propietaria: %v", got)
	}

	// Repetir la mención no repite el aviso: quien comenta no puede llenarle la campana a otra persona.
	llamar(t, srv, "bea", "acme", "POST", ruta, `{"body":"otra vez @dani@acme.es"}`)
	if got := p.esperar(t, 1); !slices.Equal(got, []string{"bases.comment→ana"}) {
		t.Errorf("avisos de la mención repetida: %v", got)
	}

	// El comentarista borra lo suyo y no lo ajeno; la propietaria, cualquiera.
	if c := llamar(t, srv, "bea", "acme", "DELETE", "/comments/"+raiz, "").Code; c != 403 {
		t.Errorf("comentarista borra lo ajeno: %d", c)
	}
	if c := llamar(t, srv, "bea", "acme", "DELETE", "/comments/"+deBea, "").Code; c != 204 {
		t.Errorf("comentarista borra lo suyo: %d", c)
	}
	if c := llamar(t, srv, "ana", "acme", "DELETE", "/comments/"+raiz, "").Code; c != 204 {
		t.Errorf("la propietaria borra: %d", c)
	}
}

// El enlace del aviso lleva a la base, en la organización que la aloja, con la tabla y
// el registro; y el cuerpo va recortado.
func TestAvisoDeComentarioLlevaElEnlaceDelRegistro(t *testing.T) {
	p := &pega{}
	srv := servidorCon(t, p.montar(t))
	base := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases", `{"name":"Clientes"}`))
	tabla := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/tables", `{"name":"T"}`))
	reg := idDeRespuesta(t, llamar(t, srv, "ana", "acme", "POST", "/tables/"+tabla+"/records", `{"data":{}}`))
	llamar(t, srv, "ana", "acme", "POST", "/bases/"+base+"/permissions", `{"principal_kind":"user","principal_id":"bea","role":"commenter"}`)
	llamar(t, srv, "bea", "acme", "POST", "/records/"+reg+"/comments", `{"body":"`+strings.Repeat("ñ", 500)+`"}`)
	as := p.tomar(t, 1)
	if len(as) != 1 {
		t.Fatalf("avisos: %+v", as)
	}
	a := as[0]
	quiero := "https://bases.sytue.eu/bases/acme/base/" + base + "?record=" + reg + "&table=" + tabla
	if a.URL != quiero || a.Tenant != "acme" || a.Source != "bases" || a.Subject != "ana" {
		t.Errorf("aviso: %+v\nenlace esperado %s", a, quiero)
	}
	if a.Params["name"] != "Clientes" || a.Params["actor"] != "bea" || len([]rune(a.Body)) > 300 {
		t.Errorf("parámetros o cuerpo del aviso: %+v", a)
	}
}
