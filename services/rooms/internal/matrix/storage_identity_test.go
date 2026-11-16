// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestStorageMatrixIdentitySeparatesActualUsersAndPreservesLegacy(t *testing.T) {
	c := testClient(t, func(http.ResponseWriter, *http.Request) { t.Error("identity formatting reached HTTP") })
	seen := map[string]bool{}
	for _, product := range storage.Products() {
		for _, tenant := range []string{"acme", "p-owner"} {
			for _, sub := range []string{subject, "ec7d2dfe-9af5-4be4-87f4-f26660b75c8e"} {
				mxid, err := c.StorageUserID(storage.Scope{Tenant: tenant, Subject: sub, Product: product})
				if err != nil || seen[mxid] || strings.HasPrefix(mxid, "@sytue_"+sub+":") {
					t.Fatal("storage accounts collided with another scope or legacy", err)
				}
				seen[mxid] = true
			}
		}
	}
	if got, _ := c.UserID(subject); got != "@sytue_"+subject+":matrix.sytue.eu" {
		t.Fatal("legacy Matrix identity changed")
	}
	got, err := c.StorageUserID(storage.Scope{Tenant: "acme", Subject: subject, Product: "drive"})
	if err != nil || got != "@sytue_storage_drive_acme_"+subject+":matrix.sytue.eu" {
		t.Fatal("scoped OIDC localpart contract changed", err)
	}
	if provider, _ := c.StorageSSOProviderID("drive"); provider != "oidc-sytue-storage-drive" {
		t.Fatal("scoped OIDC provider contract changed")
	}
	for _, scope := range []storage.Scope{{Tenant: "../acme", Subject: subject, Product: "drive"}, {Tenant: "acme", Subject: "../person", Product: "drive"}, {Tenant: "acme", Subject: subject, Product: "other"}} {
		if _, err := c.StorageUserID(scope); err == nil {
			t.Fatal("invalid storage identity accepted")
		}
	}
	products := storage.Products()
	products[0] = "unregistered"
	if storage.Product("unregistered") || !storage.Product("hub") {
		t.Fatal("caller changed the private product registry")
	}
}

func TestStorageUserProvisioningRequiresFreshTenantGateAndCreatesNoSecrets(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: subject, Product: "drive"}
	var c *Client
	puts, gates := 0, 0
	c = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		puts++
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(r.URL.Path, "/suspend/") {
			if string(body["suspend"]) != "false" || len(body) != 1 {
				t.Error("provision did not request scoped reactivation")
			}
		} else if _, unlock := body["locked"]; unlock {
			if string(body["locked"]) != "false" || string(body["logout_devices"]) != "false" || len(body) != 2 {
				t.Error("scoped resume changed another account field")
			}
		} else {
			if len(body) != 2 {
				t.Error("provisioning added an unexpected permission or identity field")
			}
			for _, name := range []string{"password", "access_token", "devices", "keys"} {
				if _, ok := body[name]; ok {
					t.Error("server provisioned user secrets")
				}
			}
			var mappings []map[string]string
			if json.Unmarshal(body["external_ids"], &mappings) != nil || len(mappings) != 1 || mappings[0]["auth_provider"] != "oidc-sytue-storage-drive" || mappings[0]["external_id"] != "acme|"+subject || string(body["logout_devices"]) != "false" {
				t.Error("OIDC binding did not include product and tenant")
			}
		}
		encode(t, w, map[string]any{})
	})
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
	if _, err := c.EnsureStorageUser(ctx, scope); !errors.Is(err, domain.ErrMatrixUnavailable) {
		t.Fatal("missing lifecycle gate accepted", err)
	}
	denied := false
	c.SetIdentityGate(identityGateFunc(func(ctx context.Context, sub string, fn func(context.Context) error) error {
		gates++
		if sub != subject {
			t.Error("gate checked a different human")
		}
		if denied {
			return domain.ErrForbidden
		}
		return fn(ctx)
	}))
	for range 2 {
		if _, err := c.EnsureStorageUser(ctx, scope); err != nil {
			t.Fatal(err)
		}
	}
	denied = true
	if _, err := c.EnsureStorageUser(ctx, scope); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("cache bypassed lifecycle denial", err)
	}
	if _, err := c.EnsureStorageUser(tenancy.Into(t.Context(), tenancy.Tenant{Slug: "another"}), scope); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("foreign tenant accepted", err)
	}
	if puts != 5 || gates != 3 {
		t.Fatal("cached provisioning changed identity or bypassed fresh authorization", puts, gates)
	}
}

