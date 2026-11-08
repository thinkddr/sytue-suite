// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// El lote lee el JSON aunque venga vallado; sin descripción no hay nada que guardar.
func TestLeerDescripcion(t *testing.T) {
	d, e, ok := leerDescripcion("Claro:\n```json\n{\"descripcion\":\" Una playa \",\"etiquetas\":[\"playa\",\"mar\"]}\n```")
	if !ok || d != "Una playa" || len(e) != 2 {
		t.Fatalf("d=%q e=%v ok=%v", d, e, ok)
	}
	for _, malo := range []string{"", "sin json", `{"descripcion":""}`, `{roto`} {
		if _, _, ok := leerDescripcion(malo); ok {
			t.Errorf("%q no es una descripción", malo)
		}
	}
}

// El idioma de la dueña va nombrado en la instrucción y en Accept-Language; sin
// idioma conocido no se fija ninguno.
func TestDescribirEnElIdiomaDeLaDuenya(t *testing.T) {
	if s := instruccionDescribir("es"); !strings.Contains(s, "every tag in Spanish") {
		t.Fatalf("es: %s", s)
	}
	if s := instruccionDescribir(""); strings.Contains(s, "every tag in") {
		t.Fatalf("sin idioma no se inventa uno: %s", s)
	}
	var visto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visto = r.Header.Get("Accept-Language")
		_, _ = w.Write([]byte(`{"text":"{}"}`))
	}))
	defer srv.Close()
	c := &clienteIA{base: srv.URL, token: "t", hc: &http.Client{Timeout: 5 * time.Second}}
	if _, err := c.describir(context.Background(), []byte("jpg"), "it"); err != nil || visto != "it" {
		t.Fatalf("Accept-Language=%q err=%v", visto, err)
	}
	if _, err := c.describir(context.Background(), []byte("jpg"), "x\nmal"); err != nil || visto != "" {
		t.Fatalf("un idioma desconocido no viaja: %q err=%v", visto, err)
	}
}
