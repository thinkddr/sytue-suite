// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func (c *Client) readStorageDevice(ctx context.Context, user, device string) error {
	var active struct {
		DeviceID string `json:"device_id"`
		UserID   string `json:"user_id"`
	}
	if err := c.call(ctx, http.MethodGet, "/_synapse/admin/v2/users/"+url.PathEscape(user)+"/devices/"+url.PathEscape(device), nil, &active); err != nil {
		return err
	}
	if active.DeviceID != device || active.UserID != user {
		return domain.ErrForbidden
	}
	return nil
}

func (c *Client) storageDeviceAccount(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) (string, error) {
	user, err := c.StorageUserID(scope)
	if err != nil {
		return "", err
	}
	tenant, err := tenancy.From(ctx)
	if err != nil || tenant.Slug != scope.Tenant || !target.Valid() {
		return "", domain.ErrForbidden
	}
	if c.identityGate == nil {
		return "", domain.ErrMatrixUnavailable
	}
	return user, nil
}

func (c *Client) storageDeviceKey(ctx context.Context, user, device, expected string) error {
	request := mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{id.UserID(user): mautrix.DeviceIDList{id.DeviceID(device)}}}
	var keys mautrix.RespQueryKeys
	if err := c.call(ctx, http.MethodPost, "/_matrix/client/v3/keys/query", request, &keys); err != nil {
		return err
	}
	actual, ok := keys.DeviceKeys[id.UserID(user)][id.DeviceID(device)]
	if !ok || actual.UserID != id.UserID(user) || actual.DeviceID != id.DeviceID(device) || actual.Keys[id.NewDeviceKeyID(id.KeyAlgorithmEd25519, id.DeviceID(device))] != expected {
		return domain.ErrConflict
	}
	return nil
}

// RevokeStorageDevice fences one original device at the configured Synapse.
// Its previously authorized public signing key prevents silently retiring a
// different device that reused the ID. Success requires readback of absence.
// Account recovery, cross-signing, backups and other devices are preserved.
func (c *Client) RevokeStorageDevice(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, expectedSigningKey string) error {
	user, err := c.storageDeviceAccount(ctx, scope, target)
	if err != nil {
		return err
	}
	if len(expectedSigningKey) != 43 {
		return domain.ErrInvalidInput
	}
	return c.identityGate.WithMatrixIdentity(ctx, scope.Subject, func(ctx context.Context) error {
		err := c.readStorageDevice(ctx, user, target.DeviceID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := c.storageDeviceKey(ctx, user, target.DeviceID, expectedSigningKey); err != nil {
			return err
		}
		if err := c.revokeDeviceIDs(ctx, "/_synapse/admin/v2/users/"+url.PathEscape(user), []string{target.DeviceID}); err != nil {
			return err
		}
		return c.requireStorageDeviceAbsent(ctx, user, target.DeviceID)
	})
}

func (c *Client) requireStorageDeviceAbsent(ctx context.Context, user, device string) error {
	err := c.readStorageDevice(ctx, user, device)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return domain.ErrConflict
}

// RequireStorageDeviceAbsent rechecks a durable fence before another device
// receives publication permission. A cached PostgreSQL timestamp is insufficient.
func (c *Client) RequireStorageDeviceAbsent(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) error {
	user, err := c.storageDeviceAccount(ctx, scope, target)
	if err != nil {
		return err
	}
	return c.identityGate.WithMatrixIdentity(ctx, scope.Subject, func(ctx context.Context) error { return c.requireStorageDeviceAbsent(ctx, user, target.DeviceID) })
}
