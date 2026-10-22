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

// TestNombresDeOrgs prueba POST /internal/orgs/names: el nombre visible de una
// organización activa, y nada de una suspendida, de una cuenta personal o de un
// slug que no existe; con su secreto, solo desde dentro y con tope.
func TestNombresDeOrgs(t *testing.T) {
	l := newLiveServer(t)
	prefix := "nombres-orgs-" + strings.ToLower(t.Name()) + "-"
	nueva := func(slug, nombre string, kind domain.OrgKind) domain.Organization {
		o, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: prefix + slug, Name: nombre, Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	activa := nueva("acme", "Acme Ingeniería S.L.", domain.OrgKindOrg)
	suspendida := nueva("parada", "Parada S.L.", domain.OrgKindOrg)
	personal := nueva("ana", "Ana Pérez", domain.OrgKindPersonal)
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE organizations SET status = 'suspended' WHERE id = $1`, suspendida.ID); err != nil {
		t.Fatal(err)
	}

	const secreto = "s-nombres-orgs-test"
	puerta := httpx.SoloDesdeDentro(secreto)(http.HandlerFunc(l.srv.handleInternalOrgsNames))
	pedir := func(cuerpo, clave string, cabeceras ...string) (int, map[string]string) {
		r := httptest.NewRequest(http.MethodPost, "/internal/orgs/names", strings.NewReader(cuerpo))
		r.Header.Set("Authorization", "Bearer "+clave)
		r.Header.Set("Content-Type", "application/json")
		for i := 0; i+1 < len(cabeceras); i += 2 {
			r.Header.Set(cabeceras[i], cabeceras[i+1])
		}
		w := httptest.NewRecorder()
		puerta.ServeHTTP(w, r)
		var resp struct {
			Orgs []struct{ Slug, Name string } `json:"orgs"`
		}
		_ = json.NewDecoder(w.Body).Decode(&resp)
		out := map[string]string{}
		for _, o := range resp.Orgs {
			out[o.Slug] = o.Name
		}
		return w.Code, out
	}

	cuerpo := `{"slugs":["` + activa.Slug + `","` + suspendida.Slug + `","` + personal.Slug + `","no-existe-xyz","` + activa.Slug + `"]}`
	code, nombres := pedir(cuerpo, secreto)
	if code != http.StatusOK || len(nombres) != 1 || nombres[activa.Slug] != "Acme Ingeniería S.L." {
		t.Fatalf("solo la activa, con su nombre: %d %v", code, nombres)
	}
	if code, _ := pedir(cuerpo, "otro"); code != http.StatusUnauthorized {
		t.Errorf("con otro secreto: %d", code)
	}
	if code, _ := pedir(cuerpo, secreto, "X-Forwarded-For", "203.0.113.9"); code != http.StatusNotFound {
		t.Errorf("desde internet: %d", code)
	}
	muchos := `{"slugs":[` + strings.TrimSuffix(strings.Repeat(`"x",`, maxNombresOrgs+1), ",") + `]}`
	if code, _ := pedir(muchos, secreto); code != http.StatusBadRequest {
		t.Errorf("más de %d: %d", maxNombresOrgs, code)
	}
}
