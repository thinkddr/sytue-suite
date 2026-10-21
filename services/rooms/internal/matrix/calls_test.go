// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestCallDeviceMustBelongToActorAndNotBeDehydrated(t *testing.T) {
	for _, tc := range []struct {
		name, user string
		dehydrated bool
		allowed    bool
	}{{"owned", "@sytue_" + subject + ":matrix.sytue.eu", false, true}, {"foreign", "@other:matrix.sytue.eu", false, false}, {"dehydrated", "@sytue_" + subject + ":matrix.sytue.eu", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/devices") || !strings.Contains(r.URL.Path, "sytue_"+subject) {
					t.Fatalf("unexpected device query: %s %s", r.Method, r.URL.Path)
				}
				encode(t, w, map[string]any{"devices": []map[string]any{{"device_id": "DEVICE", "user_id": tc.user, "dehydrated": tc.dehydrated}}})
			})
			err := c.ValidateDevice(context.Background(), subject, "DEVICE")
			if (err == nil) != tc.allowed {
				t.Fatalf("device validation: %v", err)
			}
			if err := c.ValidateDevice(context.Background(), subject, "MISSING"); !errors.Is(err, domain.ErrForbidden) {
				t.Fatalf("unregistered device allowed: %v", err)
			}
		})
	}
}

func TestCallRoomRetryUsesStableAliasAndMetadataController(t *testing.T) {
	var c *Client
	created, joined, creates := false, false, 0
	call := uuid.New()
	alias := "calls_acme_" + call.String() + "_7"
	c = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/directory/room/"):
			if !strings.Contains(r.URL.Path, alias) {
				t.Fatalf("alias did not pin call/generation: %s", r.URL.Path)
			}
			if !created {
				http.Error(w, `{"errcode":"M_NOT_FOUND"}`, http.StatusNotFound)
				return
			}
			encode(t, w, map[string]string{"room_id": roomID})
		case strings.HasSuffix(r.URL.Path, "/createRoom"):
			creates++
			created = true
			joined = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["room_alias_name"] != alias {
				t.Fatalf("unstable alias: %+v", body)
			}
			encode(t, w, map[string]string{"room_id": roomID})
		case strings.HasSuffix(r.URL.Path, "/state"):
			membership := "leave"
			if joined {
				membership = "join"
			}
			encode(t, w, map[string]any{"state": policyState(t, c, map[string]string{c.controller(): membership}, false)})
		case strings.HasSuffix(r.URL.Path, "/leave"):
			t.Fatal("metadata controller left the room and made future ACL changes impossible")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	for range 2 {
		room, err := c.EnsureCallRoom(context.Background(), "acme", call, 7, nil)
		if err != nil || room != roomID || !joined {
			t.Fatalf("call room not safely acknowledged: %s %v joined=%v", room, err, joined)
		}
	}
	if creates != 1 {
		t.Fatalf("retry created %d rooms", creates)
	}
}
