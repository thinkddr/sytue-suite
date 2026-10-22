// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientInviteForwardsAdministratorToken(t *testing.T) {
	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/invitations" || r.Header.Get("Authorization") != "Bearer user-jwt" {
			t.Fatalf("request path=%q auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["email"] != "new@example.eu" || body["role"] != "member" {
			t.Fatalf("body=%v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"expires_at": expires})
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL).Invite(context.Background(), "user-jwt", "new@example.eu", "member")
	if err != nil || !got.Equal(expires) {
		t.Fatalf("expires=%v err=%v", got, err)
	}
}

func TestClientResolveNotFoundIsNormal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, found, err := NewClient(srv.URL).ResolveMember(context.Background(), "jwt", "none@example.eu")
	if err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}
