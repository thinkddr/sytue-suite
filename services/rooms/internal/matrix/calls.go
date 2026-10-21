// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// ValidateDevice checks an existing, non-dehydrated device owned by this actor.
// Signature and cross-signing trust are checked by the participating SDKs.
func (c *Client) ValidateDevice(ctx context.Context, subject, device string) error {
	if device == "" || len(device) > 255 || strings.ContainsAny(device, "\x00\r\n") {
		return domain.ErrForbidden
	}
	mxid, err := c.UserID(subject)
	if err != nil {
		return err
	}
	var result struct {
		Devices []struct {
			DeviceID   string `json:"device_id"`
			UserID     string `json:"user_id"`
			Dehydrated bool   `json:"dehydrated"`
		} `json:"devices"`
	}
	if err := c.call(ctx, http.MethodGet, "/_synapse/admin/v2/users/"+url.PathEscape(mxid)+"/devices", nil, &result); err != nil {
		return err
	}
	for _, d := range result.Devices {
		if d.DeviceID == device && (d.UserID == "" || d.UserID == mxid) && !d.Dehydrated {
			return nil
		}
	}
	return domain.ErrForbidden
}

func (c *Client) EnsureCallRoom(ctx context.Context, tenant string, call uuid.UUID, generation int64, subjects []string) (string, error) {
	if !tenancy.ValidSlug(tenant) || call == uuid.Nil || generation < 1 {
		return "", domain.ErrInvalidInput
	}
	return c.ensureRoom(ctx, fmt.Sprintf("calls_%s_%s_%d", tenant, call, generation), "", subjects, false)
}
