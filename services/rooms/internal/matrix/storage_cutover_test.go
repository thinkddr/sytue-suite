// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestValidateStorageReceiptRequiresIsolatedProductSender(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: subject, Product: "drive"}
	for _, fault := range []string{"valid", "human", "tenant", "product", "room", "event", "plain", "algorithm", "state"} {
		t.Run(fault, func(t *testing.T) {
			var client *Client
			client = testClient(t, func(w http.ResponseWriter, _ *http.Request) {
				owner := storage.IsolatedMatrixUserPrefix(scope) + "matrix.sytue.eu"
				value := map[string]any{"type": "m.room.encrypted", "room_id": roomID, "event_id": "$receipt", "sender": owner, "content": map[string]string{"algorithm": domain.MatrixEncryption}}
				switch fault {
				case "human":
					value["sender"], _ = client.UserID(subject)
				case "tenant":
					other := scope
					other.Tenant = "other"
					value["sender"] = storage.IsolatedMatrixUserPrefix(other) + "matrix.sytue.eu"
				case "product":
					other := scope
					other.Product = "mail"
					value["sender"] = storage.IsolatedMatrixUserPrefix(other) + "matrix.sytue.eu"
				case "room":
					value["room_id"] = "!foreign:matrix.sytue.eu"
				case "event":
					value["event_id"] = "$foreign"
				case "plain":
					value["type"] = "m.room.message"
				case "algorithm":
					value["content"] = map[string]string{"algorithm": "other"}
				case "state":
					value["state_key"] = ""
				}
				encode(t, w, map[string]any{"event": value})
			})
			err := client.ValidateStorageEvent(context.Background(), domain.MatrixEventRef{RoomID: roomID, EventID: "$receipt"}, scope)
			if (fault == "valid") != (err == nil) {
				t.Fatal("isolated receipt sender check", fault, err)
			}
		})
	}
}
