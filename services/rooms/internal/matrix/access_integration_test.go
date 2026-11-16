// SPDX-License-Identifier: Apache-2.0
//go:build integration

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Network access policy against an unmodified disposable Synapse. Recovery is
// opaque synthetic account data and backup metadata: this is not an SDK crypto
// recovery proof. All passwords and tokens here belong only to the fixture.
func TestStorageAccessLifecycleAgainstSynapse(t *testing.T) {
	if os.Getenv("SYTUE_STORAGE_METADATA_SYNAPSE") != "1" {
		t.Skip("requires disposable Synapse and artificial fixture accounts")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	base := "http://127.0.0.1:18585"
	admin := storageIdentityFixtureAdmin(ctx, t, base)
	c := New(Config{InternalURL: base, HomeserverURL: "https://storage.test", ServerName: "storage.test", SSOProviderID: "oidc-sytue", AdminToken: admin})
	global, acmeAllowed := true, true
	c.SetIdentityGate(identityGateFunc(func(ctx context.Context, actor string, fn func(context.Context) error) error {
		tenant, _ := tenancy.From(ctx)
		if actor != subject || !global || tenant.Slug == "acme" && !acmeAllowed {
			return domain.ErrForbidden
		}
		return fn(ctx)
	}))
	api := func(token, method, path string, body, out any) int {
		t.Helper()
		var raw []byte
		if body != nil {
			var err error
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("fixture HTTP failed", err)
		}
		defer func() { _ = response.Body.Close() }()
		if out != nil {
			if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out); err != nil {
				t.Fatal("invalid fixture JSON", response.StatusCode, err)
			}
		} else {
			_, _ = io.Copy(io.Discard, response.Body)
		}
		return response.StatusCode
	}
	human, err := c.EnsureUser(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	scopes := []storage.Scope{{Tenant: "acme", Subject: subject, Product: "drive"}, {Tenant: "acme", Subject: subject, Product: "mail"}, {Tenant: "beta", Subject: subject, Product: "drive"}}
	users := make([]string, 1, 1+len(scopes))
	users[0] = human
	for _, scope := range scopes {
		user, err := c.EnsureStorageUser(tenancy.Into(ctx, tenancy.Tenant{Slug: scope.Tenant}), scope)
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, user)
	}
	type session struct {
		Token   string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Device  string `json:"device_id"`
	}
	login := func(mxid string) session {
		t.Helper()
		var out session
		if status := api("", http.MethodPost, "/_matrix/client/v3/login", map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": mxid}, "password": "artificial-lifecycle-password", "refresh_token": true}, &out); status != http.StatusOK || out.Token == "" || out.Device == "" || out.Refresh == "" {
			t.Fatal("fixture login did not produce a refreshable device", status)
		}
		return out
	}
	sessions := map[string]session{}
	backups := map[string]string{}
	for _, mxid := range users {
		// Test setup only: product code never assigns user passwords or mints sessions.
		if err := c.call(ctx, http.MethodPut, "/_synapse/admin/v2/users/"+url.PathEscape(mxid), map[string]any{"password": "artificial-lifecycle-password", "logout_devices": false}, nil); err != nil {
			t.Fatal(err)
		}
		current := login(mxid)
		sessions[mxid] = current
		account := "/_matrix/client/v3/user/" + url.PathEscape(mxid) + "/account_data/m.secret_storage.default_key"
		if status := api(current.Token, http.MethodPut, account, map[string]string{"key": "artificial-opaque-recovery"}, nil); status != http.StatusOK {
			t.Fatal("fixture SSSS metadata could not be stored", status)
		}
		var backup struct {
			Version string `json:"version"`
		}
		if status := api(current.Token, http.MethodPost, "/_matrix/client/v3/room_keys/version", map[string]any{"algorithm": "m.megolm_backup.v1.curve25519-aes-sha2", "auth_data": map[string]string{"public_key": strings.Repeat("A", 43)}}, &backup); status != http.StatusOK || backup.Version == "" {
			t.Fatal("fixture backup could not be created", status)
		}
		backups[mxid] = backup.Version
	}
	checkRevoked := func(mxid string) {
		t.Helper()
		old := sessions[mxid]
		if status := api(old.Token, http.MethodGet, "/_matrix/client/v3/account/whoami", nil, nil); status != http.StatusUnauthorized {
			t.Fatal("old token retained network read access", status)
		}
		if status := api("", http.MethodPost, "/_matrix/client/v3/refresh", map[string]string{"refresh_token": old.Refresh}, nil); status != http.StatusUnauthorized {
			t.Fatal("old refresh token did not return a revocation error", status)
		}
		if status := api("", http.MethodPost, "/_matrix/client/v3/login", map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": mxid}, "password": "artificial-lifecycle-password"}, nil); status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Fatal("locked account did not reject login", status)
		}
		var devices struct {
			Devices []json.RawMessage `json:"devices"`
			Total   int               `json:"total"`
		}
		if err := c.call(ctx, http.MethodGet, "/_synapse/admin/v2/users/"+url.PathEscape(mxid)+"/devices", nil, &devices); err != nil || len(devices.Devices) != 0 || devices.Total != 0 {
			t.Fatal("device retirement was incomplete", err)
		}
	}
	acmeAllowed = false
	// Fail after the first account is locked, before its devices are retired.
	// A retry must still enumerate that locked account and invalidate its tokens.
	upstream, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	var failOnce atomic.Bool
	failOnce.Store(true)
	faulty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/delete_devices") && failOnce.CompareAndSwap(true, false) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer faulty.Close()
	faultyConfig := c.cfg
	faultyConfig.InternalURL = faulty.URL
	faultyController := New(faultyConfig)
	if err := faultyController.RevokeStorageAccess(ctx, subject, "acme"); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatal("partial device retirement was acknowledged", err)
	}
	if failOnce.Load() {
		t.Fatal("fixture never reached partial withdrawal")
	}
	if err := c.RevokeStorageAccess(ctx, subject, "acme"); err != nil {
		t.Fatal(err)
	}
	for _, mxid := range users[1:3] {
		checkRevoked(mxid)
	}
	for _, mxid := range []string{human, users[3]} {
		if status := api(sessions[mxid].Token, http.MethodGet, "/_matrix/client/v3/account/whoami", nil, nil); status != http.StatusOK {
			t.Fatal("tenant removal revoked another scope", status)
		}
	}
	if _, err := c.EnsureStorageUser(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), scopes[0]); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("cached scope bypassed denial", err)
	}
	global = false
	if err := c.SetAccountAccess(ctx, subject, "suspended"); err != nil {
		t.Fatal(err)
	}
	for _, mxid := range users {
		checkRevoked(mxid)
	}
	// Reactivation of the person does not implicitly restore a removed tenant.
	global = true
	if err := c.SetAccountAccess(ctx, subject, "active"); err != nil {
		t.Fatal(err)
	}
	for _, mxid := range users[1:] {
		checkRevoked(mxid)
	}
	if _, err := c.EnsureStorageUser(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), scopes[0]); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("global activation reopened removed tenant", err)
	}
	acmeAllowed = true
	for i, mxid := range users {
		if i > 0 {
			scope := scopes[i-1]
			if _, err := c.EnsureStorageUser(tenancy.Into(ctx, tenancy.Tenant{Slug: scope.Tenant}), scope); err != nil {
				t.Fatal(err)
			}
		}
		current := login(mxid)
		var recovery struct {
			Key string `json:"key"`
		}
		if status := api(current.Token, http.MethodGet, "/_matrix/client/v3/user/"+url.PathEscape(mxid)+"/account_data/m.secret_storage.default_key", nil, &recovery); status != http.StatusOK || recovery.Key != "artificial-opaque-recovery" {
			t.Fatal("reactivation lost recovery metadata", status)
		}
		var backup struct {
			Version string `json:"version"`
		}
		if status := api(current.Token, http.MethodGet, "/_matrix/client/v3/room_keys/version", nil, &backup); status != http.StatusOK || backup.Version != backups[mxid] {
			t.Fatal("reactivation lost encrypted backup metadata", status)
		}
	}
	t.Log("real Synapse: token/refresh retirement, locked login, tenant isolation and opaque recovery/backup preservation verified")
}
