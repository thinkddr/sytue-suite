// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// ResourceSessionOriginForDocs reads public enrollment evidence from the SAME
// current registry and configured SDK directory used by resource publication.
// The internal transport grants no source access or publication authority; Docs
// still verifies its caller's signed query and original ACL/freeze transaction.
func (s *Service) ResourceSessionOriginForDocs(ctx context.Context, request resourcepg.SessionOriginRequest) (resourcepg.SessionOriginResponse, error) {
	var out resourcepg.SessionOriginResponse
	if !request.ValidForService("docs") {
		return out, domain.ErrInvalidInput
	}
	if s == nil || s.pstore == nil || s.matrix == nil {
		return out, domain.ErrMatrixUnavailable
	}
	tenant, err := tenancy.New(request.Scope.Tenant)
	if err != nil {
		return out, domain.ErrInvalidInput
	}
	ctx = tenancy.Into(ctx, tenant)
	provider, err := s.pstore.ResourceSessionProvider()
	if err != nil {
		return out, err
	}
	current, err := provider.Resolve(ctx, request.Scope, request.DeviceID)
	if err != nil {
		return out, err
	}
	origin, err := s.matrix.ResourceSessionOrigin(ctx, request.Scope, request.DeviceID)
	if err != nil {
		return out, err
	}
	expected, err := s.matrix.UserID(request.Scope.Subject)
	if err != nil {
		return out, err
	}
	device, err := matrixproof.VerifyUser(ctx, origin, id.UserID(expected), id.Ed25519(current.Binding().RootKey))
	if err != nil || device.Identity() != current.Device().Identity() {
		return out, domain.ErrForbidden
	}
	if err := json.Unmarshal(current.PublicRecordBytes(), &out.Record); err != nil {
		return resourcepg.SessionOriginResponse{}, domain.ErrMatrixUnavailable
	}
	if err := current.Revalidate(ctx); err != nil {
		return resourcepg.SessionOriginResponse{}, err
	}
	out.Version = 1
	out.Origin = origin
	return out, nil
}
