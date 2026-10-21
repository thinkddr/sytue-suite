// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// exchangeForm arma un formulario de canje bien formado, para que cada test rompa
// exactamente un parámetro.
func exchangeForm() url.Values {
	return url.Values{
		"grant_type":           {domain.GrantTokenExchange},
		"subject_token":        {"un-jwt-cualquiera"},
		"subject_token_type":   {tokenTypeAccess},
		"requested_token_type": {tokenTypeAccess},
		"requested_subject":    {"grace@acme.example"},
	}
}

func TestParseExchange(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(url.Values)
		wantBad bool
	}{
		{"petición bien formada", func(url.Values) {}, false},
		{"requested_token_type omitido (opcional)", func(f url.Values) {
			f.Del("requested_token_type")
		}, false},
		{"subject_token_type = jwt", func(f url.Values) {
			f.Set("subject_token_type", tokenTypeJWT)
		}, false},
		{"sin subject_token", func(f url.Values) { f.Del("subject_token") }, true},
		{"subject_token en blanco", func(f url.Values) { f.Set("subject_token", "   ") }, true},
		{"sin subject_token_type (obligatorio)", func(f url.Values) {
			f.Del("subject_token_type")
		}, true},
		{"subject_token_type desconocido", func(f url.Values) {
			f.Set("subject_token_type", "urn:ietf:params:oauth:token-type:saml2")
		}, true},
		// No se emiten ID tokens por canje: pedirlo es un error, no un silencio.
		{"requested_token_type = id_token", func(f url.Values) {
			f.Set("requested_token_type", "urn:ietf:params:oauth:token-type:id_token")
		}, true},
		{"sin requested_subject", func(f url.Values) { f.Del("requested_subject") }, true},
	}
	for _, tc := range cases {
		f := exchangeForm()
		tc.mutate(f)
		_, bad := parseExchange(f)
		if tc.wantBad && bad == "" {
			t.Errorf("%s: se esperaba invalid_request", tc.name)
		}
		if !tc.wantBad && bad != "" {
			t.Errorf("%s: rechazada con %q", tc.name, bad)
		}
	}
}

func TestParseExchangeTrims(t *testing.T) {
	f := exchangeForm()
	f.Set("subject_token", "  tok  ")
	f.Set("requested_subject", " grace@acme.example ")
	f.Set("audience", " sytue-drive ")
	f.Set("scope", " openid profile ")
	p, bad := parseExchange(f)
	if bad != "" {
		t.Fatalf("rechazada: %s", bad)
	}
	if p.SubjectToken != "tok" || p.Subject != "grace@acme.example" ||
		p.Audience != "sytue-drive" || p.Scope != "openid profile" {
		t.Fatalf("parámetros sin recortar: %+v", p)
	}
}

// TestGrantTokenExchangeDeniesNonDelegableClient es la prueba de que la puerta
// está ANTES de todo lo demás: con un client no delegable el handler responde
// unauthorized_client sin llegar a mirar la petición — de hecho corre con un
// Server sin store ni claves, y si tocara alguno de los dos, entraría en pánico.
func TestGrantTokenExchangeDeniesNonDelegableClient(t *testing.T) {
	cases := []struct {
		name   string
		client domain.Client
	}{
		{"client corriente (may_impersonate por defecto)", domain.Client{
			ID: "sytue-console", Type: domain.ClientConfidential,
			GrantTypes: []string{"authorization_code", domain.GrantTokenExchange},
			OrgID:      "org-acme",
		}},
		{"marcado pero global (sin organización)", domain.Client{
			ID: "sytue-gateway", Type: domain.ClientConfidential,
			GrantTypes: []string{domain.GrantTokenExchange}, MayImpersonate: true,
		}},
		{"marcado pero público", domain.Client{
			ID: "app-movil", TokenEndpointAuthMethod: "none",
			GrantTypes: []string{domain.GrantTokenExchange},
			OrgID:      "org-acme", MayImpersonate: true,
		}},
		{"marcado pero sin el grant declarado", domain.Client{
			ID: "acme-migrator", Type: domain.ClientConfidential,
			GrantTypes: []string{"client_credentials"},
			OrgID:      "org-acme", MayImpersonate: true,
		}},
	}
	for _, tc := range cases {
		s := &Server{}
		r := httptest.NewRequest(http.MethodPost, "/oauth2/token",
			strings.NewReader(exchangeForm().Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		rr := httptest.NewRecorder()
		s.grantTokenExchange(rr, r, tc.client)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s: status=%d, quiero 400", tc.name, rr.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: json: %v", tc.name, err)
		}
		if body["error"] != "unauthorized_client" {
			t.Errorf("%s: error=%q, quiero unauthorized_client", tc.name, body["error"])
		}
		// No debe filtrar NADA sobre el sujeto pedido ni sobre el token.
		if strings.Contains(rr.Body.String(), "grace@acme.example") {
			t.Errorf("%s: la respuesta menciona el sujeto pedido: %s", tc.name, rr.Body)
		}
	}
}

// TestGrantTokenExchangeMalformed: con un client SÍ delegable, una petición mal
// formada muere en invalid_request antes de tocar claves o store (Server vacío).
func TestGrantTokenExchangeMalformed(t *testing.T) {
	client := domain.Client{
		ID: "acme-migrator", Type: domain.ClientConfidential,
		GrantTypes: []string{domain.GrantTokenExchange},
		OrgID:      "org-acme", MayImpersonate: true,
	}
	f := exchangeForm()
	f.Del("subject_token")

	s := &Server{}
	r := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(f.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	rr := httptest.NewRecorder()
	s.grantTokenExchange(rr, r, client)

	var body map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if rr.Code != http.StatusBadRequest || body["error"] != "invalid_request" {
		t.Fatalf("status=%d error=%q, quiero 400 invalid_request", rr.Code, body["error"])
	}
}

// TestDiscoveryAnnouncesTokenExchange: el canje se anuncia porque está
// implementado, y el resto de grants siguen anunciados.
func TestDiscoveryAnnouncesTokenExchange(t *testing.T) {
	s := &Server{cfg: Config{Issuer: "https://id.example"}}
	rr := httptest.NewRecorder()
	s.handleDiscovery(rr, httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", http.NoBody))
	var doc map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	got := map[string]bool{}
	for _, v := range doc["grant_types_supported"].([]any) {
		got[v.(string)] = true
	}
	for _, want := range []string{
		"authorization_code", "refresh_token", "client_credentials", domain.GrantTokenExchange,
	} {
		if !got[want] {
			t.Errorf("grant_types_supported no anuncia %q: %v", want, doc["grant_types_supported"])
		}
	}
}
