// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestIngressOriginUsesConfiguredServerAndDerivedIsolatedUser(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: subject, Product: "mail"}
	target := storage.PublicationTarget{RoomID: roomID, DeviceID: "OWNER_DEVICE"}
	user := "@sytue_storage_mail_acme_" + subject + ":matrix.sytue.eu"
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer server-secret" {
			t.Error("missing independent server authorization")
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/devices/OWNER_DEVICE"):
			if !strings.Contains(r.URL.Path, user) {
				t.Error("device query escaped derived owner")
			}
			encode(t, w, map[string]any{"device_id": target.DeviceID, "user_id": user})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, user):
			encode(t, w, map[string]any{"name": user, "deactivated": false, "locked": false, "suspended": false})
		case r.Method == http.MethodPost && r.URL.Path == "/_matrix/client/v3/keys/query":
			var request mautrix.ReqQueryKeys
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if len(request.DeviceKeys) != 1 || len(request.DeviceKeys[id.UserID(user)]) != 1 || request.DeviceKeys[id.UserID(user)][0] != id.DeviceID(target.DeviceID) {
				t.Error("query requested caller-selected account or other devices")
			}
			encode(t, w, mautrix.RespQueryKeys{})
		default:
			t.Errorf("unexpected origin mutation/query: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant})
	gates := 0
	c.SetIdentityGate(identityGateFunc(func(ctx context.Context, sub string, fn func(context.Context) error) error {
		gates++
		if sub != scope.Subject {
			t.Error("identitygate checkedanother owner")
		}
		return fn(ctx)
	}))
	for range 2 {
		origin, err := c.IngressDeviceOrigin(ctx, scope, target)
		if err != nil || origin.Scope != scope || origin.UserID.String() != user || origin.DeviceID.String() != target.DeviceID || !origin.AccountActive || !origin.DeviceActive {
			t.Fatal("origin lost independent binding", err)
		}
	}
	if gates != 2 || calls != 6 {
		t.Fatal("device authorization was cached", gates, calls)
	}
}

func TestIngressOriginRejectsMissingForeignInactiveOrRevokedAuthority(t *testing.T) {
	for _, fault := range []string{"missing-gate", "foreign-tenant", "denied-gate", "invalid-device", "account-owner", "account-deactivated", "account-locked", "account-suspended", "account-flags-missing", "device-owner", "device-id", "device-missing", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			scope := storage.Scope{Tenant: "acme", Subject: subject, Product: "calendar"}
			target := storage.PublicationTarget{RoomID: roomID, DeviceID: "OWNER_DEVICE"}
			user := "@sytue_storage_calendar_acme_" + subject + ":matrix.sytue.eu"
			calls := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if strings.Contains(r.URL.Path, "/devices/") {
					if fault == "device-missing" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					owner, device := user, target.DeviceID
					if fault == "device-owner" {
						owner = "@other:matrix.sytue.eu"
					}
					if fault == "device-id" {
						device = "OTHER"
					}
					encode(t, w, map[string]any{"device_id": device, "user_id": owner})
					return
				}
				if r.Method == http.MethodGet {
					account := map[string]any{"name": user, "deactivated": fault == "account-deactivated", "locked": fault == "account-locked", "suspended": fault == "account-suspended"}
					if fault == "account-owner" {
						account["name"] = "@other:matrix.sytue.eu"
					}
					if fault == "account-flags-missing" {
						delete(account, "locked")
					}
					encode(t, w, account)
					return
				}
				t.Error("rejected device reached key query")
			})
			if fault != "missing-gate" {
				c.SetIdentityGate(identityGateFunc(func(ctx context.Context, _ string, fn func(context.Context) error) error {
					if fault == "denied-gate" {
						return domain.ErrForbidden
					}
					return fn(ctx)
				}))
			}
			tenant := scope.Tenant
			if fault == "foreign-tenant" {
				tenant = "another"
			}
			ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: tenant})
			if fault == "invalid-device" {
				target.DeviceID = "bad\n"
			}
			if fault == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := c.IngressDeviceOrigin(ctx, scope, target); err == nil {
				t.Fatal("rejected authority accepted", fault)
			} else if fault == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			switch fault {
			case "missing-gate", "foreign-tenant", "denied-gate", "invalid-device", "cancelled":
				if calls != 0 {
					t.Fatal("denial still reached independent origin", calls)
				}
			}
		})
	}
}
