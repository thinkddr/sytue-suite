// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

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
)

func TestMatrixNativeHandoffUsesPKCEAndMatchingState(t *testing.T) {
	var challenge string
	finishes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer sytue-session" {
			t.Error("handoff lost actor bearer")
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/v1/matrix/native/start":
			if _, extra := body["product"]; extra {
				t.Error("Rooms login lost compatibility with the existing broker")
			}
			challenge = body["code_challenge"]
			if len(challenge) != 43 || body["code_challenge_method"] != "S256" || body["code_verifier"] != "" {
				t.Error("PKCE verifier exposed at start")
			}
			_, _ = w.Write([]byte(`{"authorize_url":"https://rooms.sytue.eu/rooms/matrix/native/start?state=bound-state","state":"bound-state"}`))
		case "/api/v1/matrix/native/finish":
			finishes++
			digest := sha256.Sum256([]byte(body["code_verifier"]))
			if len(body["code_verifier"]) != 43 || base64.RawURLEncoding.EncodeToString(digest[:]) != challenge || body["ticket"] != "opaque-ticket" || body["state"] != "bound-state" {
				t.Error("finish lost PKCE/state binding")
			}
			_, _ = w.Write([]byte(`{"login_token":"one-use-private-token"}`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := NewRooms(server.URL, sesionFija("sytue-session"), server.Client())
	flow, err := client.MatrixNativeStart(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !flow.ForRooms("sytue.eu") || flow.ForRooms("another.invalid") {
		t.Fatal("browser bootstrap escaped first-party Rooms")
	}
	for _, callback := range []MatrixNativeCallback{{State: "unrelated", Ticket: "opaque-ticket"}, {State: "bound-state", Ticket: ""}} {
		if _, err = client.MatrixNativeFinish(t.Context(), flow, callback); !errors.Is(err, ErrMatrixSSO) {
			t.Fatalf("foreign callback: %v", err)
		}
	}
	if _, err = client.MatrixNativeFinish(t.Context(), flow, MatrixNativeCallback{State: "bound-state", Error: "denied"}); !errors.Is(err, ErrOAuthDenied) {
		t.Fatalf("denial: %v", err)
	}
	if finishes != 0 {
		t.Fatal("invalid callback reached authenticated exchange")
	}
	u, _ := url.Parse("sytue-rooms://matrix/callback?state=bound-state&ticket=opaque-ticket")
	callback, ok := ParseMatrixNativeCallback(u)
	if !ok || !flow.Accepts(callback) {
		t.Fatal("valid callback rejected")
	}
	token, err := client.MatrixNativeFinish(t.Context(), flow, callback)
	if err != nil || token != "one-use-private-token" || finishes != 1 {
		t.Fatalf("finish: %v", err)
	}
}

func TestMatrixNativeReturnRejectsCredentialsAndForeignRoutes(t *testing.T) {
	for _, raw := range []string{
		"sytue-drive://matrix/callback?state=s&ticket=t",
		"sytue-rooms://matrix/callback?state=s&loginToken=credential",
		"sytue-rooms://matrix/callback?state=s&ticket=t&code=credential",
		"sytue-rooms://matrix/callback?state=s&state=other&ticket=t",
		"sytue-rooms://matrix/callback?state=s&ticket=t#secret",
		"sytue-rooms://user@matrix/callback?state=s&ticket=t",
		"sytue-rooms://matrix/callback?state=s&ticket=" + strings.Repeat("x", 4097),
	} {
		u, _ := url.Parse(raw)
		if _, ok := ParseMatrixNativeCallback(u); ok {
			t.Fatal("accepted credential-bearing or unrelated Matrix return")
		}
	}
}

func TestMatrixStorageNativeProductsBindAppBeforeBrowserConsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"product": input["product"], "state": "bound-state", "authorize_url": "https://rooms.sytue.eu/rooms/matrix/native/start?state=bound-state"})
	}))
	defer server.Close()
	client := NewRooms(server.URL, nil, server.Client())
	for _, product := range []string{"hub", "mail", "drive", "docs", "sheets", "slides", "pdf", "calendar", "contacts", "notes", "photos", "rooms", "cast", "erp", "bases"} {
		flow, err := client.MatrixNativeStartFor(t.Context(), product)
		if err != nil || !flow.ForProduct("sytue.eu", product) || flow.ForProduct("sytue.eu", "unregistered") {
			t.Fatal("unbound native app", product, err)
		}
		native := product
		if native == "hub" {
			native = "one"
		}
		if flow.ReturnScheme() != Esquema(native) {
			t.Fatal("wrong native catalog scheme", product)
		}
		u, _ := url.Parse(Esquema(native) + "://matrix/callback?state=bound-state&ticket=opaque-ticket")
		callback, ok := ParseMatrixNativeCallbackFor(u, product)
		if !ok || !flow.Accepts(callback) {
			t.Fatal("own app return rejected", product)
		}
		other := "drive"
		if product == other {
			other = "mail"
		}
		if _, ok := ParseMatrixNativeCallbackFor(u, other); ok {
			t.Fatal("callback crossed app boundary", product)
		}
		callback.product = other
		if flow.Accepts(callback) {
			t.Fatal("same state accepted for another app", product)
		}
	}
	if _, err := client.MatrixNativeStartFor(t.Context(), "https://attacker.invalid"); !errors.Is(err, ErrMatrixSSO) {
		t.Fatal("arbitrary app destination accepted", err)
	}
}
