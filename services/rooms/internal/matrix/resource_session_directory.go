// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"strings"
	"unicode/utf8"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// ResourceSessionOrigin resolves only canonical Sytue users. It does not widen
// IngressDeviceOrigin/Verify, which remain restricted to isolated storage users.
func (c *Client) ResourceSessionOrigin(ctx context.Context, scope storage.Scope, device string) (matrixproof.Origin, error) {
	if err := ctx.Err(); err != nil {
		return matrixproof.Origin{}, err
	}
	tenant, err := tenancy.From(ctx)
	if err != nil || !scope.Valid() || tenant.Slug != scope.Tenant || len(device) < 1 || len(device) > 255 || !utf8.ValidString(device) || strings.ContainsAny(device, "\x00\r\n\t") {
		return matrixproof.Origin{}, domain.ErrForbidden
	}
	if c == nil || c.identityGate == nil {
		return matrixproof.Origin{}, domain.ErrMatrixUnavailable
	}
	user, err := c.UserID(scope.Subject)
	if err != nil {
		return matrixproof.Origin{}, err
	}
	return c.deviceOriginForUser(ctx, scope, user, device)
}

func (c *Client) ResourceSessionDevice(ctx context.Context, scope storage.Scope, device, pinnedRoot string) (matrixproof.Device, error) {
	origin, err := c.ResourceSessionOrigin(ctx, scope, device)
	if err != nil {
		return matrixproof.Device{}, err
	}
	user, err := c.UserID(scope.Subject)
	if err != nil {
		return matrixproof.Device{}, err
	}
	return matrixproof.VerifyUser(ctx, origin, id.UserID(user), id.Ed25519(pinnedRoot))
}

var _ resource.SessionDirectory = (*Client)(nil)
