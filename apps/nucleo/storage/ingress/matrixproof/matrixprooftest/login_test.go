// SPDX-License-Identifier: Apache-2.0
package matrixprooftest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLoginProxyRejectsReplayAndNonDisposableOrigins(t *testing.T) {
	var logins atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_matrix/client/v3/login" {
			t.Error("unexpected fixture route")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		logins.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"user_id": "@synthetic:storage.test", "device_id": "DEVICE", "access_token": "synthetic-disposable-token"})
	}))
	defer upstream.Close()
	proxy, err := NewLoginProxy(t, LoginProxyConfig{SynapseURL: upstream.URL, UserID: "@synthetic:storage.test", Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"type":"m.login.token","token":"` + proxy.nonce + `","device_id":"DEVICE"}`
	for i, want := range []int{http.StatusOK, http.StatusUnauthorized} {
		response, err := proxy.Server.Client().Post(proxy.Server.URL+"/_matrix/client/v3/login", "application/json", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != want {
			t.Fatal("one-use login response", i, response.StatusCode, want)
		}
	}
	if logins.Load() != 1 {
		t.Fatal("replayed nonce reached Synapse", logins.Load())
	}
	if _, err := NewLoginProxy(t, LoginProxyConfig{SynapseURL: "https://rooms.example.com", UserID: "u", Password: "p"}); err == nil {
		t.Fatal("production-like origin accepted")
	}
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("loginToken") != proxy.nonce {
			t.Error("wrong callback nonce")
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer callback.Close()
	if err := proxy.OpenURL(t.Context(), "https://sso.invalid/?redirectUrl=https://example.com/matrix/callback/invalid"); err == nil {
		t.Fatal("non-loopback callback accepted")
	}
	if err := proxy.OpenURL(t.Context(), "https://sso.invalid/?redirectUrl="+callback.URL+"/matrix/callback/artificial"); err != nil {
		t.Fatal("loopback callback rejected", err)
	}
}
