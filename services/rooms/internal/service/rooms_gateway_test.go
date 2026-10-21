// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestRoomsOAuthGatewayKeepsIdentityAndSecretsInternal(t *testing.T) {
	instance := uuid.NewString()
	channel, message := uuid.New(), uuid.New()
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Sytue-Internal") != "internal-test" || r.Header.Get("Authorization") != "" {
			t.Error("internal authentication was not isolated")
		}
		switch r.URL.Path {
		case "/internal/v1/rooms/destinations":
			if r.URL.Query().Get("tenant") != "acme" || r.URL.Query().Get("instance") != instance {
				t.Error("wrong tenant/account")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []IntegrationDestination{{ID: "C123", Name: "General"}}})
		case "/internal/v1/rooms/messages":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["tenant"] != "acme" || body["instance"] != instance || body["remote_id"] != "C123" || body["channel_id"] != channel.String() || body["message_id"] != message.String() || body["text"] != "" || len(body) != 5 {
				t.Errorf("unexpected payload: %+v", body)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	svc := New(config.Config{PublicURL: "https://rooms.sytue.eu", IntegrationsURL: server.URL, IntegrationsInternalSecret: "internal-test"}, nil, nil, nil)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	for _, roles := range [][]string{nil, {"member"}} {
		if _, err := svc.IntegrationDestinations(authn.Into(ctx, authn.Claims{Subject: "bob", Roles: roles}), instance); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("member could enumerate destinations: %v", err)
		}
		member := authn.Into(ctx, authn.Claims{Subject: "bob", Roles: roles})
		if _, err := svc.CreateOAuthBridge(member, uuid.New(), "bob", domain.BridgeSlack, instance, "C123"); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("member could attach account: %v", err)
		}
		if _, err := svc.CreateIntegrationMeeting(member, instance, uuid.New(), "bob", "Bob"); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("member could create remote meeting: %v", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("unauthorized request reached the gateway")
	}
	destinations, err := svc.IntegrationDestinations(authn.Into(ctx, authn.Claims{Subject: "alice", Roles: []string{"owner"}}), instance)
	if err != nil || len(destinations) != 1 || destinations[0].ID != "C123" {
		t.Fatalf("destinations = %+v, %v", destinations, err)
	}
	if err := svc.sendBridge(ctx, domain.Bridge{Platform: domain.BridgeSlack, InstanceID: instance, RemoteID: "C123"}, bridgeMessage{ID: message, ChannelID: channel, AuthorName: "Ana", Body: "hola"}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestRoomsOAuthGatewayFailsClosedAndDoesNotFollowRedirects(t *testing.T) {
	var leaked atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	if err := newRoomsGateway(server.URL, "").send(ctx, uuid.NewString(), "C123", uuid.New(), uuid.New()); !errors.Is(err, ErrRoomsIntegrationUnavailable) {
		t.Fatalf("missing secret: %v", err)
	}
	if err := newRoomsGateway(server.URL, "internal-test").send(ctx, uuid.NewString(), "C123", uuid.New(), uuid.New()); !errors.Is(err, ErrRoomsIntegrationUnavailable) {
		t.Fatalf("redirect: %v", err)
	}
	if leaked.Load() != 0 {
		t.Fatal("the internal secret followed a redirect")
	}
}
