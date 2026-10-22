// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNativeConsentCallbackIsBoundToProductAndNonce(t *testing.T) {
	start := OAuthAuthorization{State: "sealed-state"}
	for _, raw := range []string{
		"sytue-rooms://connect/callback?state=sealed-state&code=a",
		"sytue-drive://auth/callback?state=sealed-state&code=a",
		"sytue-drive://connect/callback/other?state=sealed-state&code=a",
		"sytue-drive://connect/callback?state=sealed-state&state=other&code=a",
		"sytue-drive://user@connect/callback?state=sealed-state&code=a",
		"sytue-drive://connect/callback?code=a",
	} {
		u, _ := url.Parse(raw)
		if _, ok := ParseOAuthCallback("drive", u); ok {
			t.Fatalf("accepted foreign/malformed return %s", raw)
		}
	}
	u, _ := url.Parse("sytue-drive://connect/callback?state=sealed-state&code=a")
	callback, ok := ParseOAuthCallback("drive", u)
	if !ok || !start.Accepts(callback) || callback.Code != "a" {
		t.Fatal("matching return rejected")
	}
	callback.State = "another-flow"
	if start.Accepts(callback) {
		t.Fatal("an unrelated flow would reach authenticated finish")
	}
	if NativeConsentProduct("native-apps") != "one" || NativeConsentProduct("https://elsewhere.example") != "" {
		t.Fatal("native destination allowlist changed")
	}
	if NativeConsentProductForScheme("sytue-mail") != "mail" || NativeConsentProductForScheme("sytue-unknown") != "" || NativeConsentProductForScheme("") != "" {
		t.Fatal("native scheme dispatch does not match the consent allowlist")
	}
}

func TestConsentReadinessFailsClosedAndReusesRegisteredOwnApp(t *testing.T) {
	for _, ownReady := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy response without readiness", true: "registered organisation app"}[ownReady], func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/templates":
					_, _ = w.Write([]byte(`{"templates":[{"key":"google-drive","enabled":true,"manifest":{"name":"Drive","oauth":{"authorize_url":"https://accounts.google.com/auth"}}}]}`))
				case "/api/v1/instances":
					_ = json.NewEncoder(w).Encode(map[string]any{"instances": []map[string]any{
						{"id": "incomplete", "template_key": "google-drive", "status": "draft"},
						{"id": "registered", "template_key": "google-drive", "status": "draft", "oauth_available": ownReady},
					}})
				case "/api/v1/instances/registered/oauth/start":
					writes++
					_, _ = w.Write([]byte(`{"authorize_url":"https://accounts.google.com/auth?state=sealed"}`))
				default:
					t.Errorf("unexpected consent request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()
			client := NewIntegraciones(server.URL, nil, server.Client())
			flow, err := client.OAuthStart(t.Context(), "google-drive", "", "native-storage")
			if !ownReady {
				if !errors.Is(err, ErrOAuthUnavailable) || writes != 0 {
					t.Fatal("unconfigured provider can start consent")
				}
			} else if err != nil || flow.InstanceID != "registered" || writes != 1 {
				t.Fatalf("registered app not reused: instance=%s writes=%d err=%v", flow.InstanceID, writes, err)
			}
		})
	}
}

func TestOAuthStartReusesOnlyMatchingInstanceAndValidatesProvider(t *testing.T) {
	for _, target := range []string{"https://accounts.google.com/authorize?state=sealed-state", "https://attacker.invalid/authorize?state=sealed-state", "http://accounts.google.com/authorize?state=sealed-state", "https://user:secret@accounts.google.com/authorize?state=sealed-state", "https://accounts.google.com/authorize"} {
		t.Run(target, func(t *testing.T) {
			starts, created := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /api/v1/templates":
					_ = json.NewEncoder(w).Encode(map[string]any{"templates": []any{map[string]any{"key": "google-drive", "enabled": true, "oauth_available": true, "manifest": map[string]any{"name": "Google Drive", "oauth": map[string]any{"authorize_url": "https://accounts.google.com/authorize", "scopes": []string{"drive.file"}}}}}})
				case "GET /api/v1/instances":
					_, _ = w.Write([]byte(`{"instances":[{"id":"account","template_key":"google-drive","status":"draft"}]}`))
				case "POST /api/v1/instances":
					created++
					t.Error("reused account should not create another instance")
				case "POST /api/v1/instances/account/oauth/start":
					starts++
					var body map[string]string
					if json.NewDecoder(r.Body).Decode(&body) != nil || body["return_to"] != "native-storage" {
						t.Error("native return not sealed by gateway")
					}
					_ = json.NewEncoder(w).Encode(map[string]string{"authorize_url": target})
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()
			client := NewIntegraciones(server.URL, nil, server.Client())
			flow, err := client.OAuthStart(context.Background(), "google-drive", "account", "native-storage")
			valid := target == "https://accounts.google.com/authorize?state=sealed-state"
			if (err == nil) != valid {
				t.Fatalf("valid=%v error=%v", valid, err)
			}
			if valid && (flow.State != "sealed-state" || flow.InstanceID != "account") {
				t.Fatal("flow binding lost")
			}
			_, err = client.OAuthStart(context.Background(), "google-drive", "foreign", "native-storage")
			if !errors.Is(err, ErrOAuthState) {
				t.Fatal("accepted account outside selected template inventory")
			}
			if starts != 1 || created != 0 {
				t.Fatal("a mismatch reached OAuth start or instance creation")
			}
		})
	}
}

