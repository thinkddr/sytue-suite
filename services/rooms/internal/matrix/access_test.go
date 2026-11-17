// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestAccountWithdrawalLocksBeforeDeviceDeletionAndScopesReactivation(t *testing.T) {
	human := "@sytue_" + subject + ":matrix.sytue.eu"
	scoped := "@sytue_storage_drive_acme_" + subject + ":matrix.sytue.eu"
	orphan := "@sytue_storage_mail_beta_" + subject + ":matrix.sytue.eu"
	calls := map[string][]string{}
	enumerations := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		base := "/_synapse/admin/v2/users/"
		if r.URL.Path == "/_synapse/admin/v2/users" {
			enumerations++
			if r.URL.Query().Get("locked") != "true" {
				t.Error("retry pagination excludes locked identities")
			}
			encode(t, w, map[string]any{"users": []map[string]string{{"name": human}, {"name": scoped}, {"name": orphan}, {"name": "@sytue_storage_mail_acme_other:matrix.sytue.eu"}}})
			return
		}
		mxid := strings.TrimPrefix(r.URL.Path, base)
		if strings.HasPrefix(r.URL.Path, "/_synapse/admin/v1/suspend/") {
			mxid = strings.TrimPrefix(r.URL.Path, "/_synapse/admin/v1/suspend/")
			var body map[string]bool
			_ = json.NewDecoder(r.Body).Decode(&body)
			label := "suspend"
			if !body["suspend"] {
				label = "resume"
			}
			calls[mxid] = append(calls[mxid], label)
			encode(t, w, map[string]any{})
			return
		}
		if strings.HasSuffix(mxid, "/devices") {
			mxid = strings.TrimSuffix(mxid, "/devices")
			calls[mxid] = append(calls[mxid], "devices")
			encode(t, w, map[string]any{"devices": []map[string]string{{"device_id": "ARTIFICIAL_A"}, {"device_id": "ARTIFICIAL_B"}}, "total": 2})
			return
		}
		if strings.HasSuffix(mxid, "/delete_devices") {
			mxid = strings.TrimSuffix(mxid, "/delete_devices")
			calls[mxid] = append(calls[mxid], "delete")
			var body struct {
				Devices []string `json:"devices"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !slices.Equal(body.Devices, []string{"ARTIFICIAL_A", "ARTIFICIAL_B"}) {
				t.Error("incomplete token retirement")
			}
			encode(t, w, map[string]any{})
			return
		}
		if r.Method == http.MethodGet {
			calls[mxid] = append(calls[mxid], "get")
			encode(t, w, map[string]any{"name": mxid, "admin": false})
			return
		}
		if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, base) {
			t.Error("unexpected destructive account operation")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body) != 2 || string(body["logout_devices"]) != "false" {
			t.Error("access control touched profile, SSSS, password or recovery data")
		}
		label := "lock"
		if string(body["locked"]) == "false" {
			label = "unlock"
		}
		calls[mxid] = append(calls[mxid], label)
		encode(t, w, map[string]any{})
	})
	if err := c.SetAccountAccess(t.Context(), subject, "suspended"); err != nil {
		t.Fatal(err)
	}
	for _, mxid := range []string{human, scoped, orphan} {
		if !slices.Equal(calls[mxid], []string{"get", "lock", "suspend", "devices", "delete"}) {
			t.Fatal("unsafe access withdrawal order", mxid, calls[mxid])
		}
	}
	calls = map[string][]string{}
	if err := c.SetAccountAccess(t.Context(), subject, "active"); err != nil {
		t.Fatal(err)
	}
	if enumerations != 1 || len(calls) != 1 || !slices.Equal(calls[human], []string{"get", "resume", "unlock"}) {
		t.Fatal("global activation reopened tenant-scoped identities", calls)
	}
}

func TestStorageWithdrawalRetiresOrphansEvenWithoutLegacyHuman(t *testing.T) {
	scoped := "@sytue_storage_drive_acme_" + subject + ":matrix.sytue.eu"
	locked, deleted := false, false
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_synapse/admin/v2/users" {
			encode(t, w, map[string]any{"users": []map[string]string{{"name": scoped}}})
			return
		}
		if strings.Contains(r.URL.Path, "@sytue_"+subject+":") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/devices") {
			if !locked {
				t.Error("listed before account lock")
			}
			encode(t, w, map[string]any{"devices": []map[string]string{{"device_id": "ARTIFICIAL"}}, "total": 1})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/delete_devices") {
			deleted = true
			encode(t, w, map[string]any{})
			return
		}
		if strings.Contains(r.URL.Path, "/suspend/") {
			encode(t, w, map[string]any{})
			return
		}
		if r.Method == http.MethodGet {
			encode(t, w, map[string]any{"name": scoped, "admin": false})
			return
		}
		locked = true
		encode(t, w, map[string]any{})
	})
	if err := c.SetAccountAccess(t.Context(), subject, "deleted"); err != nil || !locked || !deleted {
		t.Fatal("missing human hid isolated storage", err)
	}
}

func TestStorageWithdrawalFailsClosedOnIncompleteDevicesAndSafeProviderErrors(t *testing.T) {
	for _, fixture := range []string{"{}", `{"devices":[],"total":1}`, `{"devices":[{"device_id":"same"},{"device_id":"same"}],"total":2}`, `{"devices":[{"device_id":""}],"total":1}`, "unavailable"} {
		t.Run(fixture, func(t *testing.T) {
			scoped := "@sytue_storage_drive_acme_" + subject + ":matrix.sytue.eu"
			deletes := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/_synapse/admin/v2/users":
					encode(t, w, map[string]any{"users": []map[string]string{{"name": scoped}}})
				case strings.HasSuffix(r.URL.Path, "/devices"):
					if fixture == "unavailable" {
						http.Error(w, "provider-secret-fixture", http.StatusServiceUnavailable)
					} else {
						_, _ = w.Write([]byte(fixture))
					}
				case strings.HasSuffix(r.URL.Path, "/delete_devices"):
					deletes++
					encode(t, w, map[string]any{})
				case r.Method == http.MethodGet:
					encode(t, w, map[string]any{"name": scoped, "admin": false})
				default:
					encode(t, w, map[string]any{})
				}
			})
			err := c.RevokeStorageAccess(t.Context(), subject, "acme")
			if !errors.Is(err, domain.ErrMatrixUnavailable) || strings.Contains(err.Error(), "secret") || deletes != 0 {
				t.Fatal("incomplete revocation acknowledged", err)
			}
		})
	}
}
