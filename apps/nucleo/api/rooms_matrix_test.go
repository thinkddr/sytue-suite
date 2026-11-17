// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
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

func TestMatrixIsolatedNativeAuthorizationRejectsDowngradeAtStartAndFinish(t *testing.T) {
	for _, phase := range []string{"start", "finish", "missing-tenant", "invalid-tenant", "valid"} {
		t.Run(phase, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["identity_version"] != float64(storage.IsolatedIdentity) || body["product"] != "drive" {
					t.Error("client failed to bind isolated identity")
				}
				version := 2
				if strings.HasSuffix(r.URL.Path, "/start") {
					if phase == "start" {
						version = 1
					}
					tenant := "acme"
					if phase == "missing-tenant" {
						tenant = ""
					}
					if phase == "invalid-tenant" {
						tenant = "foreign\x00tenant"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"product": "drive", "tenant": tenant, "identity_version": version, "state": "isolated-state", "authorize_url": "https://rooms.sytue.eu/rooms/matrix/native/start?state=isolated-state"})
				} else {
					if phase == "finish" {
						version = 1
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"login_token": "artificial-one-use-token", "identity_version": version})
				}
			}))
			defer server.Close()
			client := NewRooms(server.URL, nil, server.Client())
			flow, err := client.MatrixNativeStartForIdentity(t.Context(), "drive", storage.IsolatedIdentity)
			if phase == "start" || phase == "missing-tenant" || phase == "invalid-tenant" {
				if !errors.Is(err, ErrMatrixSSO) {
					t.Fatal("server downgraded new account", err)
				}
				return
			}
			if err != nil || flow.IdentityVersion() != storage.IsolatedIdentity || flow.Tenant() != "acme" {
				t.Fatal(err)
			}
			u, _ := url.Parse("sytue-drive://matrix/callback?state=isolated-state&ticket=opaque-ticket")
			callback, ok := ParseMatrixNativeCallbackFor(u, "drive")
			if !ok {
				t.Fatal("fixture callback rejected")
			}
			token, err := client.MatrixNativeFinish(t.Context(), flow, callback)
			if phase == "finish" {
				if !errors.Is(err, ErrMatrixSSO) || token != "" {
					t.Fatal("finish accepted downgraded identity", err)
				}
				return
			}
			if err != nil || token != "artificial-one-use-token" {
				t.Fatal(err)
			}
		})
	}
}

// La copia con KEK de ADR 0081 solo se lee, para la transición de ADR 0082. Un
// 404 es «no hay ninguna» y la lectura nunca puede acabar en la caché sin conexión.
func TestMatrixRecoveryKeyLegacyReadFollowsTheWebContract(t *testing.T) {
	const key = "EsTb 1234 5678 9ABC DEFG HJKL MNPQ RSTU VWXY Zabc defg hijk"
	stored := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/matrix/recovery-key" || r.Header.Get("Authorization") != "Bearer sytue-session" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Cache-Control") != "no-store" {
			t.Error("the recovery key read can be cached on disk")
		}
		if stored == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"recovery_key": stored})
	}))
	defer server.Close()
	client := NewRooms(server.URL, sesionFija("sytue-session"), server.Client())
	if got, err := client.MatrixRecoveryKey(t.Context()); err != nil || got != "" {
		t.Fatalf("missing escrow must read as none: %q %v", got, err)
	}
	stored = key
	if got, err := client.MatrixRecoveryKey(t.Context()); err != nil || got != key {
		t.Fatalf("read back: %q %v", got, err)
	}
	stored = "<script>"
	if got, err := client.MatrixRecoveryKey(t.Context()); err != nil || got != "" {
		t.Fatalf("a value that is not a recovery key was accepted: %q %v", got, err)
	}
}

