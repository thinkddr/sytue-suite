// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBucketPolicyScopedToBucket es el corazón de seguridad: la policy debe acotar
// la credencial a EXACTAMENTE el bucket del tenant, sin comodines de recurso ni de
// principal (nada de acceso de cuenta).
func TestBucketPolicyScopedToBucket(t *testing.T) {
	const bucket = "sytue-app-abc123"
	const appID = "11111111-2222-3333-4444-555555555555"
	raw, err := bucketPolicyJSON(bucket, appID)
	if err != nil {
		t.Fatalf("bucketPolicyJSON: %v", err)
	}
	var pol struct {
		Version   string `json:"Version"`
		Statement []struct {
			Effect    string              `json:"Effect"`
			Principal map[string][]string `json:"Principal"`
			Action    []string            `json:"Action"`
			Resource  []string            `json:"Resource"`
		} `json:"Statement"`
	}
	if err := json.Unmarshal([]byte(raw), &pol); err != nil {
		t.Fatalf("policy no es JSON válido: %v\n%s", err, raw)
	}
	if len(pol.Statement) != 1 {
		t.Fatalf("se esperaba 1 statement, hay %d", len(pol.Statement))
	}
	st := pol.Statement[0]
	if st.Effect != "Allow" {
		t.Errorf("Effect = %q, quiero Allow", st.Effect)
	}
	// Recurso ACOTADO: solo el bucket y sus objetos. Ni comodín de cuenta ni otro
	// bucket. Esta es la invariante que cierra la fuga cross-tenant.
	wantRes := map[string]bool{bucket: true, bucket + "/*": true}
	if len(st.Resource) != len(wantRes) {
		t.Fatalf("Resource = %v, quiero exactamente %v", st.Resource, wantRes)
	}
	for _, r := range st.Resource {
		if !wantRes[r] {
			t.Errorf("Resource inesperado %q (no acotado al bucket)", r)
		}
		if r == "*" || strings.HasPrefix(r, "*") {
			t.Errorf("Resource comodín %q: fuga de cuenta", r)
		}
	}
	// Principal ACOTADO a la application del tenant, nunca comodín.
	scw := st.Principal["SCW"]
	if len(scw) != 1 || scw[0] != "application_id:"+appID {
		t.Errorf("Principal SCW = %v, quiero [application_id:%s]", scw, appID)
	}
	for _, p := range scw {
		if p == "*" {
			t.Errorf("Principal comodín: cualquiera tendría acceso")
		}
	}
}

// TestSCWIAMClientCreateRotateMapping ejercita el mapeo REST de acuñado contra un
// Scaleway IAM simulado (httptest): no hace falta un Scaleway vivo.
func TestSCWIAMClientCreateRotateMapping(t *testing.T) {
	var seenToken string
	var createdName, createdOrg string
	var apiKeyAppID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenToken = r.Header.Get("X-Auth-Token")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/applications":
			_, _ = io.WriteString(w, `{"applications":[]}`) // miss ⇒ fuerza create
		case r.Method == http.MethodPost && r.URL.Path == "/applications":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			createdName, createdOrg = body["name"], body["organization_id"]
			_, _ = io.WriteString(w, `{"id":"app-xyz","name":"`+body["name"]+`"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api-keys":
			_, _ = io.WriteString(w, `{"api_keys":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api-keys":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			apiKeyAppID = body["application_id"]
			_, _ = io.WriteString(w, `{"access_key":"SCWAK","secret_key":"topsecret"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusTeapot)
		}
	}))
	defer srv.Close()

	c := &scwIAMClient{baseURL: srv.URL, token: "tok-123", orgID: "org-9", hc: srv.Client()}
	ctx := context.Background()

	appID, err := c.EnsureApplication(ctx, "sytue-app-abc")
	if err != nil {
		t.Fatalf("EnsureApplication: %v", err)
	}
	if appID != "app-xyz" {
		t.Errorf("appID = %q, quiero app-xyz", appID)
	}
	if createdName != "sytue-app-abc" || createdOrg != "org-9" {
		t.Errorf("body create = name %q org %q", createdName, createdOrg)
	}
	if seenToken != "tok-123" {
		t.Errorf("X-Auth-Token = %q, quiero tok-123", seenToken)
	}

	ak, sk, err := c.RotateAPIKey(ctx, appID)
	if err != nil {
		t.Fatalf("RotateAPIKey: %v", err)
	}
	if ak != "SCWAK" || sk != "topsecret" {
		t.Errorf("key = %q/%q, quiero SCWAK/topsecret", ak, sk)
	}
	if apiKeyAppID != "app-xyz" {
		t.Errorf("api-key application_id = %q, quiero app-xyz", apiKeyAppID)
	}
}

// TestSCWIAMClientDeleteMapping comprueba que el borrado quita primero las API keys
// y luego la application (no se confía en cascada para no filtrar credenciales).
func TestSCWIAMClientDeleteMapping(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/applications":
			_, _ = io.WriteString(w, `{"applications":[{"id":"app-del","name":"sytue-app-del"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api-keys":
			_, _ = io.WriteString(w, `{"api_keys":[{"access_key":"SCWK"}]}`)
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := &scwIAMClient{baseURL: srv.URL, token: "tok", orgID: "org", hc: srv.Client()}
	if err := c.DeleteApplication(context.Background(), "sytue-app-del"); err != nil {
		t.Fatalf("DeleteApplication: %v", err)
	}
	// La key ANTES que la application.
	if len(deleted) != 2 || deleted[0] != "/api-keys/SCWK" || deleted[1] != "/applications/app-del" {
		t.Errorf("orden/paths de borrado = %v", deleted)
	}
}

// TestSCWIAMClientErrorStatus comprueba que un status no-2xx se propaga como error
// (no se sigue adelante como si hubiera acuñado credencial).
func TestSCWIAMClientErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"denied"}`, http.StatusForbidden)
	}))
	defer srv.Close()
	c := &scwIAMClient{baseURL: srv.URL, token: "tok", orgID: "org", hc: srv.Client()}
	if _, err := c.EnsureApplication(context.Background(), "x"); err == nil {
		t.Fatal("se esperaba error ante 403")
	}
}
