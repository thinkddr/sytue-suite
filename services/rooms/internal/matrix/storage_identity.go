// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

const storageUserPrefix = storage.IsolatedUserPrefix

// StorageUserID separates the actual Matrix account, SSSS and backup namespace.
// A routing room or product label alone cannot isolate an extension's keys.
func (c *Client) StorageUserID(scope storage.Scope) (string, error) {
	if _, err := c.UserID(scope.Subject); err != nil {
		return "", err
	}
	if !tenancy.ValidSlug(scope.Tenant) || !storage.Product(scope.Product) {
		return "", domain.ErrInvalidInput
	}
	mxid := storage.IsolatedMatrixUserPrefix(scope) + c.cfg.ServerName
	if len(mxid) > 255 {
		return "", domain.ErrInvalidInput
	}
	return mxid, nil
}

// StorageSSOProviderID matches the maintained Synapse OIDC provider registry.
func (c *Client) StorageSSOProviderID(product string) (string, error) {
	if !c.configured() || !storage.Product(product) {
		return "", domain.ErrMatrixUnavailable
	}
	return c.cfg.SSOProviderID + "-storage-" + product, nil
}

// EnsureStorageUser runs under the existing identity AND tenant lifecycle gate.
// It never creates passwords, tokens, devices or encryption material.
func (c *Client) EnsureStorageUser(ctx context.Context, scope storage.Scope) (string, error) {
	mxid, err := c.StorageUserID(scope)
	if err != nil {
		return "", err
	}
	tenant, err := tenancy.From(ctx)
	if err != nil || tenant.Slug != scope.Tenant {
		return "", domain.ErrForbidden
	}
	if c.identityGate == nil {
		return "", domain.ErrMatrixUnavailable
	}
	provider, err := c.StorageSSOProviderID(scope.Product)
	if err != nil {
		return "", err
	}
	err = c.identityGate.WithMatrixIdentity(ctx, scope.Subject, func(ctx context.Context) error {
		if err := c.ensureStorageMapping(ctx, mxid, provider, scope.Tenant+"|"+scope.Subject); err != nil {
			return err
		}
		// Reactivation is deliberately scoped: enabling the human account never
		// reopens a product in an organization that has removed this person.
		return c.resumeStorageUser(ctx, mxid)
	})
	return mxid, err
}

func (c *Client) ensureStorageMapping(ctx context.Context, mxid, provider, external string) error {
	if _, ok := c.users.Load(mxid); ok {
		return nil
	}
	var existing struct {
		Name        string `json:"name"`
		ExternalIDs []struct {
			Provider string `json:"auth_provider"`
			ID       string `json:"external_id"`
		} `json:"external_ids"`
	}
	err := c.call(ctx, http.MethodGet, "/_synapse/admin/v2/users/"+url.PathEscape(mxid), nil, &existing)
	if errors.Is(err, domain.ErrNotFound) {
		return c.provisionUser(ctx, mxid, provider, external)
	}
	if err != nil {
		return err
	}
	if existing.Name != mxid || len(existing.ExternalIDs) != 1 || existing.ExternalIDs[0].Provider != provider || existing.ExternalIDs[0].ID != external {
		return domain.ErrConflict
	}
	c.users.Store(mxid, true)
	return nil
}

func (c *Client) suspendStorageUser(ctx context.Context, mxid string, suspended bool) error {
	return c.call(ctx, http.MethodPut, "/_synapse/admin/v1/suspend/"+url.PathEscape(mxid), map[string]bool{"suspend": suspended}, nil)
}

// SuspendStorageUsers also finds a provision completed remotely before a failed
// database commit. It does not depend on a complete local room mapping snapshot.
// A non-empty tenant limits organization removal; the human Rooms account stays.
// Suspension is write policy, not token invalidation or a ban on historical reads.
func (c *Client) SuspendStorageUsers(ctx context.Context, subject, tenant string) error {
	return c.eachStorageUser(ctx, subject, tenant, func(mxid string) error { return c.suspendStorageUser(ctx, mxid, true) })
}

func (c *Client) eachStorageUser(ctx context.Context, subject, tenant string, apply func(string) error) error {
	if _, err := c.UserID(subject); err != nil {
		return err
	}
	if tenant != "" && !tenancy.ValidSlug(tenant) {
		return domain.ErrInvalidInput
	}
	seen := map[string]bool{}
	cursor := ""
	for range 100 {
		query := url.Values{"name": {subject}, "limit": {"100"}, "order_by": {"name"}, "dir": {"f"}, "deactivated": {"true"}, "locked": {"true"}}
		if cursor != "" {
			query.Set("from", cursor)
		}
		var page struct {
			Users []struct {
				Name        string `json:"name"`
				Deactivated bool   `json:"deactivated"`
			} `json:"users"`
			Next json.RawMessage `json:"next_token"`
		}
		if err := c.call(ctx, http.MethodGet, "/_synapse/admin/v2/users?"+query.Encode(), nil, &page); err != nil {
			return err
		}
		for _, user := range page.Users {
			scope, ok := c.storageIdentity(user.Name, subject)
			if ok && !user.Deactivated && (tenant == "" || tenant == scope.Tenant) {
				if err := apply(user.Name); err != nil {
					return err
				}
			}
		}
		next, err := storageUsersCursor(page.Next)
		if err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		if seen[next] {
			return domain.ErrMatrixUnavailable
		}
		seen[next], cursor = true, next
	}
	return domain.ErrMatrixUnavailable
}

func (c *Client) storageIdentity(mxid, subject string) (storage.Scope, bool) {
	suffix := "_" + subject + ":" + c.cfg.ServerName
	if !strings.HasPrefix(mxid, storageUserPrefix) || !strings.HasSuffix(mxid, suffix) {
		return storage.Scope{}, false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(mxid, storageUserPrefix), suffix)
	product, tenant, ok := strings.Cut(body, "_")
	scope := storage.Scope{Tenant: tenant, Subject: subject, Product: product}
	expected, err := c.StorageUserID(scope)
	return scope, ok && err == nil && expected == mxid
}

func storageUsersCursor(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var cursor string
	if json.Unmarshal(raw, &cursor) != nil {
		var value int64
		if json.Unmarshal(raw, &value) != nil || value < 0 {
			return "", domain.ErrMatrixUnavailable
		}
		cursor = strconv.FormatInt(value, 10)
	}
	if cursor == "" {
		return "", nil
	}
	if _, err := strconv.ParseUint(cursor, 10, 63); err != nil {
		return "", domain.ErrMatrixUnavailable
	}
	return cursor, nil
}

// EnsureIsolatedStorageRoom never invites the legacy human identity. Its
// deterministic alias is separate even when an old application room exists.
func (c *Client) EnsureIsolatedStorageRoom(ctx context.Context, scope storage.Scope, room string) (string, error) {
	mxid, err := c.EnsureStorageUser(ctx, scope)
	if err != nil {
		return "", err
	}
	return c.ensureRoomWithPolicy(ctx, "storage2_"+scope.Product+"_"+scope.Tenant+"_"+scope.Subject, room, false, func(ctx context.Context, room string) error {
		p, err := c.roomState(ctx, room)
		if err != nil {
			return err
		}
		return c.reconcilePolicy(ctx, room, p, map[string]bool{mxid: true}, false, true)
	})
}
