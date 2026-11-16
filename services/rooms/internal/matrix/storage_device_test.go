// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestStorageDeviceFenceRequiresOriginalKeyAndAbsenceReadback(t *testing.T) {
	for _, fault := range []string{"valid", "absent", "changed-key", "present-after-delete", "delete-failure", "foreign-device", "malformed-key", "denied", "foreign-tenant", "missing-gate", "invalid-key", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			scope := storage.Scope{Tenant: "acme", Subject: subject, Product: "mail"}
			target := storage.PublicationTarget{RoomID: roomID, DeviceID: "ORIGINAL_A"}
			user := "@sytue_storage_mail_acme_" + subject + ":matrix.sytue.eu"
			key := strings.Repeat("A", 43)
			deleted := fault == "absent"
			calls, deletes := 0, 0
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer server-secret" {
					t.Error("device fence escaped configured authority")
				}
				switch {
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/devices/ORIGINAL_A"):
					if !strings.Contains(r.URL.Path, user) {
						t.Error("device fence queried another scope")
					}
					if deleted {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					owner := user
					if fault == "foreign-device" {
						owner = "@other:matrix.sytue.eu"
					}
					encode(t, w, map[string]string{"user_id": owner, "device_id": target.DeviceID})
				case r.Method == http.MethodPost && r.URL.Path == "/_matrix/client/v3/keys/query":
					var request mautrix.ReqQueryKeys
					if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.DeviceKeys) != 1 || !slices.Equal(request.DeviceKeys[id.UserID(user)], mautrix.DeviceIDList{id.DeviceID(target.DeviceID)}) {
						t.Error("device fence queried unrelated keys")
					}
					actual := key
					if fault == "changed-key" {
						actual = strings.Repeat("B", 43)
					}
					device := mautrix.DeviceKeys{UserID: id.UserID(user), DeviceID: id.DeviceID(target.DeviceID), Keys: mautrix.KeyMap{id.NewDeviceKeyID(id.KeyAlgorithmEd25519, id.DeviceID(target.DeviceID)): actual}}
					if fault == "malformed-key" {
						device.UserID = "@other:matrix.sytue.eu"
					}
					encode(t, w, mautrix.RespQueryKeys{DeviceKeys: map[id.UserID]map[id.DeviceID]mautrix.DeviceKeys{id.UserID(user): {id.DeviceID(target.DeviceID): device}}})
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/delete_devices"):
					deletes++
					var request struct {
						Devices []string `json:"devices"`
					}
					if json.NewDecoder(r.Body).Decode(&request) != nil || !slices.Equal(request.Devices, []string{target.DeviceID}) {
						t.Error("device fence retired another device")
					}
					if fault == "delete-failure" {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					deleted = fault != "present-after-delete"
					encode(t, w, map[string]any{})
				default:
					t.Errorf("device fence touched unrelated state: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			})
			if fault != "missing-gate" {
				client.SetIdentityGate(identityGateFunc(func(ctx context.Context, owner string, fn func(context.Context) error) error {
					if owner != scope.Subject || fault == "denied" {
						return domain.ErrForbidden
					}
					return fn(ctx)
				}))
			}
			tenant := scope.Tenant
			if fault == "foreign-tenant" {
				tenant = "other"
			}
			ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: tenant})
			if fault == "cancelled" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if fault == "invalid-key" {
				key = "invalid"
			}
			err := client.RevokeStorageDevice(ctx, scope, target, key)
			if fault == "valid" || fault == "absent" {
				if err != nil {
					t.Fatal(err)
				}
				expected := 1
				if fault == "absent" {
					expected = 0
				}
				if deletes != expected {
					t.Fatal("wrong number of device mutations", deletes)
				}
				if err := client.RequireStorageDeviceAbsent(ctx, scope, target); err != nil {
					t.Fatal("successful fence was not independently observable", err)
				}
				if err := client.RevokeStorageDevice(ctx, scope, target, key); err != nil || deletes != expected {
					t.Fatal("uncertain retry changed an already confirmed fence", err, deletes)
				}
			} else {
				if err == nil {
					t.Fatal("unconfirmed or substituted device fence accepted", fault)
				}
				if fault == "changed-key" || fault == "malformed-key" {
					if !errors.Is(err, domain.ErrConflict) || deletes != 0 {
						t.Fatal("replacement key caused a deletion", err, deletes)
					}
				}
				if slices.Contains([]string{"denied", "foreign-tenant", "missing-gate", "invalid-key", "cancelled"}, fault) && calls != 0 {
					t.Fatal("invalid scope reached the configured server", fault, calls)
				}
			}
		})
	}
}