func TestOAuthStartNewNeverReplacesAnExistingAccount(t *testing.T) {
	for _, plantilla := range []string{"google-drive", "onedrive", "dropbox-storage", "slack"} {
		t.Run(plantilla, func(t *testing.T) {
			creadas, comienzos := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /api/v1/templates":
					_ = json.NewEncoder(w).Encode(map[string]any{"templates": []any{map[string]any{"key": plantilla, "enabled": true, "oauth_available": true, "manifest": map[string]any{"name": "Proveedor", "oauth": map[string]any{"authorize_url": "https://provider.example/authorize"}}}}})
				case "POST /api/v1/instances":
					creadas++
					var body struct {
						Template string            `json:"template_key"`
						Config   map[string]string `json:"config"`
					}
					if json.NewDecoder(r.Body).Decode(&body) != nil || body.Template != plantilla || body.Config == nil || len(body.Config) != 0 {
						t.Error("la cuenta nueva requiere secretos manuales o usa otra plantilla")
					}
					_, _ = w.Write([]byte(`{"id":"new-account"}`))
				case "POST /api/v1/instances/new-account/oauth/start":
					comienzos++
					_, _ = w.Write([]byte(`{"authorize_url":"https://provider.example/authorize?state=sealed-new"}`))
				default:
					t.Errorf("crear otra cuenta consulta o modifica una existente: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()
			flow, err := NewIntegraciones(server.URL, nil, server.Client()).OAuthStartNew(t.Context(), plantilla, "native-storage")
			if err != nil || flow.InstanceID != "new-account" || flow.State != "sealed-new" || creadas != 1 || comienzos != 1 {
				t.Fatalf("alta no independiente: %+v err=%v creadas=%d OAuth=%d", flow, err, creadas, comienzos)
			}
		})
	}
}

func TestOAuthNewDoesNotBorrowAnotherAccountsAppCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/templates" {
			t.Errorf("el alta sin cliente de plataforma intenta usar otra cuenta: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"templates":[{"key":"google-drive","enabled":true,"oauth_available":false,"manifest":{"name":"Drive","oauth":{"authorize_url":"https://accounts.google.com/auth"}}}]}`))
	}))
	defer server.Close()
	_, err := NewIntegraciones(server.URL, nil, server.Client()).OAuthStartNew(t.Context(), "google-drive", "native-storage")
	if !errors.Is(err, ErrOAuthUnavailable) {
		t.Fatalf("un cliente OAuth no registrado permite crear cuentas: %v", err)
	}
}

func TestOAuthDraftCanRetryWithoutCreatingOrReplacingAnotherAccount(t *testing.T) {
	creadas, inicios := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/templates":
			_, _ = w.Write([]byte(`{"templates":[{"key":"google-drive","enabled":true,"oauth_available":true,"manifest":{"name":"Drive","oauth":{"authorize_url":"https://accounts.google.com/auth"}}}]}`))
		case "GET /api/v1/instances":
			_, _ = w.Write([]byte(`{"instances":[{"id":"anterior","template_key":"google-drive","status":"active","has_secrets":true,"oauth_available":true},{"id":"borrador","template_key":"google-drive","status":"draft","oauth_available":true}]}`))
		case "POST /api/v1/instances":
			creadas++
			_, _ = w.Write([]byte(`{"id":"borrador"}`))
		case "POST /api/v1/instances/borrador/oauth/start":
			inicios++
			if inicios == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"authorize_url":"https://accounts.google.com/auth?state=segundo"}`))
		default:
			t.Errorf("el reintento consulta otra cuenta: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	client := NewIntegraciones(server.URL, nil, server.Client())
	fallido, err := client.OAuthStartNew(t.Context(), "google-drive", "native-storage")
	if err == nil || fallido.InstanceID != "borrador" {
		t.Fatal("el fallo pierde el ID de la cuenta en preparación")
	}
	reintento, err := client.OAuthStart(t.Context(), "google-drive", fallido.InstanceID, "native-storage")
	if err != nil || reintento.InstanceID != "borrador" || creadas != 1 || inicios != 2 {
		t.Fatalf("el reintento crea otra cuenta: %+v err=%v creadas=%d", reintento, err, creadas)
	}
}
