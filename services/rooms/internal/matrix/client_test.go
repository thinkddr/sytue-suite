// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

const (
	subject = "c2236155-0c17-4d4f-ad5b-6b657d44286a"
	roomID  = "!opaque:matrix.sytue.eu"
)

type identityGateFunc func(context.Context, string, func(context.Context) error) error

func (f identityGateFunc) WithMatrixIdentity(ctx context.Context, sub string, fn func(context.Context) error) error {
	return f(ctx, sub, fn)
}

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return New(Config{InternalURL: server.URL, HomeserverURL: "https://matrix.sytue.eu", ServerName: "matrix.sytue.eu", SSOProviderID: "oidc-sytue", AdminToken: "server-secret"})
}

func encode(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Error(err)
	}
}

func policyState(t *testing.T, c *Client, members map[string]string, archived bool) []map[string]any {
	t.Helper()
	events := make([]map[string]any, 0, 5+len(members))
	events = append(events,
		map[string]any{"type": "m.room.create", "sender": c.controller(), "content": map[string]any{"room_version": "12", "m.federate": false}},
		map[string]any{"type": "m.room.encryption", "content": map[string]string{"algorithm": domain.MatrixEncryption}},
		map[string]any{"type": "m.room.join_rules", "content": map[string]string{"join_rule": "invite"}},
		map[string]any{"type": "m.room.history_visibility", "content": map[string]string{"history_visibility": "joined"}},
		map[string]any{"type": "m.room.power_levels", "content": c.powerLevels(archived)},
	)
	for user, membership := range members {
		events = append(events, map[string]any{"type": "m.room.member", "state_key": user, "content": map[string]string{"membership": membership}})
	}
	return events
}

func TestEnsureUserRequiresIdentityGateEvenWhenCached(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer server-secret" {
			t.Error("missing admin credential")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, ok := body["password"]; ok {
			t.Error("server must not create user passwords")
		}
		if strings.Contains(r.URL.Path, "/devices") {
			t.Error("server must not create crypto devices")
		}
		encode(t, w, map[string]any{})
	})
	denied := false
	gates := 0
	c.SetIdentityGate(identityGateFunc(func(ctx context.Context, sub string, fn func(context.Context) error) error {
		gates++
		if denied {
			return domain.ErrForbidden
		}
		return fn(ctx)
	}))
	if _, err := c.EnsureUser(context.Background(), subject); err != nil {
		t.Fatal(err)
	}
	denied = true
	if _, err := c.EnsureUser(context.Background(), subject); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("cached identity bypassed denial: %v", err)
	}
	if calls != 1 || gates != 2 {
		t.Fatalf("remote=%d gate=%d", calls, gates)
	}
	if _, err := c.UserID("../other"); err == nil {
		t.Fatal("invalid subject accepted")
	}
}

func TestStableRoomReconcileRetainsMetadataController(t *testing.T) {
	var c *Client
	calls := 0
	c = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/state") {
			t.Fatalf("stable room mutated: %s %s", r.Method, r.URL.Path)
		}
		encode(t, w, map[string]any{"state": policyState(t, c, map[string]string{"@sytue_" + subject + ":matrix.sytue.eu": "join", c.controller(): "join"}, false)})
	})
	if err := c.Reconcile(context.Background(), roomID, []string{subject}, false, false); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestRevocationFailureKeepsControllerAndDoesNotEchoProviderBody(t *testing.T) {
	var c *Client
	calls := []string{}
	c = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/state"):
			encode(t, w, map[string]any{"state": policyState(t, c, map[string]string{"@removed:matrix.sytue.eu": "join", c.controller(): "join"}, false)})
		case strings.HasSuffix(r.URL.Path, "/kick"):
			http.Error(w, "provider secret body", http.StatusServiceUnavailable)
		default:
			encode(t, w, map[string]any{})
		}
	})
	err := c.Reconcile(context.Background(), roomID, nil, false, false)
	if !errors.Is(err, domain.ErrMatrixUnavailable) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe failure: %v", err)
	}
	if len(calls) != 2 || !strings.HasSuffix(calls[1], "/kick") {
		t.Fatalf("unexpected metadata controller lifecycle: %v", calls)
	}
}

func TestValidateEventRejectsPlaintextForeignRoomOrSender(t *testing.T) {
	for _, field := range []string{"valid", "type", "sender", "room_id", "algorithm", "state_key"} {
		t.Run(field, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/_synapse/admin/v1/rooms/") || r.URL.Query().Get("limit") != "0" {
					t.Fatalf("unsafe event query: %s", r.URL)
				}
				event := map[string]any{"type": "m.room.encrypted", "room_id": roomID, "event_id": "$event", "sender": "@sytue_" + subject + ":matrix.sytue.eu", "content": map[string]string{"algorithm": domain.MatrixEncryption}}
				switch field {
				case "type":
					event[field] = "m.room.message"
				case "sender":
					event[field] = "@other:matrix.sytue.eu"
				case "room_id":
					event[field] = "!foreign:matrix.sytue.eu"
				case "algorithm":
					event["content"] = map[string]string{"algorithm": "unknown"}
				case "state_key":
					event[field] = ""
				}
				encode(t, w, map[string]any{"event": event})
			})
			err := c.ValidateEvent(context.Background(), domain.MatrixEventRef{RoomID: roomID, EventID: "$event"}, subject)
			if field == "valid" && err != nil {
				t.Fatal(err)
			}
			if field != "valid" && !errors.Is(err, domain.ErrForbidden) {
				t.Fatalf("%s accepted: %v", field, err)
			}
		})
	}
}

func TestRoomVersionPowerLevelsAndCreators(t *testing.T) {
	c := testClient(t, func(http.ResponseWriter, *http.Request) {})
	for _, version := range []string{"11", "12"} {
		levels := c.powerLevels(false, version)
		raw, _ := json.Marshal(levels)
		if !c.validPowers(raw, false, version) {
			t.Fatalf("valid room %s policy rejected", version)
		}
		users := levels["users"].(map[string]int)
		users["@intruder:matrix.sytue.eu"] = 100
		raw, _ = json.Marshal(levels)
		if c.validPowers(raw, false, version) {
			t.Fatalf("foreign admin accepted in room %s", version)
		}
	}
	for _, tc := range []struct {
		name    string
		content map[string]any
	}{
		{"unexpected creator", map[string]any{"room_version": "12", "m.federate": false, "additional_creators": []string{"@intruder:matrix.sytue.eu"}}},
		{"federated", map[string]any{"room_version": "12", "m.federate": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var candidate *Client
			candidate = testClient(t, func(w http.ResponseWriter, _ *http.Request) {
				state := policyState(t, candidate, map[string]string{}, false)
				state[0]["content"] = tc.content
				encode(t, w, map[string]any{"state": state})
			})
			if _, err := candidate.roomState(context.Background(), roomID); err == nil {
				t.Fatal("unsafe immutable room policy accepted")
			}
		})
	}
}
