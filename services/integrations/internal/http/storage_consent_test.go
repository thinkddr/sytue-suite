// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/config"
	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
)

func TestDropboxStorageNoConfundeElCatalogoSAML(t *testing.T) {
	if !esPlantillaDeAlmacen("dropbox-storage") || esPlantillaDeAlmacen("dropbox") {
		t.Fatal("la cuenta OAuth de almacenamiento es distinta de Dropbox Business SAML")
	}
	l := &loaderFalso{datos: map[string]struct{ ClientID, ClientSecret string }{}}
	r := newOAuthClientResolver(l, map[string]config.OAuthClient{"dropbox": {ClientID: "app", ClientSecret: "secret"}})
	if got := r.get(context.Background(), "dropbox-storage"); got.ClientID != "app" || got.ClientSecret != "secret" {
		t.Fatalf("no reutilizó la credencial de plataforma del proveedor: %+v", got)
	}
}

func TestOAuthAceptaLosDosFormatosDeCaducidad(t *testing.T) {
	for _, raw := range []string{`3600`, `"3600"`} {
		var tok tokenResponse
		if err := json.Unmarshal([]byte(`{"access_token":"a","refresh_token":"r","expires_in":`+raw+`}`), &tok); err != nil {
			t.Fatal(err)
		}
		if tok.ExpiresIn != 3600 || tok.AccessToken != "a" || tok.RefreshToken != "r" {
			t.Fatalf("token inválido: %+v", tok)
		}
	}
	var tok tokenResponse
	if err := json.Unmarshal([]byte(`{"expires_in":"mucho"}`), &tok); err == nil {
		t.Fatal("un vencimiento inventado no se puede tratar como un token permanente")
	}
}

func TestDenegarVuelveAlAlmacenSoloConEstadoYPersonaValidos(t *testing.T) {
	cipher, err := keycrypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	st, err := consent.Begin(cipher, consent.Flow{AuthorizeURL: "https://www.dropbox.com/oauth2/authorize", ClientID: "app"},
		"acme", "ana", "instance", "https://integrations.sytue.eu/callback", "storage", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(st.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ sub, state, section, outcome string }{
		{"ana", u.Query().Get("state"), "storage", "denied"},
		{"bea", u.Query().Get("state"), "apps", "state"},
		{"ana", "fabricado", "apps", "state"},
	} {
		body, _ := json.Marshal(oauthFinishReq{State: tc.state, Error: "access_denied"})
		req := httptest.NewRequest(http.MethodPost, "/oauth/finish", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		ctx := authn.Into(tenancy.Into(req.Context(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Subject: tc.sub})
		w := httptest.NewRecorder()
		(&Handlers{oauthCipher: cipher}).OAuthFinish(w, req.WithContext(ctx))
		var out oauthFinishResp
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || out.Section != tc.section || out.Error != tc.outcome {
			t.Fatalf("sujeto %q: status=%d out=%+v", tc.sub, w.Code, out)
		}
	}
}
