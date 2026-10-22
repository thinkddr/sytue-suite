// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEncryptedCallBindsJoinDeviceEpochAndNonce(t *testing.T) {
	var ready bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/rooms/from-source":
			_, _ = w.Write([]byte(`{"id":"call"}`))
		case "POST /api/v1/rooms/call/join":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["matrix_device_id"] != "DEVICE" || in["display_name"] != "Alice" {
				t.Errorf("join lacks authenticated device: %+v", in)
			}
			_, _ = w.Write([]byte(`{"room":{"id":"call"},"join_id":"nonce/+?","expires_at_ms":1792144920000,"token":"receive-only-token","encryption":{"call_id":"call","matrix_room_id":"!room:matrix.test","livekit_room":"call-epoch-7","generation":7,"expires_at_ms":1792144920000,"leader_subject":"alice","leader_device_id":"DEVICE","members":[{"subject":"alice","device_id":"DEVICE"}]}}`))
		case "GET /api/v1/rooms/call/encryption":
			if r.URL.Query().Get("matrix_device_id") != "DEVICE" || r.URL.Query().Get("join_id") != "nonce/+?" {
				t.Error("binding lost in read")
			}
			_, _ = w.Write([]byte(`{"call_id":"call","matrix_room_id":"!room:matrix.test","livekit_room":"call-epoch-8","generation":8,"expires_at_ms":1792144920000,"members":[{"subject":"alice","device_id":"DEVICE"}]}`))
		case "POST /api/v1/rooms/call/encryption/ready", "POST /api/v1/rooms/call/encryption/reconnect":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if len(in) != 3 || in["matrix_device_id"] != "DEVICE" || in["join_id"] != "nonce/+?" || in["generation"] != float64(8) {
				t.Errorf("stale/omitted binding: %+v", in)
			}
			if r.URL.Path == "/api/v1/rooms/call/encryption/ready" {
				ready = true
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if !ready {
				t.Error("reconnect before key acknowledgement")
			}
			_, _ = w.Write([]byte(`{"room":{"id":"call"},"join_id":"nonce/+?","token":"new-epoch-token","encryption":{"generation":8,"livekit_room":"call-epoch-8"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	client := NewRooms(srv.URL, nil, nil)
	ctx := context.Background()
	joined, err := client.CallChannelDevice(ctx, "channel", "General", "Alice", "DEVICE")
	if err != nil || joined.JoinID != "nonce/+?" || joined.ExpiresAtMS != 1792144920000 || joined.Encryption == nil || joined.Encryption.Generation != 7 || joined.Encryption.LeaderDeviceID != "DEVICE" || len(joined.Encryption.Members) != 1 {
		t.Fatalf("join binding: %+v %v", joined, err)
	}
	epoch, err := client.CallEncryption(ctx, "call", "DEVICE", joined.JoinID)
	if err != nil || epoch.Generation != 8 || epoch.LiveKitRoom != "call-epoch-8" {
		t.Fatalf("rotated epoch: %+v %v", epoch, err)
	}
	if err = client.CallEncryptionReady(ctx, "call", "DEVICE", joined.JoinID, epoch.Generation); err != nil {
		t.Fatal(err)
	}
	reconnected, err := client.CallEncryptionReconnect(ctx, "call", "DEVICE", joined.JoinID, epoch.Generation)
	if err != nil || reconnected.Token != "new-epoch-token" || reconnected.JoinID != joined.JoinID || reconnected.Encryption == nil || reconnected.Encryption.Generation != epoch.Generation {
		t.Fatalf("reconnect binding: %+v %v", reconnected, err)
	}
}
