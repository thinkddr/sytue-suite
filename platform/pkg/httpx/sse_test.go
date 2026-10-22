// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Un flujo SSE sobrevive a los plazos del servidor. Es la campana de la suite: con el
// WriteTimeout de NewServer, un aviso que llegaba pasado el plazo se escribía en una
// conexión muerta y no lo veía nadie (ver AbrirSSE). Los plazos aquí son de décimas
// para que el test no espere 30 s, y la petición pasa por RequestLogger porque es el
// envoltorio que llevan todos los servicios: si deja de desenvolverse, el
// ResponseController no llega a la conexión y esto se pone en rojo.
func TestElFlujoSSESobreviveALosPlazosDelServidor(t *testing.T) {
	const plazo = 150 * time.Millisecond
	tarde := make(chan struct{})
	h := RequestLogger(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := AbrirSSE(w)
		if !ok {
			t.Error("el writer del servidor debería vaciar su búfer")
			return
		}
		_, _ = fmt.Fprint(w, "event: ready\ndata: {}\n\n")
		f.Flush()
		select {
		case <-r.Context().Done():
			t.Error("el contexto se canceló con el flujo abierto: el plazo de lectura sigue puesto")
			return
		case <-tarde:
		}
		_, _ = fmt.Fprint(w, "event: notification\ndata: {\"id\":\"x\"}\n\n")
		f.Flush()
	}))
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ReadTimeout, srv.Config.WriteTimeout = plazo, plazo
	srv.Start()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}
	lector := bufio.NewReader(res.Body)
	leer := func() string {
		linea, err := lector.ReadString('\n')
		if err != nil {
			t.Fatalf("el flujo se cortó: %v", err)
		}
		return strings.TrimSpace(linea)
	}
	if l := leer(); l != "event: ready" {
		t.Fatalf("primera línea = %q", l)
	}
	time.Sleep(3 * plazo) // los dos plazos, vencidos de sobra
	close(tarde)
	for l := leer(); l != "event: notification"; l = leer() {
		if l != "" && !strings.HasPrefix(l, "data:") {
			t.Fatalf("línea inesperada %q", l)
		}
	}
}

// Sin Flusher no se escribe nada: quien llama aún puede contestar su error.
func TestAbrirSSESinFlusherNoEscribe(t *testing.T) {
	var w sinFlush
	if _, ok := AbrirSSE(&w); ok || w.escrito {
		t.Fatalf("ok=%v escrito=%v", ok, w.escrito)
	}
}

type sinFlush struct{ escrito bool }

func (s *sinFlush) Header() http.Header       { return http.Header{} }
func (s *sinFlush) Write([]byte) (int, error) { s.escrito = true; return 0, nil }
func (s *sinFlush) WriteHeader(int)           { s.escrito = true }
