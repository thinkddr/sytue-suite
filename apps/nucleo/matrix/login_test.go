// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSSOForeignIdentityIsRevokedBeforePrivateStorage(t *testing.T) {
	var logouts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/login") && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"flows":[{"type":"m.login.sso"},{"type":"m.login.token"}]}`)
		case strings.HasSuffix(r.URL.Path, "/login") && r.Method == http.MethodPost:
			var request map[string]any
			_ = json.NewDecoder(r.Body).Decode(&request)
			if request["token"] != "one-use-fixture-token" || request["type"] != "m.login.token" {
				t.Error("did not use maintained SSO token login")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "@other:matrix.test", "device_id": request["device_id"], "access_token": "wrong-account-fixture"})
		case strings.HasSuffix(r.URL.Path, "/logout"):
			logouts.Add(1)
			_, _ = io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	cfg := configFor(t, server)
	_, err := Login(t.Context(), cfg, func(target string) error {
		u, parseErr := url.Parse(target)
		if parseErr != nil {
			return parseErr
		}
		if !strings.HasSuffix(u.Path, "/login/sso/redirect") {
			t.Error("wrong SSO route")
		}
		callback, parseErr := url.Parse(u.Query().Get("redirectUrl"))
		if parseErr != nil {
			return parseErr
		}
		if callback.Hostname() != "127.0.0.1" || !strings.HasPrefix(callback.Path, "/matrix/callback/") || len(strings.TrimPrefix(callback.Path, "/matrix/callback/")) < 20 {
			t.Fatal("callback was not private loopback with a random binding")
		}
		q := callback.Query()
		q.Set("loginToken", "one-use-fixture-token")
		callback.RawQuery = q.Encode()
		response, requestErr := http.Get(callback.String())
		if requestErr != nil {
			return requestErr
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil
	})
	if !errors.Is(err, ErrIdentity) || logouts.Load() != 1 {
		t.Fatalf("foreign identity err=%v revoked=%d", err, logouts.Load())
	}
	if len(cfg.Secrets.(*memorySecrets).values) != 0 {
		t.Fatal("foreign SSO account reached private storage")
	}
}

func TestSSOStaleSessionWithoutDatabaseEnrollsANewDevice(t *testing.T) {
	var sent atomic.Value
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/login") && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"flows":[{"type":"m.login.sso"},{"type":"m.login.token"}]}`)
		case strings.HasSuffix(r.URL.Path, "/login") && r.Method == http.MethodPost:
			var request map[string]any
			_ = json.NewDecoder(r.Body).Decode(&request)
			sent.Store(request["device_id"])
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "@other:matrix.test", "device_id": request["device_id"], "access_token": "x"})
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer server.Close()
	cfg := configFor(t, server)
	probe, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	stale, _ := json.Marshal(credentials{Homeserver: probe.cfg.Bootstrap.HomeserverURL, UserID: "@user:matrix.test", DeviceID: "OLDDEVICE", AccessToken: "old", Subject: cfg.Subject, Tenant: cfg.Tenant})
	if err = cfg.Secrets.Write(probe.sessionKey(), stale); err != nil {
		t.Fatal(err)
	}
	_ = probe.Close()
	_, _ = Login(t.Context(), cfg, func(target string) error {
		u, _ := url.Parse(target)
		callback, _ := url.Parse(u.Query().Get("redirectUrl"))
		q := callback.Query()
		q.Set("loginToken", "t")
		callback.RawQuery = q.Encode()
		response, requestErr := http.Get(callback.String())
		if requestErr == nil {
			_ = response.Body.Close()
		}
		return requestErr
	})
	if got, _ := sent.Load().(string); got == "" || got == "OLDDEVICE" {
		t.Fatalf("device reused without its database: %q", got)
	}
}
