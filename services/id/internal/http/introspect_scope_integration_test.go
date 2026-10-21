// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// ID-03: la introspección acotada al client dueño del token. Un client de
// terceros (acotado a una organización) solo puede introspeccionar los access
// tokens que se emitieron para él; los clients globales de la suite (resource
// servers) mantienen la introspección abierta tras la auth de client.
package http

import (
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

func TestIntrospectAcotadaAlClientDeTerceros(t *testing.T) {
	l := newLiveServer(t)
	org, adminTok := l.orgAdmin(t, "acme", "admin@acme.example")
	u := l.mkUser(t, "ana@acme.example", "")

	// App de terceros de la organización (client acotado a acme).
	const redirectURI = "https://tercero.example/cb"
	rr, created := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
		"name":          "App de terceros",
		"redirect_uris": []string{redirectURI},
		"scopes":        []string{"openid", "profile", "email"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta de la app: %d %s", rr.Code, rr.Body)
	}
	appID, _ := created["client_id"].(string)
	appSecret, _ := created["client_secret"].(string)
	basicApp := "Basic " + base64.StdEncoding.EncodeToString([]byte(appID+":"+appSecret))

	// Token emitido PARA la app (su client_id va en el aud).
	suyo, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: u.ID, Audience: appID, Scope: "openid profile", Tenant: org.Slug, Roles: []string{"admin"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("IssueAccessToken(suyo): %v", err)
	}
	// Token emitido para OTRO client (la consola, de primera parte).
	ajeno, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: u.ID, Audience: "sytue-console", Scope: "openid profile", Tenant: org.Slug, Roles: []string{"admin"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("IssueAccessToken(ajeno): %v", err)
	}

	// La app ve SU token.
	if rr, out := l.introspect(t, basicApp, suyo); rr.Code != http.StatusOK || out["active"] != true {
		t.Fatalf("la app debe ver su propio token: %d %v", rr.Code, out)
	}
	// La app NO ve un token ajeno: {active:false} y nada más (ID-03).
	if _, out := l.introspect(t, basicApp, ajeno); out["active"] == true || len(out) != 1 {
		t.Fatalf("la app no debe introspeccionar un token ajeno: %v", out)
	}

	// Control: un client GLOBAL de la suite (la consola) sí ve cualquier token
	// válido tras autenticarse (resource servers), sin regresión.
	basicConsola := l.setClientSecret(t, "sytue-console", "console-secret-1")
	if _, out := l.introspect(t, basicConsola, suyo); out["active"] != true {
		t.Fatalf("un client global debe ver un token de otra app: %v", out)
	}
	if _, out := l.introspect(t, basicConsola, ajeno); out["active"] != true {
		t.Fatalf("un client global debe ver su propio token: %v", out)
	}
}
