// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// Una redirección tiene que seguir siendo https y pública: es el rodeo clásico de
// una barrera que solo mira la dirección de entrada.
func TestLasRedireccionesDeUnAlmacenDeTercerosSeRevalidan(t *testing.T) {
	c := clienteDeTerceros(0)
	salto := func(dir string) error {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, dir, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		return c.CheckRedirect(req, nil)
	}
	if err := salto("http://example.com/"); !errors.Is(err, ErrDestinoNoPermitido) {
		t.Errorf("redirección a http: %v", err)
	}
	for _, dentro := range []string{"https://10.0.0.5/", "https://127.0.0.1:8081/", "https://169.254.169.254/", "https://[::1]/"} {
		if err := salto(dentro); !errors.Is(err, ssrfhttp.ErrBlocked) {
			t.Errorf("redirección a %s: %v, esperaba que la cortara la barrera SSRF", dentro, err)
		}
	}
	muchos := make([]*http.Request, maxSaltos)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/", http.NoBody)
	if err := c.CheckRedirect(req, muchos); !errors.Is(err, ErrDestinoNoPermitido) {
		t.Errorf("la sexta redirección: %v", err)
	}
}
