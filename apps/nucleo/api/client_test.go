// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

type sesionFalsa struct {
	tokens      []string // el primero es el vigente; Invalidate pasa al siguiente
	invalidados []string
}

func (s *sesionFalsa) AccessToken(context.Context) (string, error) {
	if len(s.tokens) == 0 {
		return "", errors.New("hace falta iniciar sesión")
	}
	return s.tokens[0], nil
}

func (s *sesionFalsa) Invalidate(t string) {
	s.invalidados = append(s.invalidados, t)
	if len(s.tokens) > 0 && s.tokens[0] == t {
		s.tokens = s.tokens[1:]
	}
}

func TestUn401RenuevaYReintentaUnaVez(t *testing.T) {
	var vistos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistos = append(vistos, r.Header.Get("Authorization")+" "+string(cuerpo))
		if r.Header.Get("Authorization") != "Bearer bueno" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s := &sesionFalsa{tokens: []string{"caducado", "bueno"}}
	got, err := PostJSON[struct{ OK bool }](context.Background(), New(srv.URL, s, nil), "/x", map[string]int{"n": 1})
	if err != nil || !got.OK {
		t.Fatalf("got %+v, %v", got, err)
	}
	// El cuerpo tiene que viajar ENTERO también en el reintento.
	if len(vistos) != 2 || vistos[0] != `Bearer caducado {"n":1}` || vistos[1] != `Bearer bueno {"n":1}` {
		t.Fatalf("peticiones = %q", vistos)
	}

	// Dos 401 seguidos: no hay tercer intento y el error sale como 401.
	vistos, s.tokens = nil, []string{"malo1", "malo2", "bueno"}
	_, err = JSON[any](context.Background(), New(srv.URL, s, nil), Request{Path: "/x"})
	var e *Error
	if !errors.As(err, &e) || e.Status != http.StatusUnauthorized || len(vistos) != 2 {
		t.Fatalf("err = %v tras %d peticiones, quiero un 401 tras 2", err, len(vistos))
	}
}

func TestAnonimoNoPideToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("una petición anónima lleva Authorization")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	for _, c := range []*Client{New(srv.URL, nil, nil), New(srv.URL+"/", &sesionFalsa{}, nil)} {
		if _, err := JSON[any](context.Background(), c, Request{Path: "/x", Anonymous: true}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestErrorDeServicio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"no_active_organization","message":"…"}`))
	}))
	defer srv.Close()
	_, err := JSON[any](context.Background(), New(srv.URL, nil, nil), Request{Path: "/api/v1/files"})
	var e *Error
	if !errors.As(err, &e) || !e.NoActiveOrg() || e.URL != srv.URL+"/api/v1/files" {
		t.Fatalf("err = %#v", err)
	}
	if (&Error{Status: 403, Body: "<html>"}).NoActiveOrg() || (&Error{Status: 500, Body: `{"code":"no_active_organization"}`}).NoActiveOrg() {
		t.Error("NoActiveOrg salta sin ser un 403 con ese código")
	}
}

// Los servicios mandan el error DENTRO de su sobre (`httpx.Fail`), no suelto: leído
// solo suelto, `Code()` y `Message()` salían vacíos contra cualquier servicio real.
func TestElErrorSeLeeDentroDeSuSobre(t *testing.T) {
	e := &Error{Status: 422, Body: `{"error":{"code":"unprocessable","message":"total: required",` +
		`"details":{"fields":[{"field":"total","message":"required"}]}},"request_id":"r1"}`}
	if e.Code() != "unprocessable" || e.Message() != "total: required" {
		t.Errorf("Code = %q, Message = %q", e.Code(), e.Message())
	}
	if c := e.Campos(); len(c) != 1 || c["total"] != "required" {
		t.Errorf("Campos = %v", c)
	}
	if c := (&Error{Status: 500, Body: "<html>"}).Campos(); len(c) != 0 {
		t.Errorf("sin cuerpo JSON no hay campos: %v", c)
	}
}

func TestConsultaVaPorQueryYSeRepliegaAGet(t *testing.T) {
	var metodos []string
	entiendeQuery := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metodos = append(metodos, r.Method)
		filtro := r.URL.Query().Get("q")
		if r.Method == "QUERY" {
			if !entiendeQuery {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.URL.RawQuery != "" {
				t.Error("con QUERY los filtros siguen en la URL")
			}
			// ParseForm solo lee el cuerpo en POST, PUT y PATCH: con QUERY va a mano.
			cuerpo, _ := io.ReadAll(r.Body)
			forma, _ := url.ParseQuery(string(cuerpo))
			filtro = forma.Get("q")
		}
		_, _ = w.Write([]byte(`"` + filtro + `"`))
	}))
	defer srv.Close()
	ctx, pedir := context.Background(), Request{Path: "/buscar", Query: url.Values{"q": {"factura marzo"}}}

	c := New(srv.URL, nil, nil)
	if got, err := Consulta[string](ctx, c, pedir); err != nil || got != "factura marzo" {
		t.Fatalf("got %q, %v", got, err)
	}
	if len(metodos) != 1 || metodos[0] != "QUERY" {
		t.Fatalf("métodos = %v", metodos)
	}

	// Un servicio que no lo entiende: se repliega, y a la segunda ya ni lo intenta.
	entiendeQuery, metodos = false, nil
	c = New(srv.URL, nil, nil)
	for range 2 {
		if got, err := Consulta[string](ctx, c, pedir); err != nil || got != "factura marzo" {
			t.Fatalf("got %q, %v", got, err)
		}
	}
	if len(metodos) != 3 || metodos[0] != "QUERY" || metodos[1] != "GET" || metodos[2] != "GET" {
		t.Fatalf("métodos = %v, quiero [QUERY GET GET]", metodos)
	}
}

func TestURLs(t *testing.T) {
	c := New("https://rooms.sytue.eu/", nil, nil)
	if got := c.URL("/api/v1/x", url.Values{"a": {"1 2"}}); got != "https://rooms.sytue.eu/api/v1/x?a=1+2" {
		t.Error(got)
	}
	if got := c.WSURL("/rooms/ws", nil); got != "wss://rooms.sytue.eu/rooms/ws" {
		t.Error(got)
	}
	if got := New("http://localhost:8080", nil, nil).WSURL("/ws", nil); got != "ws://localhost:8080/ws" {
		t.Error(got)
	}
}

// sinSesion es quien no ha iniciado sesión: el llavero no tiene nada que dar.
type sinSesion struct{}

func (sinSesion) AccessToken(context.Context) (string, error) { return "", auth.ErrLoginRequired }
func (sinSesion) Invalidate(string)                           {}

// Sin sesión, lo que vuelve es el 401 que el servicio habría contestado, sin llegar a
// preguntarle, y las pantallas pintan «inicia sesión» y no «no se pudo cargar». Se ha
// visto en una ventana real: Calcus decía «Comprueba tu conexión» y Contacts, que la
// libreta estaba vacía.
func TestSinSesionEsUn401(t *testing.T) {
	llamadas := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { llamadas++ }))
	defer srv.Close()
	c := New(srv.URL, sinSesion{}, nil)
	_, err := JSON[map[string]any](context.Background(), c, Request{Path: "/cosas"})
	var e *Error
	if !errors.As(err, &e) || e.Status != http.StatusUnauthorized {
		t.Fatalf("sin sesión ha dado %v, quiero un 401", err)
	}
	if !errors.Is(err, auth.ErrLoginRequired) {
		t.Error("el 401 tiene que seguir diciendo que falta iniciar sesión")
	}
	if f := FaseDe(err, true, 0); f != FaseSinSesion {
		t.Errorf("FaseDe = %v, quiero FaseSinSesion", f)
	}
	if llamadas != 0 {
		t.Errorf("sin token no hay nada que preguntar al servicio, y se ha preguntado %d veces", llamadas)
	}
}

// TestUnaRespuestaSinFinNoAgotaLaMemoria (FYS-11): un JSON de más de maxJSON no se
// decodifica entero.
func TestUnaRespuestaSinFinNoAgotaLaMemoria(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`"`))
		trozo := bytes.Repeat([]byte("a"), 1<<20)
		for range maxJSON/len(trozo) + 2 {
			if _, err := w.Write(trozo); err != nil {
				return
			}
		}
		_, _ = w.Write([]byte(`"`))
	}))
	defer srv.Close()
	if _, err := JSON[string](context.Background(), New(srv.URL, nil, nil), Request{Path: "/x", Anonymous: true}); !errors.Is(err, ErrRespuestaGrande) {
		t.Fatalf("una respuesta de más de %d bytes: %v", maxJSON, err)
	}
}

// TestElClientePorDefectoTienePlazo (FYS-11): sin cliente propio, el transporte
// corta a quien no empieza a contestar.
func TestElClientePorDefectoTienePlazo(t *testing.T) {
	c := New("http://x", nil, nil)
	tr, ok := c.http.Transport.(*http.Transport)
	if !ok || tr.ResponseHeaderTimeout <= 0 {
		t.Fatalf("transporte sin plazo: %#v", c.http.Transport)
	}
}
