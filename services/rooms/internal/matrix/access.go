// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// RevokeStorageAccess also covers accounts whose room provisioning did not
// commit locally. Locked users remain in enumeration, so retries and pagination
// do not skip earlier partial revocations. SSSS, cross-signing and backups remain.
func (c *Client) RevokeStorageAccess(ctx context.Context, subject, tenant string) error {
	return c.eachStorageUser(ctx, subject, tenant, func(mxid string) error { return c.setUserAccess(ctx, mxid, true) })
}

func (c *Client) resumeStorageUser(ctx context.Context, mxid string) error {
	if err := c.suspendStorageUser(ctx, mxid, false); err != nil {
		return err
	}
	return c.call(ctx, http.MethodPut, "/_synapse/admin/v2/users/"+url.PathEscape(mxid), map[string]bool{"locked": false, "logout_devices": false}, nil)
}

func (c *Client) setUserAccess(ctx context.Context, mxid string, blocked bool) error {
	base := "/_synapse/admin/v2/users/" + url.PathEscape(mxid)
	var user struct {
		Name        string `json:"name"`
		Admin       *bool  `json:"admin"`
		Deactivated bool   `json:"deactivated"`
	}
	err := c.call(ctx, http.MethodGet, base, nil, &user)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if user.Name != mxid || user.Admin == nil || *user.Admin {
		return domain.ErrConflict
	}
	if user.Deactivated {
		return nil
	}
	if !blocked {
		return c.resumeStorageUser(ctx, mxid)
	}
	// Lock before listing devices: existing requests and new logins are refused
	// while all known device tokens are invalidated. Never deactivate an account:
	// deactivation would delete its independent recovery and cross-signing data.
	if err := c.call(ctx, http.MethodPut, base, map[string]bool{"locked": true, "logout_devices": false}, nil); err != nil {
		return err
	}
	if err := c.suspendStorageUser(ctx, mxid, true); err != nil {
		return err
	}
	return c.revokeUserDevices(ctx, base)
}

func (c *Client) revokeUserDevices(ctx context.Context, base string) error {
	var response struct {
		Devices *[]struct {
			ID string `json:"device_id"`
		} `json:"devices"`
		Total *int `json:"total"`
	}
	if err := c.call(ctx, http.MethodGet, base+"/devices", nil, &response); err != nil {
		return err
	}
	if response.Devices == nil || response.Total == nil || len(*response.Devices) != *response.Total {
		return domain.ErrMatrixUnavailable
	}
	ids := make([]string, 0, len(*response.Devices))
	seen := map[string]bool{}
	for _, device := range *response.Devices {
		if device.ID == "" || len(device.ID) > 512 || strings.ContainsAny(device.ID, "\x00\r\n") || seen[device.ID] {
			return domain.ErrMatrixUnavailable
		}
		seen[device.ID] = true
		ids = append(ids, device.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return c.revokeDeviceIDs(ctx, base, ids)
}

func (c *Client) revokeDeviceIDs(ctx context.Context, base string, ids []string) error {
	return c.call(ctx, http.MethodPost, base+"/delete_devices", map[string]any{"devices": ids}, nil)
}