// Los envoltorios (ADR 0082) van por cuenta: sin parámetros es Rooms; la cuenta
// aislada lleva product e identity_version. Lo que viaja no lleva el secreto, y lo
// que vuelve con otra forma se descarta antes de derivar nada.
func TestMatrixRecoveryWrappersFollowTheContract(t *testing.T) {
	const key = "EsTb 1234 5678 9ABC DEFG HJKL MNPQ RSTU VWXY Zabc defg hijk"
	good, err := recoverywrap.WrapPassword("contraseña", "@sytue_storage_drive_acme_a:x", "kid", key)
	if err != nil {
		t.Fatal(err)
	}
	weak := good
	weak.KDF.Memory = 8
	var queries []string
	var put []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/matrix/recovery-wrappers" || r.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
		queries = append(queries, r.Method+" "+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"wrappers": []recoverywrap.Wrapper{good, weak}, "legacy_escrow": true})
		case http.MethodPut:
			put, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "w1"})
		}
	}))
	defer server.Close()
	client := NewRooms(server.URL, sesionFija("sytue-session"), server.Client())
	isolated := MatrixRecoveryAccount{Product: "drive", IdentityVersion: storage.IsolatedIdentity}
	got, err := client.MatrixRecoveryWrappers(t.Context(), isolated)
	if err != nil || len(got.Wrappers) != 1 || !got.LegacyEscrow {
		t.Fatalf("wrappers = %+v, %v", got, err)
	}
	if _, err := client.MatrixRecoveryWrappers(t.Context(), MatrixRecoveryAccount{}); err != nil {
		t.Fatal(err)
	}
	if id, err := client.PutMatrixRecoveryWrapper(t.Context(), isolated, good); err != nil || id != "w1" {
		t.Fatalf("put = %q, %v", id, err)
	}
	if strings.Contains(string(put), "EsTb") || strings.Contains(string(put), "contraseña") {
		t.Fatalf("the secret left the device: %s", put)
	}
	if _, err := client.PutMatrixRecoveryWrapper(t.Context(), isolated, weak); err == nil {
		t.Fatal("a weak wrapper was sent")
	}
	if _, err := client.MatrixRecoveryWrappers(t.Context(), MatrixRecoveryAccount{IdentityVersion: storage.IsolatedIdentity}); err == nil {
		t.Fatal("a version without a product selected an account")
	}
	want := []string{"GET identity_version=2&product=drive", "GET ", "PUT identity_version=2&product=drive"}
	if strings.Join(queries, "|") != strings.Join(want, "|") {
		t.Fatalf("queries = %q; want %q", queries, want)
	}
}

// Un fallo del servicio no se lee como «no hay nada».
func TestMatrixRecoveryKeyErrorsAreNotEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := NewRooms(server.URL, sesionFija("sytue-session"), server.Client())
	if _, err := client.MatrixRecoveryKey(t.Context()); err == nil {
		t.Fatal("an unavailable escrow read as an empty one")
	}
	if _, err := client.MatrixRecoveryWrappers(t.Context(), MatrixRecoveryAccount{}); err == nil {
		t.Fatal("unavailable wrappers read as none")
	}
}

// Retirar los envoltorios de la 0082 (ADR 0085) es un guardado del `sytue-id` con
// `retire_older=1`; con otro tipo ni se pide.
func TestRetirarLosViejosEsUnGuardadoDelSytueIDConRetireOlder(t *testing.T) {
	const key = "EsTb 1234 5678 9ABC DEFG HJKL MNPQ RSTU VWXY Zabc defg hijk"
	unlock, _ := recoverywrap.NewUnlockKey()
	nuevo, err := recoverywrap.WrapSytueID(unlock, "@sytue_storage_drive_acme_a:x", "kid", key)
	if err != nil {
		t.Fatal(err)
	}
	viejo, err := recoverywrap.WrapPassword("contraseña", "@sytue_storage_drive_acme_a:x", "kid", key)
	if err != nil {
		t.Fatal(err)
	}
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.Method+" "+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "w1"})
	}))
	defer server.Close()
	client := NewRooms(server.URL, sesionFija("sytue-session"), server.Client())
	isolated := MatrixRecoveryAccount{Product: "drive", IdentityVersion: storage.IsolatedIdentity}
	if _, err := client.PutMatrixRecoveryWrapperRetiringOlder(t.Context(), isolated, viejo); err == nil {
		t.Fatal("un envoltorio de la 0082 pidió retirar los demás")
	}
	if id, err := client.PutMatrixRecoveryWrapperRetiringOlder(t.Context(), isolated, nuevo); err != nil || id != "w1" {
		t.Fatalf("retirar = %q, %v", id, err)
	}
	if want := []string{"PUT identity_version=2&product=drive&retire_older=1"}; strings.Join(queries, "|") != strings.Join(want, "|") {
		t.Fatalf("queries = %q; want %q", queries, want)
	}
}
