// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// El techo de caducidad de los enlaces, por la API directa (sin interfaz ni BFF que lo
// impongan): el servicio pone la caducidad, rechaza la que se pasa del techo, y «sin
// caducidad» solo existe si quien administra la organización quitó el techo.
func TestTechoDeCaducidadPorLaAPI(t *testing.T) {
	e := nuevoEscenarioPermisos(t, domain.KindDocument)
	e.hd.politicaEnlaces = e.st

	type enlace struct {
		ID        string     `json:"id"`
		Token     string     `json:"token"`
		ExpiresAt *time.Time `json:"expires_at"`
		Ajustado  *time.Time `json:"policy_adjusted_at"`
	}
	crear := func(cuerpo string) (int, enlace, string) {
		t.Helper()
		rec := httptest.NewRecorder()
		e.hd.createShare(rec, e.peticion("POST", "owner", cuerpo, nil, "/shares"))
		var out struct {
			Share enlace `json:"share"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out.Share, out.Error.Code
	}
	politica := func(metodo, sub string, roles []string, cuerpo string) (int, map[string]any) {
		t.Helper()
		r := req(t, metodo, "/v1/link-policy", &authn.Claims{Subject: sub, Tenant: "acme", Roles: roles}, nil, cuerpo)
		r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
		rec := httptest.NewRecorder()
		if metodo == "GET" {
			e.hd.verPoliticaEnlaces(rec, r)
		} else {
			e.hd.guardarPoliticaEnlaces(rec, r)
		}
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	resuelve := func(token string) int {
		rec := httptest.NewRecorder()
		e.hd.resolveShare(rec, req(t, "GET", "/v1/share/"+token, nil, map[string]string{"token": token}, ""))
		return rec.Code
	}
	enDias := func(n int) string {
		return `{"role":"viewer","expires_at":"` + time.Now().Add(time.Duration(n)*24*time.Hour).UTC().Format(time.RFC3339) + `"}`
	}
	admin := []string{"admin"}

	// Sin decir nada: el techo por defecto, y lo ve cualquiera de la organización.
	if c, p := politica("GET", "lector", nil, ""); c != http.StatusOK || p["max_days"] != float64(domain.TechoEnlacesPorDefecto) || p["is_default"] != true || p["can_manage"] != false {
		t.Fatalf("política por defecto: %d %v", c, p)
	}

	// Crear sin fecha NO da un enlace eterno: caduca en el techo.
	c, l, _ := crear(`{"role":"viewer"}`)
	if c != http.StatusCreated || l.ExpiresAt == nil || l.ExpiresAt.Sub(time.Now().Add(365*24*time.Hour)).Abs() > 2*time.Minute {
		t.Fatalf("crear sin fecha: %d, caduca %v", c, l.ExpiresAt)
	}
	if resuelve(l.Token) != http.StatusOK {
		t.Fatal("el enlace recién creado no se resuelve")
	}
	// Pedir más que el techo, o una fecha pasada: 400, y no se crea nada.
	if c, _, code := crear(enDias(366)); c != http.StatusBadRequest || code != "share_expiry_too_far" {
		t.Fatalf("más allá del techo: %d %s", c, code)
	}
	if c, _, code := crear(enDias(-1)); c != http.StatusBadRequest || code != "share_expiry_past" {
		t.Fatalf("fecha pasada: %d %s", c, code)
	}
	if ls, _ := e.st.ListShareLinks(e.ctx, e.doc.ID); len(ls) != 1 {
		t.Fatalf("un intento rechazado dejó un enlace: %d", len(ls))
	}
	if c, l, _ := crear(enDias(30)); c != http.StatusCreated || l.ExpiresAt == nil {
		t.Fatalf("dentro del techo: %d", c)
	}

	// Cambiar el techo es de quien administra la ORGANIZACIÓN: ni el propietario del
	// documento ni un editor, y un rol cualquiera tampoco.
	for _, quien := range []struct {
		sub   string
		roles []string
	}{{"owner", nil}, {"ana", nil}, {"owner", []string{"member"}}, {"owner", []string{"billing"}}} {
		if c, _ := politica("PUT", quien.sub, quien.roles, `{"max_days":0}`); c != http.StatusForbidden {
			t.Fatalf("%s %v cambió la política: %d", quien.sub, quien.roles, c)
		}
	}
	if c, _, _ := crear(`{"role":"viewer"}`); c != http.StatusCreated {
		t.Fatal(c)
	}
	if p, _ := e.st.PoliticaEnlaces(e.ctx); !p.PorDefecto {
		t.Fatalf("un intento denegado cambió la política: %+v", p)
	}
	// Valores que no son un techo.
	for _, malo := range []string{`{"max_days":-1}`, `{"max_days":3651}`, `{}`, `{"max_days":null}`} {
		if c, _ := politica("PUT", "jefa", admin, malo); c != http.StatusBadRequest {
			t.Errorf("%s: %d", malo, c)
		}
	}

	// «Sin caducidad», solo tras quitar el techo quien administra.
	if c, p := politica("PUT", "jefa", admin, `{"max_days":0}`); c != http.StatusOK || p["adjusted"] != float64(0) {
		t.Fatalf("quitar el techo: %d %v", c, p)
	}
	c, eterno, _ := crear(`{"role":"viewer"}`)
	if c != http.StatusCreated || eterno.ExpiresAt != nil || resuelve(eterno.Token) != http.StatusOK {
		t.Fatalf("sin techo: %d, caduca %v", c, eterno.ExpiresAt)
	}
	if c, _, _ := crear(enDias(5000)); c != http.StatusCreated {
		t.Fatalf("sin techo, una fecha lejana: %d", c)
	}

	// Volver a poner techo: los que ya no cumplen pasan a caducar, marcados, y siguen
	// sirviendo hasta entonces.
	c, p := politica("PUT", "jefa", []string{"owner"}, `{"max_days":7}`)
	if c != http.StatusOK || p["adjusted"].(float64) < 2 {
		t.Fatalf("poner techo de 7 días: %d %v", c, p)
	}
	rec := httptest.NewRecorder()
	e.hd.listShares(rec, e.peticion("GET", "owner", "", nil, "/shares"))
	var lista struct {
		Shares []enlace `json:"shares"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(7*24*time.Hour + 2*time.Minute)
	for _, s := range lista.Shares {
		if s.ExpiresAt == nil || s.ExpiresAt.After(limite) {
			t.Errorf("el enlace %s sigue más allá del techo: %v", s.ID, s.ExpiresAt)
		}
		if s.ID == eterno.ID && s.Ajustado == nil {
			t.Error("el enlace ajustado no lo dice")
		}
	}
	if resuelve(eterno.Token) != http.StatusOK {
		t.Fatal("el enlace ajustado dejó de servir antes de su nueva fecha")
	}
	if c, _, code := crear(enDias(8)); c != http.StatusBadRequest || code != "share_expiry_too_far" {
		t.Fatalf("con techo de 7, pedir 8: %d %s", c, code)
	}
}
