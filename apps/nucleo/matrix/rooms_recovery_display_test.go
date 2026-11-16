// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestUnreadableRoomsMessageClearsPrivateContentAndClassifiesRecovery(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		recovery bool
	}{
		{"old device required", errors.Join(ErrUnverified, ErrSessionProvenanceMissing), true},
		{"other failure", ErrUnverified, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := api.RoomsMessage{ID: "retained", Body: "private", Priority: "urgent", RequiresAck: true, Mentions: []string{"private"}, Attachments: []api.RoomsAttachment{{Name: "private"}}, Card: &api.RoomsCard{}, IdentityVerified: true, AuthenticityVerified: true, RequiresOriginalDevice: true}
			got := unreadableMessage(original, tc.err)
			if !got.EncryptionError || got.RequiresOriginalDevice != tc.recovery || got.IdentityVerified || got.AuthenticityVerified {
				t.Fatal("incorrect local trust or recovery state")
			}
			if got.Body != "" || got.Priority != "" || got.RequiresAck || got.Card != nil || len(got.Mentions) != 0 || len(got.Attachments) != 0 {
				t.Fatal("unreadable message retained private content")
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var public map[string]any
			if err = json.Unmarshal(raw, &public); err != nil {
				t.Fatal(err)
			}
			if _, ok := public["RequiresOriginalDevice"]; ok {
				t.Fatal("local recovery state leaked into metadata API")
			}
		})
	}
}

func TestRoomsServiceCannotUsePrivateStorageClient(t *testing.T) {
	c := &Client{cfg: Config{Bootstrap: Bootstrap{StorageProduct: "drive"}}}
	service := NewRoomsService(nil, func(context.Context) (*Client, error) { return c, nil })
	if _, _, err := service.connection(t.Context(), "channel"); !errors.Is(err, ErrIdentity) {
		t.Fatalf("storage client crossed Rooms trust boundary: %v", err)
	}
	if _, _, err := decodeDecryptedMessage(api.RoomsMessage{}, c, DecryptedEvent{}); !errors.Is(err, ErrIdentity) {
		t.Fatalf("storage event was accepted as Rooms provenance: %v", err)
	}
}
