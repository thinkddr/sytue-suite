// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type currentResourceReaderFence struct {
	t       *testing.T
	binding resource.SessionBinding
	action  resource.Action
	calls   int
}

func (f *currentResourceReaderFence) Revalidate(ctx context.Context, access resource.VerifiedAccess) error {
	f.calls++
	binding, ok := access.CurrentSessionBinding()
	if !ok || binding != f.binding || access.Actor() != f.binding.Actor() || access.Action() != f.action {
		f.t.Fatal("current B authority discarded by ACK reader")
	}
	return ctx.Err()
}

// Registry lifecycle is an artificial witness; signatures/enrollment/Provider
// and the HTTP SDK client are real. PostgreSQL+Synapse have separate fixtures.
func TestResourceAcceptanceReaderKeepsCurrentDeviceAndRevalidatesAfterHTTP(t *testing.T) {
	for _, action := range []resource.Action{resource.Read, resource.Manage} {
		t.Run(string(action), func(t *testing.T) {
			for _, phase := range []string{"active", "retired-before", "retired-during"} {
				t.Run(phase, func(t *testing.T) {
					scope := storage.Scope{Tenant: "fixture", Subject: uuid.NewString(), Product: "docs"}
					user := id.UserID("@sytue_" + scope.Subject + ":matrix.test")
					chain := matrixprooftest.NewUser(t, scope, "matrix.test", user, "ORIGINAL_A")
					raw, err := json.Marshal(&event.EncryptedEventContent{Algorithm: id.AlgorithmMegolmV1, SessionID: id.SessionID(chain.Master.PublicKey()), SenderKey: id.Curve25519(chain.Self.PublicKey()), DeviceID: chain.Origin.DeviceID, MegolmCiphertext: []byte("opaque synthetic ciphertext")}) //nolint:staticcheck // SA1019: Megolm wire payload fixture
					if err != nil {
						t.Fatal(err)
					}
					fixture := resourcetest.NewWithSDK(t, false, chain.Origin, chain.Master.PublicKey(), func(value any) signatures.Signatures {
						return matrixprooftest.Sign(t, chain.Device, value, user, "ORIGINAL_A")
					}, "!resource:matrix.test", raw)
					b := matrixprooftest.NewDevice(t, chain, "CURRENT_B")
					key, err := pqc.GenerateIdentity()
					if err != nil {
						t.Fatal(err)
					}
					current, registry := resourcetest.EnrolledSession(t, b, key)
					access, err := resource.AuthorizeSession(t.Context(), fixture.Epoch, fixture.Cap, current, action)
					if err != nil {
						t.Fatal(err)
					}
					var calls atomic.Int64
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/event/") || r.URL.Query().Get("user_id") != user.String() || r.URL.Query().Get("device_id") != "" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("A", 32) {
							t.Error("ACK reader changed immutable account or asserted old producer")
						}
						if phase == "retired-during" {
							registry.Retire()
						}
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(event.Event{ID: id.EventID("$accepted"), RoomID: id.RoomID("!resource:matrix.test"), Sender: user, Type: event.EventEncrypted, Content: event.Content{VeryRaw: raw}})
					}))
					defer server.Close()
					relay, err := NewResourceRelay(ResourceRelayConfig{InternalURL: server.URL, ServerName: "matrix.test", ASToken: strings.Repeat("A", 32)}, server.Client())
					if err != nil {
						t.Fatal(err)
					}
					fence := &currentResourceReaderFence{t: t, binding: current.Binding(), action: action}
					source, err := relay.AcceptanceReader(access, fence)
					if err != nil {
						t.Fatal(err)
					}
					if phase == "retired-before" {
						registry.Retire()
					}
					value, err := source.AcceptedEvent(t.Context(), storage.Reference{RoomID: "!resource:matrix.test", EventID: "$accepted"})
					if phase == "active" {
						if err != nil || value == nil || value.ID != "$accepted" || calls.Load() != 1 || fence.calls != 2 {
							t.Fatal("current authorized read failed", err, calls.Load(), fence.calls)
						}
					} else {
						if err == nil || value != nil {
							t.Fatal("retired current reader returned event")
						}
						if phase == "retired-before" && calls.Load() != 0 {
							t.Fatal("withdrawn reader reached HTTP")
						}
					}
				})
			}
		})
	}
}
