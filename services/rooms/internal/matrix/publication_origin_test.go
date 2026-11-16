// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestStoragePublicationOriginReadsOnlyExactOpaqueEvent(t *testing.T) {
	for _, fault := range []string{"valid", "route-room", "foreign-tenant", "room", "event", "sender", "type", "missing-cipher", "missing", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			scope := storage.Scope{Tenant: "acme", Subject: subject, Product: "mail"}
			ref := storage.Reference{RoomID: roomID, EventID: "$opaque"}
			expected := "@sytue_storage_mail_acme_" + subject + ":matrix.sytue.eu"
			calls := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/_matrix/client/v3/rooms/"+roomID+"/event/"+ref.EventID || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer server-secret" {
					t.Error("readback changed route or credentials")
				}
				if fault == "missing" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				content := map[string]any{"algorithm": "m.megolm.v1.aes-sha2", "session_id": "synthetic", "ciphertext": "opaque"}
				value := map[string]any{"room_id": ref.RoomID, "event_id": ref.EventID, "sender": expected, "type": "m.room.encrypted", "content": content}
				switch fault {
				case "route-room":
					delete(value, "room_id")
				case "room":
					value["room_id"] = "!other:matrix.sytue.eu"
				case "event":
					value["event_id"] = "$other"
				case "sender":
					value["sender"] = "@other:matrix.sytue.eu"
				case "type":
					value["type"] = "m.room.message"
					value["content"] = map[string]string{"body": "must-never-be-acknowledged"}
				case "missing-cipher":
					delete(content, "ciphertext")
				}
				encode(t, w, value)
			})
			c.SetIdentityGate(identityGateFunc(func(context.Context, string, func(context.Context) error) error {
				t.Error("historical readback required live producer")
				return nil
			}))
			tenant := scope.Tenant
			if fault == "foreign-tenant" {
				tenant = "other"
			}
			ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: tenant})
			if fault == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			origin, err := c.StoragePublicationOrigin(ctx, scope, ref)
			if fault == "valid" || fault == "route-room" {
				if err != nil || origin.Reference != ref || origin.Sender != expected || !origin.Valid() {
					t.Fatal("exact readback rejected", err)
				}
			} else if err == nil {
				t.Fatal("altered origin accepted", fault)
			}
			if (fault == "foreign-tenant" || fault == "cancelled") && calls != 0 {
				t.Fatal("rejected request reached origin", calls)
			}
			if strings.Contains(string(origin.Content), "must-never-be-acknowledged") {
				t.Fatal("plaintext retained")
			}
		})
	}
}

func TestRetainedPublicationOriginKeepsCanonicalOrIsolatedSender(t *testing.T) {
	for _, kind := range []string{"canonical", "isolated", "other-product", "other-subject", "empty", "wire-sender"} {
		t.Run(kind, func(t *testing.T) {
			scope := storage.Scope{Tenant: "acme", Subject: subject, Product: "mail"}
			ref := storage.Reference{RoomID: roomID, EventID: "$retained"}
			canonical := "@sytue_" + subject + ":matrix.sytue.eu"
			isolated := "@sytue_storage_mail_acme_" + subject + ":matrix.sytue.eu"
			retained := canonical
			switch kind {
			case "isolated":
				retained = isolated
			case "other-product":
				retained = strings.Replace(isolated, "_mail_", "_calendar_", 1)
			case "other-subject":
				retained = "@sytue_other:matrix.sytue.eu"
			case "empty":
				retained = ""
			}
			calls := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				sender := retained
				if kind == "wire-sender" {
					sender = isolated
				}
				encode(t, w, map[string]any{"event_id": ref.EventID, "sender": sender, "type": "m.room.encrypted", "content": map[string]string{"algorithm": "m.megolm.v1.aes-sha2", "session_id": "synthetic", "ciphertext": "opaque"}})
			})
			origin, err := c.RetainedPublicationOrigin(tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant}), scope, ref, retained)
			if kind == "canonical" || kind == "isolated" {
				if err != nil || origin.Sender != retained || origin.Reference != ref {
					t.Fatal("retained producer changed", err)
				}
			} else if err == nil {
				t.Fatal("unbound historical sender accepted", kind)
			}
			if (kind == "other-product" || kind == "other-subject" || kind == "empty") && calls != 0 {
				t.Fatal("foreign retained sender reached origin")
			}
		})
	}
}
