// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package matrix

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// This is metadata provisioning against a disposable server, not an SSO or
// client-crypto claim. Registration uses only an artificial fixture admin.
func TestStorageIdentityAgainstSynapse(t *testing.T) {
	if os.Getenv("SYTUE_STORAGE_METADATA_SYNAPSE") != "1" {
		t.Skip("requires disposable remote Synapse with synthetic registration")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	base := "http://127.0.0.1:18585"
	admin := storageIdentityFixtureAdmin(ctx, t, base)
	cfg := Config{InternalURL: base, HomeserverURL: "https://storage.test", ServerName: "storage.test", SSOProviderID: "oidc-sytue", AdminToken: admin}
	c := New(cfg)
	allowed := true
	gate := identityGateFunc(func(ctx context.Context, actor string, fn func(context.Context) error) error {
		if actor != subject || !allowed {
			return domain.ErrForbidden
		}
		return fn(ctx)
	})
	c.SetIdentityGate(gate)
	if err := c.Ready(ctx); err != nil {
		t.Fatal("fixture controller is unavailable", err)
	}
	human, err := c.EnsureUser(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	users := map[storage.Scope]string{}
	for _, tenant := range []string{"acme", "p-owner"} {
		for _, product := range []string{"drive", "mail"} {
			scope := storage.Scope{Tenant: tenant, Subject: subject, Product: product}
			mxid, err := c.EnsureStorageUser(tenancy.Into(ctx, tenancy.Tenant{Slug: tenant}), scope)
			if err != nil || mxid == human {
				t.Fatal("scoped fixture could not provision its own user", err)
			}
			users[scope] = mxid
		}
	}

	// Real private rooms must contain the isolated reader only; sharing a human
	// account would allow another product's device to request the same keys.
	scopedRooms := map[string]bool{}
	for scope, mxid := range users {
		scoped := tenancy.Into(ctx, tenancy.Tenant{Slug: scope.Tenant})
		room, err := c.EnsureIsolatedStorageRoom(scoped, scope, "")
		if err != nil || scopedRooms[room] {
			t.Fatal("isolated room creation or namespace failed", err)
		}
		scopedRooms[room] = true
		again, err := c.EnsureIsolatedStorageRoom(scoped, scope, "")
		if err != nil || again != room {
			t.Fatal("deterministic room did not survive retry", err)
		}
		policy, err := c.roomState(ctx, room)
		if err != nil || policy.Members[mxid] != "invite" || policy.Members[human] != "" || len(policy.Members) != 2 {
			t.Fatal("private room invited another identity", err)
		}
		if err := c.call(ctx, http.MethodPost, "/_matrix/client/v3/rooms/"+url.PathEscape(room)+"/invite", map[string]string{"user_id": human}, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := c.EnsureIsolatedStorageRoom(scoped, scope, room); err != nil {
			t.Fatal(err)
		}
		policy, err = c.roomState(ctx, room)
		if err != nil || policy.Members[human] != "leave" {
			t.Fatal("foreign reader survived reconciliation", err)
		}
	}
	type remoteUser struct {
		Name        string `json:"name"`
		Admin       *bool  `json:"admin"`
		Suspended   bool   `json:"suspended"`
		Display     string `json:"displayname"`
		ExternalIDs []struct {
			Provider string `json:"auth_provider"`
			External string `json:"external_id"`
		} `json:"external_ids"`
	}
	read := func(mxid string) remoteUser {
		t.Helper()
		var value remoteUser
		if err := c.call(ctx, http.MethodGet, "/_synapse/admin/v2/users/"+url.PathEscape(mxid), nil, &value); err != nil {
			t.Fatal("fixture metadata read failed", err)
		}
		return value
	}
	for scope, mxid := range users {
		value := read(mxid)
		if value.Admin == nil || *value.Admin || value.Name != mxid || value.Suspended || len(value.ExternalIDs) != 1 || value.ExternalIDs[0].Provider != "oidc-sytue-storage-"+scope.Product || value.ExternalIDs[0].External != scope.Tenant+"|"+subject {
			t.Fatal("real Synapse stored an unexpected SSO binding")
		}
	}
	if err = c.SuspendStorageUsers(ctx, subject, "acme"); err != nil {
		t.Fatal("scoped suspension failed", err)
	}
	for scope, mxid := range users {
		if read(mxid).Suspended != (scope.Tenant == "acme") {
			t.Fatal("suspension crossed organizations or missed a product")
		}
	}
	if read(human).Suspended {
		t.Fatal("scoped suspension touched the legacy human account")
	}
	drive := storage.Scope{Tenant: "acme", Subject: subject, Product: "drive"}
	if _, err = c.EnsureStorageUser(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), drive); err != nil {
		t.Fatal(err)
	}
	if read(users[drive]).Suspended {
		t.Fatal("authorized product did not resume")
	}
	mail := storage.Scope{Tenant: "acme", Subject: subject, Product: "mail"}
	if !read(users[mail]).Suspended {
		t.Fatal("resuming Drive also resumed Mail")
	}
	allowed = false
	if _, err = c.EnsureStorageUser(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), mail); !errors.Is(err, domain.ErrForbidden) || !read(users[mail]).Suspended {
		t.Fatal("cached user bypassed lifecycle denial", err)
	}
	allowed = true
	// A pre-existing namespace is inspected, never taken over by an upsert.
	foreign := []map[string]string{{"auth_provider": "oidc-foreign", "external_id": "another-person"}}
	if err = c.call(ctx, http.MethodPut, "/_synapse/admin/v2/users/"+url.PathEscape(users[mail]), map[string]any{"external_ids": foreign, "logout_devices": false, "displayname": "preserve this fixture"}, nil); err != nil {
		t.Fatal(err)
	}
	cold := New(cfg)
	cold.SetIdentityGate(gate)
	if _, err = cold.EnsureStorageUser(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), mail); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("foreign SSO identity was adopted", err)
	}
	value := read(users[mail])
	if value.Display != "preserve this fixture" || len(value.ExternalIDs) != 1 || value.ExternalIDs[0].Provider != "oidc-foreign" {
		t.Fatal("foreign SSO binding or profile was overwritten")
	}
	t.Log("real Synapse: four product/tenant accounts, legacy preserved, scoped suspension/reactivation, fresh gate and takeover denial verified")
}

func storageIdentityFixtureAdmin(ctx context.Context, t *testing.T, base string) string {
	t.Helper()
	request := func(method, path string, body, out any) {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("synthetic fixture registration failed", err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode < 200 || response.StatusCode >= 300 || json.NewDecoder(response.Body).Decode(out) != nil {
			t.Fatal("synthetic fixture registration returned an invalid response")
		}
	}
	var challenge struct {
		Nonce string `json:"nonce"`
	}
	request(http.MethodGet, "/_synapse/admin/v1/register", nil, &challenge)
	username, password := "rooms_controller", "artificial-identity-admin-password"
	// Synapse's shared-secret registration protocol requires HMAC-SHA1. This
	// is fixture setup only, never application content encryption.
	mac := hmac.New(sha1.New, []byte("artificial-storage-registration"))
	_, _ = mac.Write([]byte(challenge.Nonce + "\x00" + username + "\x00" + password + "\x00admin"))
	var registered struct {
		Token string `json:"access_token"`
	}
	request(http.MethodPost, "/_synapse/admin/v1/register", map[string]any{"nonce": challenge.Nonce, "username": username, "password": password, "admin": true, "mac": hex.EncodeToString(mac.Sum(nil))}, &registered)
	if registered.Token == "" {
		t.Fatal("fixture admin did not receive a token")
	}
	return registered.Token
}