func TestStorageProvisioningNeverTakesOverAnotherSSOMapping(t *testing.T) {
	for _, field := range []string{"valid", "name", "provider", "external", "extra"} {
		t.Run(field, func(t *testing.T) {
			puts := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					puts++
					encode(t, w, map[string]any{})
					return
				}
				name, provider, external := "@sytue_storage_drive_acme_"+subject+":matrix.sytue.eu", "oidc-sytue-storage-drive", "acme|"+subject
				switch field {
				case "name":
					name = "@other:matrix.sytue.eu"
				case "provider":
					provider = "oidc-sytue"
				case "external":
					external = "another|" + subject
				}
				mappings := []map[string]string{{"auth_provider": provider, "external_id": external}}
				if field == "extra" {
					mappings = append(mappings, map[string]string{"auth_provider": "another", "external_id": "person"})
				}
				encode(t, w, map[string]any{"name": name, "external_ids": mappings})
			})
			c.SetIdentityGate(identityGateFunc(func(ctx context.Context, _ string, fn func(context.Context) error) error { return fn(ctx) }))
			_, err := c.EnsureStorageUser(tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"}), storage.Scope{Tenant: "acme", Subject: subject, Product: "drive"})
			if field == "valid" {
				if err != nil || puts != 2 {
					t.Fatal("valid mapping was rewritten or not resumed", err)
				}
			} else if !errors.Is(err, domain.ErrConflict) || puts != 0 {
				t.Fatal("foreign mapping was adopted or overwritten", err)
			}
		})
	}
}

func TestStorageSuspensionPagesThroughScopesWithoutTouchingHumanOrOtherTenants(t *testing.T) {
	for _, tenant := range []string{"", "acme"} {
		t.Run("tenant="+tenant, func(t *testing.T) {
			pages, suspended := 0, []string{}
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					suspended = append(suspended, strings.TrimPrefix(r.URL.Path, "/_synapse/admin/v1/suspend/"))
					encode(t, w, map[string]any{})
					return
				}
				pages++
				if r.URL.Query().Get("name") != subject || r.URL.Query().Get("order_by") != "name" || r.URL.Query().Get("locked") != "true" {
					t.Error("enumeration lost its stable subject filter")
				}
				if pages == 1 {
					encode(t, w, map[string]any{"users": []map[string]string{{"name": "@sytue_" + subject + ":matrix.sytue.eu"}, {"name": "@sytue_storage_drive_acme_" + subject + ":matrix.sytue.eu"}, {"name": "@sytue_storage_mail_acme_other:matrix.sytue.eu"}}, "next_token": "100"})
				} else {
					if r.URL.Query().Get("from") != "100" {
						t.Error("enumeration ignored the server cursor")
					}
					encode(t, w, map[string]any{"users": []map[string]any{{"name": "@sytue_storage_mail_beta_" + subject + ":matrix.sytue.eu"}, {"name": "@sytue_storage_mail_acme_" + subject + ":evil.test"}, {"name": "@sytue_storage_wrong_acme_" + subject + ":matrix.sytue.eu"}, {"name": "@sytue_storage_photos_acme_" + subject + ":matrix.sytue.eu", "deactivated": true}}})
				}
			})
			if err := c.SuspendStorageUsers(t.Context(), subject, tenant); err != nil {
				t.Fatal(err)
			}
			want := []string{"@sytue_storage_drive_acme_" + subject + ":matrix.sytue.eu"}
			if tenant == "" {
				want = append(want, "@sytue_storage_mail_beta_"+subject+":matrix.sytue.eu")
			}
			if pages != 2 || !slices.Equal(suspended, want) {
				t.Fatal("scope enumeration missed a page or suspended another account", pages, suspended)
			}
		})
	}
}

func TestStorageSuspensionRejectsBrokenPaginationAndProviderErrors(t *testing.T) {
	for _, cursor := range []any{"100", "bad", -1, []int{1}} {
		c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
			encode(t, w, map[string]any{"users": []any{}, "next_token": cursor})
		})
		if err := c.SuspendStorageUsers(t.Context(), subject, ""); !errors.Is(err, domain.ErrMatrixUnavailable) {
			t.Fatal("invalid or repeating pagination was accepted", err)
		}
	}
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider secret fixture", http.StatusServiceUnavailable)
	})
	if err := c.SuspendStorageUsers(t.Context(), subject, ""); !errors.Is(err, domain.ErrMatrixUnavailable) || strings.Contains(err.Error(), "secret") {
		t.Fatal("unsafe provider failure", err)
	}
}
