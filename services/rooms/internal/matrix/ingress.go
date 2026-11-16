// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"net/http"
	"net/url"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// IngressDeviceOrigin independently queries the configured homeserver. No
// request-supplied master, user ID, origin URL or device-key JSON is trusted.
// It neither enrolls a root nor grants queue access; the durable authority must
// verify this chain against the owner's previously confirmed root.
func (c *Client) IngressDeviceOrigin(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) (matrixproof.Origin, error) {
	if err := ctx.Err(); err != nil {
		return matrixproof.Origin{}, err
	}
	user, err := c.StorageUserID(scope)
	if err != nil {
		return matrixproof.Origin{}, err
	}
	tenant, err := tenancy.From(ctx)
	device := target.DeviceID
	if err != nil || tenant.Slug != scope.Tenant || !target.Valid() {
		return matrixproof.Origin{}, domain.ErrForbidden
	}
	if c.identityGate == nil {
		return matrixproof.Origin{}, domain.ErrMatrixUnavailable
	}
	return c.deviceOriginForUser(ctx, scope, user, device)
}

// deviceOriginForUser is the single public-SDK directory lookup shared by
// isolated storage and canonical resource identities. Callers derive user IDs
// independently before entering; no request may choose an origin or account.
func (c *Client) deviceOriginForUser(ctx context.Context, scope storage.Scope, user, device string) (matrixproof.Origin, error) {
	out := matrixproof.Origin{Scope: scope, ServerName: c.cfg.ServerName, UserID: id.UserID(user), DeviceID: id.DeviceID(device)}
	err := c.identityGate.WithMatrixIdentity(ctx, scope.Subject, func(ctx context.Context) error {
		var account struct {
			Name        string `json:"name"`
			Deactivated *bool  `json:"deactivated"`
			Locked      *bool  `json:"locked"`
			Suspended   *bool  `json:"suspended"`
		}
		if err := c.call(ctx, http.MethodGet, "/_synapse/admin/v2/users/"+url.PathEscape(user), nil, &account); err != nil {
			return err
		}
		if account.Name != user || account.Deactivated == nil || account.Locked == nil || account.Suspended == nil || *account.Deactivated || *account.Locked || *account.Suspended {
			return domain.ErrForbidden
		}
		if err := c.readStorageDevice(ctx, user, device); err != nil {
			return err
		}
		request := mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{id.UserID(user): mautrix.DeviceIDList{id.DeviceID(device)}}}
		if err := c.call(ctx, http.MethodPost, "/_matrix/client/v3/keys/query", request, &out.Keys); err != nil {
			return err
		}
		out.AccountActive, out.DeviceActive = true, true
		return nil
	})
	if err != nil {
		return matrixproof.Origin{}, err
	}
	return out, nil
}
