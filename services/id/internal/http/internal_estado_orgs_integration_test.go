// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestEstadoDeOrgs prueba POST /internal/orgs/status: estados activo, suspendido,
// fusionado y desconocido, secreto correcto e incorrecto, y tope de 500 slugs.
func TestEstadoDeOrgs(t *testing.T) {
	l := newLiveServer(t)
	nueva := func(slug string) domain.Organization {
		o, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: slug, Name: slug})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}

	// Los slugs de organización solo admiten minúsculas; el nombre Go del test
	// contiene mayúsculas y no debe enviarse literalmente a PostgreSQL.
	prefix := "estado-orgs-" + strings.ToLower(t.Name()) + "-"
	orgActiva := nueva(prefix + "activa")
	orgSuspendida := nueva(prefix + "suspendida")
	orgFusionada := nueva(prefix + "fusionada")

	// Poner org suspendida.
	if _, err := l.st.DB().Exec(l.ctx,
		`UPDATE organizations SET status = 'suspended' WHERE id = $1`, orgSuspendida.ID); err != nil {
		t.Fatal(err)
	}
	// Poner org fusionada (estado de BD; la otra org sigue activa).
	if _, err := l.st.DB().Exec(l.ctx,
		`UPDATE organizations SET status = 'merged', merged_into_org_id = $2, merged_at = now() WHERE id = $1`,
		orgFusionada.ID, orgActiva.ID); err != nil {
		t.Fatal(err)
	}

	const secreto = "s-estado-orgs-test"
	puerta := httpx.SoloDesdeDentro(secreto)(http.HandlerFunc(l.srv.handleInternalOrgsStatus))

	type orgEstado struct {
		Slug   string `json:"slug"`
		Status string `json:"status"`
	}
	pedir := func(body string, extraHdrs ...string) (int, map[string]string) {
		r := httptest.NewRequest(http.MethodPost, "/internal/orgs/status", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+secreto)
		r.Header.Set("Content-Type", "application/json")
		for i := 0; i+1 < len(extraHdrs); i += 2 {
			r.Header.Set(extraHdrs[i], extraHdrs[i+1])
		}
		w := httptest.NewRecorder()
		puerta.ServeHTTP(w, r)
		var resp struct {
			Orgs []orgEstado `json:"orgs"`
		}
		_ = json.NewDecoder(w.Body).Decode(&resp)
		out := make(map[string]string, len(resp.Orgs))
		for _, o := range resp.Orgs {
			out[o.Slug] = o.Status
		}
		return w.Code, out
	}

	t.Run("estados_correctos", func(t *testing.T) {
		slugs := `{"slugs":["` + orgActiva.Slug + `","` + orgSuspendida.Slug + `","` + orgFusionada.Slug + `","slug-inexistente-xyz"]}`
		code, estados := pedir(slugs)
		if code != http.StatusOK {
			t.Fatalf("código: %d", code)
		}
		if got := estados[orgActiva.Slug]; got != "active" {
			t.Errorf("activa: %q", got)
		}
		if got := estados[orgSuspendida.Slug]; got != "suspended" {
			t.Errorf("suspendida: %q", got)
		}
		if got := estados[orgFusionada.Slug]; got != "merged" {
			t.Errorf("fusionada: %q", got)
		}
		if got := estados["slug-inexistente-xyz"]; got != "deleted" {
			t.Errorf("desconocida: %q", got)
		}
	})

	t.Run("secreto_incorrecto_401", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/internal/orgs/status",
			strings.NewReader(`{"slugs":["x"]}`))
		r.Header.Set("Authorization", "Bearer otro-secreto")
		w := httptest.NewRecorder()
		puerta.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("con otro secreto: %d", w.Code)
		}
	})

	t.Run("desde_internet_404", func(t *testing.T) {
		code, _ := pedir(`{"slugs":["x"]}`, "X-Forwarded-For", "203.0.113.9")
		if code != http.StatusNotFound {
			t.Errorf("desde internet: %d", code)
		}
	})

	t.Run("demasiados_slugs_400", func(t *testing.T) {
		slugSlice := make([]string, 501)
		for i := range slugSlice {
			slugSlice[i] = `"slug-x"`
		}
		body := `{"slugs":[` + strings.Join(slugSlice, ",") + `]}`
		code, _ := pedir(body)
		if code != http.StatusBadRequest {
			t.Errorf("demasiados slugs: %d", code)
		}
	})

	t.Run("slugs_vacios_ok", func(t *testing.T) {
		code, estados := pedir(`{"slugs":[]}`)
		if code != http.StatusOK || len(estados) != 0 {
			t.Errorf("vacio: %d %v", code, estados)
		}
	})
}
