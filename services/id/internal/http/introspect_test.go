// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestIntrospectRequiresClientAuth: sin auth de client (ni Basic ni client_id en
// el form) la introspección devuelve 401 — nunca se escanean tokens de forma
// anónima. No toca el store, así que corre sin base de datos.
func TestIntrospectRequiresClientAuth(t *testing.T) {
	s := &Server{}
	form := strings.NewReader("token=whatever")
	r := httptest.NewRequest(http.MethodPost, "/oauth2/introspect", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.handleIntrospect(rr, r)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth de client: status=%d, quiero 401", rr.Code)
	}
	// No debe filtrar nada del token: solo el error OAuth de client.
	if strings.Contains(rr.Body.String(), "active") {
		t.Fatalf("no debe revelar estado del token: %s", rr.Body)
	}
}

func TestClientIDFromAud(t *testing.T) {
	const res = "https://id.test/api"
	cases := []struct {
		aud  []string
		sub  string
		want string
	}{
		{[]string{res, "sytue-console"}, "user-1", "sytue-console"}, // user token
		{[]string{res}, "sytue-cloud-api", "sytue-cloud-api"},       // client_credentials (sub=client)
		{nil, "user-1", ""}, // sin aud
	}
	for _, tc := range cases {
		if got := clientIDFromAud(tc.aud, res, tc.sub); got != tc.want {
			t.Fatalf("clientIDFromAud(%v)=%q, quiero %q", tc.aud, got, tc.want)
		}
	}
}

// TestMeUpdateRejectsBlankName: un nombre vacío (o solo espacios/control) tras
// SanitizeDisplayName se rechaza con 400, sin tocar el store.
func TestMeUpdateRejectsBlankName(t *testing.T) {
	s := &Server{}
	ctx := context.WithValue(context.Background(), ctxKeyClaims{}, tokenClaims{Subject: "user-1"})
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/me",
		strings.NewReader(`{"name":"   "}`)).WithContext(ctx)
	rr := httptest.NewRecorder()
	s.handleMeUpdate(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("nombre en blanco: status=%d, quiero 400", rr.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if _, isErr := body["error"]; !isErr {
		t.Fatalf("se esperaba un error JSON, cuerpo=%s", rr.Body)
	}
}
