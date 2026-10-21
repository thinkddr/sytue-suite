// SPDX-License-Identifier: Apache-2.0
//go:build integration

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
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Real migrations/RLS, local permissions before provider side effects, and outbox delivery.
func TestRoomsOAuthBridgeAndMeetingLifecycle(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	var calls, sends, meetings atomic.Int64
	var meetingURL atomic.Value
	meetingURL.Store("https://meet.google.com/abc-defg-hij")
	instance := uuid.NewString()
	remote := "spaces/AAAA"
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Sytue-Internal") != "internal-test" {
			t.Error("wrong internal secret")
		}
		switch r.URL.Path {
		case "/internal/v1/rooms/destinations":
			if r.URL.Query().Get("tenant") != "acme" {
				t.Error("tenant was not derived from Rooms context")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []IntegrationDestination{{ID: remote, Name: "Project chat"}}})
		case "/internal/v1/rooms/messages":
			sends.Add(1)
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["tenant"] != "acme" || body["instance"] != instance || body["remote_id"] != remote || body["channel_id"] == "" || body["text"] != "" {
				t.Errorf("bad delivery scope: %+v", body)
			}
			w.WriteHeader(http.StatusNoContent)
		case "/internal/v1/rooms/meetings":
			meetings.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"url": meetingURL.Load().(string)})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gateway.Close()
	svc := New(config.Config{PublicURL: "https://rooms.sytue.eu", MaxMessageBytes: 4096, MessagePageSize: 50, IntegrationsURL: gateway.URL, IntegrationsInternalSecret: "internal-test"}, st, nil, nil)
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Subject: "alice", Tenant: "acme", Roles: []string{"owner"}})
	teams, err := svc.EnsureDefaultTeams(ctx, "alice", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	ch, err := svc.CreateChannel(ctx, CreateChannelInput{TeamID: teams[0].ID, Name: "OAuth project", Kind: domain.ChannelPrivate, Creator: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddTeamMember(ctx, teams[0].ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateOAuthBridge(ctx, ch.ID, "bob", domain.BridgeGoogleChat, instance, remote); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("member bridge: %v", err)
	}
	if _, err := svc.CreateIntegrationMeeting(ctx, instance, ch.ID, "bob", "Bob"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("member meeting: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("provider was called before checking local administration")
	}
	bridge, err := svc.CreateOAuthBridge(ctx, ch.ID, "alice", domain.BridgeGoogleChat, instance, remote)
	if err != nil {
		t.Fatal(err)
	}
	if bridge.InstanceID != instance || bridge.RemoteID != remote || bridge.Target != "Project chat" || bridge.WebhookURL != "" {
		t.Fatalf("bridge metadata lost: %+v", bridge)
	}
	got, err := st.BridgesForChannel(ctx, ch.ID)
	if err != nil || len(got) != 1 || got[0].InstanceID != instance || got[0].RemoteID != remote {
		t.Fatalf("persisted bridge = %+v, %v", got, err)
	}
	other := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "other"})
	if got, err := st.BridgesForChannel(other, ch.ID); err != nil || len(got) != 0 {
		t.Fatalf("cross-tenant bridge: %+v, %v", got, err)
	}
	payload, err := json.Marshal(eventEnvelope{ChannelID: ch.ID, Data: bridgeMessage{ID: uuid.New(), ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "Project update"}})
	if err != nil {
		t.Fatal(err)
	}
	svc.deliverBridgesForEvent(context.Background(), events.Event{Channel: EventChannel, Type: EventMessageCreated, Tenant: "acme", Payload: payload})
	if sends.Load() != 2 {
		t.Fatalf("expected probe and outbox send, got %d", sends.Load())
	}
	meeting, err := svc.CreateIntegrationMeeting(ctx, instance, ch.ID, "alice", "Alice")
	if err != nil || meeting != "https://meet.google.com/abc-defg-hij" {
		t.Fatalf("meeting = %q, %v", meeting, err)
	}
	messages, err := st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 20})
	if err != nil || len(messages) != 0 {
		t.Fatalf("meeting was inserted as plaintext: %+v, %v", messages, err)
	}
	meetingURL.Store("https://meet.google.com.attacker.invalid/abc")
	if _, err := svc.CreateIntegrationMeeting(ctx, instance, ch.ID, "alice", "Alice"); !errors.Is(err, ErrRoomsIntegrationUnavailable) {
		t.Fatalf("accepted an unsafe provider URL: %v", err)
	}
	messages, err = st.ListMessages(ctx, ch.ID, "alice", store.MessagePage{Limit: 20})
	if err != nil || len(messages) != 0 {
		t.Fatalf("provider URL was shared as plaintext: %+v, %v", messages, err)
	}
	if meetings.Load() != 2 {
		t.Fatalf("meetings = %d", meetings.Load())
	}
	if err := svc.DeleteBridge(ctx, bridge.ID, "alice"); err != nil {
		t.Fatal(err)
	}
}
