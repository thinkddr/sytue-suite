// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestERPLibroYBalancePorPeriodo: el período va como `desde`/`hasta`, igual que en la
// web (`rangoDePeriodo`), y sin período no se manda nada.
//
// VISTO EN ROJO: sin `periodo(q, …)` en BalanceEntre, la segunda petición de balance
// sale sin consulta.
func TestERPLibroYBalancePorPeriodo(t *testing.T) {
	var mu sync.Mutex
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		vistas = append(vistas, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		if r.URL.Path == "/api/v1/balance" {
			_, _ = io.WriteString(w, `{"cuentas":[],"total_debe":0,"total_haber":0}`)
			return
		}
		_, _ = io.WriteString(w, `{"asientos":[]}`)
	}))
	defer srv.Close()
	c, ctx := NewERP(srv.URL, nil, nil, "es"), context.Background()
	_, _ = c.Libro(ctx, 50)
	_, _ = c.LibroEntre(ctx, 50, "2026-11-01", "2026-11-30")
	_, _ = c.Balance(ctx)
	_, _ = c.BalanceEntre(ctx, "2026-10-01", "2026-12-31")
	quiero := []string{
		"/api/v1/libro?limite=50",
		"/api/v1/libro?desde=2026-11-01&hasta=2026-11-30&limite=50",
		"/api/v1/balance?",
		"/api/v1/balance?desde=2026-10-01&hasta=2026-12-31",
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(vistas, "\n") != strings.Join(quiero, "\n") {
		t.Errorf("peticiones:\n%s\nquiero:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
	// Y el servicio lee esos dos nombres en las dos rutas.
	codigo := webref.Read(t, "services/syerp/internal/http/handlers.go")
	for _, l := range []string{`q.Get("desde")`, `q.Get("hasta")`, `h.store.BalanceEntre(r.Context(), desde, hasta)`} {
		if !strings.Contains(codigo, l) {
			t.Errorf("handlers.go ya no dice %s", l)
		}
	}
}
