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
