// SPDX-License-Identifier: Apache-2.0
package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestMatrixResourceNativeConsentRetainsPurposeAndPKCE(t *testing.T) {
	const state = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	var challenge string
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer resource-client" {
			t.Error("resource consent lost current actor")
		}
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/api/v1/matrix/resources/native/start":
			if input["product"] != "docs" || input["code_challenge_method"] != "S256" || input["code_verifier"] != "" || input["namespace"] != "" {
				t.Error("unbound purpose or exposed PKCE verifier")
			}
			challenge = input["code_challenge"]
			_ = json.NewEncoder(w).Encode(map[string]any{"product": "docs", "tenant": "acme", "purpose": "resources:docs", "resource_approved": false, "state": state, "authorize_url": "https://rooms.sytue.eu/rooms/matrix/native/start?state=" + state})
		case "/api/v1/matrix/resources/native/finish":
			calls++
			if len(input) != 3 || input["state"] != state || input["ticket"] != "opaque" {
				t.Error("finish selected new product/scope/actor")
			}
			hash := sha256.Sum256([]byte(input["code_verifier"]))
			if base64.RawURLEncoding.EncodeToString(hash[:]) != challenge {
				t.Error("retained verifier changed")
			}
			_ = json.NewEncoder(w).Encode(MatrixResourceGrant{LoginToken: "artificial-one-use-sdk-token", Product: "docs", Namespace: "resources:docs", Tenant: "acme", ConsentID: "00000000-0000-4000-8000-000000000001", Grant: strings.Repeat("G", 43)})
		default:
			t.Error("wrong consent endpoint")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewRooms(server.URL, sesionFija("resource-client"), server.Client())
	flow, err := client.MatrixResourceNativeStart(t.Context(), "docs")
	if err != nil {
		t.Fatal(err)
	}
	if flow.Purpose() != "resources:docs" || !flow.ForResources("sytue.eu", "docs") || flow.ForResources("sytue.eu", "mail") {
		t.Fatal("browser consent not bound to product")
	}
	callbackURL := &url.URL{Scheme: flow.ReturnScheme(), Host: "matrix", Path: "/callback", RawQuery: url.Values{"state": {state}, "ticket": {"opaque"}}.Encode()}
	callback, ok := ParseMatrixNativeCallbackFor(callbackURL, "docs")
	if !ok {
		t.Fatal("product callback rejected")
	}
	if _, err := client.MatrixNativeFinish(t.Context(), flow, callback); !errors.Is(err, ErrMatrixSSO) || calls != 0 {
		t.Fatal("legacy exchange consumed resource authority")
	}
	if _, err := client.MatrixResourceNativeFinish(t.Context(), flow, MatrixNativeCallback{State: "wrong", Ticket: "opaque"}); !errors.Is(err, ErrMatrixSSO) || calls != 0 {
		t.Fatal("foreign callback reached resource exchange")
	}
	grant, err := client.MatrixResourceNativeFinish(t.Context(), flow, callback)
	if err != nil || grant.Namespace != "resources:docs" || calls != 1 {
		t.Fatal("consent exchange failed", err)
	}
}

func TestMatrixResourceNativeStartRejectsMissingPurposeAndSilentApproval(t *testing.T) {
	approved, denied := true, false
	base := matrixNativeStartResponse{Purpose: "resources:docs", ResourceApproved: &denied, Product: "docs", Tenant: "acme", State: "bound", AuthorizeURL: "https://rooms.sytue.eu/rooms/matrix/native/start?state=bound"}
	for name, mutate := range map[string]func(*matrixNativeStartResponse){
		"legacy":           func(v *matrixNativeStartResponse) { v.Purpose = "" },
		"another-product":  func(v *matrixNativeStartResponse) { v.Purpose = "resources:mail" },
		"missing-approval": func(v *matrixNativeStartResponse) { v.ResourceApproved = nil },
		"silent-approval":  func(v *matrixNativeStartResponse) { v.ResourceApproved = &approved },
		"missing-tenant":   func(v *matrixNativeStartResponse) { v.Tenant = "" },
		"changed-product":  func(v *matrixNativeStartResponse) { v.Product = "mail" },
	} {
		t.Run(name, func(t *testing.T) {
			v := base
			mutate(&v)
			if _, err := checkedMatrixNativeStartPurpose(v, "docs", "", "private-verifier", storage.LegacyIdentity, "resources:docs"); !errors.Is(err, ErrMatrixSSO) {
				t.Fatal("unretained or silently granted purpose", err)
			}
		})
	}
}
