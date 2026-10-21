// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package log

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// El contrato de cable: POST, bearer, JSON y la forma que el receptor de Sytue
// ID decodifica. Si alguien cambia una etiqueta de Entry, los lotes siguen
// saliendo y el receptor los guarda VACÍOS, sin que falle nada en ningún lado.
func TestHTTPSinkWrite(t *testing.T) {
	var visto struct {
		metodo, auth, tipo string
		lote               []Entry
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visto.metodo = r.Method
		visto.auth = r.Header.Get("Authorization")
		visto.tipo = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&visto.lote)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	s := &httpSink{url: srv.URL, token: "secreto", c: srv.Client()}
	cuando := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	err := s.Write(context.Background(), []Entry{{
		At: cuando, Level: "ERROR", Service: "drive", Message: "algo falló",
		Attrs: map[string]string{"error": "boom"},
	}})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if visto.metodo != http.MethodPost || visto.auth != "Bearer secreto" || visto.tipo != "application/json" {
		t.Fatalf("petición=%+v", visto)
	}
	if len(visto.lote) != 1 {
		t.Fatalf("entradas=%d; quería 1", len(visto.lote))
	}
	e := visto.lote[0]
	if !e.At.Equal(cuando) || e.Level != "ERROR" || e.Service != "drive" ||
		e.Message != "algo falló" || e.Attrs["error"] != "boom" {
		t.Fatalf("entrada=%+v", e)
	}
}

// Un 5xx del receptor tiene que devolver error: el capturador lo cuenta como
// lote perdido en vez de darlo por guardado.
func TestHTTPSinkWriteErrorDelReceptor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	s := &httpSink{url: srv.URL, token: "malo", c: srv.Client()}
	if err := s.Write(context.Background(), []Entry{{Level: "ERROR", Service: "drive"}}); err == nil {
		t.Fatal("quería error con 401 del receptor")
	}
}

// El lazo entero, que es lo que de verdad se rompe: New() tiene que enganchar
// el recolector SOLO con las variables del entorno. Sin este test, quitar ese
// bloque de New() no falla nada —los servicios siguen arrancando y sirviendo—
// y la consola de plataforma vuelve a enseñar solo lo del IdP, en silencio.
func TestNewEnganchaElRecolector(t *testing.T) {
	llegada := make(chan []Entry, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var lote []Entry
		_ = json.NewDecoder(r.Body).Decode(&lote)
		select {
		case llegada <- lote:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv(CollectorURLEnv, srv.URL)
	t.Setenv(CollectorTokenEnv, "secreto")
	New(Options{Level: "info", Service: "drive"}).Error("algo falló", "error", "boom")

	select {
	case lote := <-llegada:
		if len(lote) != 1 || lote[0].Service != "drive" || lote[0].Message != "algo falló" {
			t.Fatalf("lote=%+v", lote)
		}
	case <-time.After(10 * time.Second): // el volcado periódico es de 2 s
		t.Fatal("el recolector no recibió nada: New() no enganchó el sumidero")
	}
}

// Sin las DOS variables no se engancha nada: media configuración es un error
// cada dos segundos contra un destino que no autentica.
func TestCollectorDesdeEntorno(t *testing.T) {
	for _, c := range []struct{ url, tok string }{
		{"", ""}, {"http://id/service-logs", ""}, {"", "secreto"}, {"  ", "  "},
	} {
		t.Setenv(CollectorURLEnv, c.url)
		t.Setenv(CollectorTokenEnv, c.tok)
		if s := collectorDesdeEntorno(); s != nil {
			t.Fatalf("url=%q token=%q: quería nil", c.url, c.tok)
		}
	}
	t.Setenv(CollectorURLEnv, "http://id/service-logs")
	t.Setenv(CollectorTokenEnv, "secreto")
	if collectorDesdeEntorno() == nil {
		t.Fatal("con URL y token quería sumidero")
	}
}
