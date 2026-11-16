// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/resourcepg"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// resourceBootstrapSource resolves a configured backend only. No request URL,
// owner-vault selector or publication flag participates in source selection.
func (s *Service) resourceBootstrapSource(query resource.VerifiedBootstrapSourceQuery) (*resourcepg.BootstrapHTTPSource, error) {
	if s == nil || resource.OriginServiceID(query.Value().Resource.Product) != "docs" || s.cfg.ResourceSourceDocsURL == "" || s.cfg.ResourceSourceDocsSecret == "" {
		return nil, domain.ErrMatrixUnavailable
	}
	endpoint, err := resourcepg.NewOriginSourceEndpoint("docs", s.cfg.ResourceSourceDocsURL, s.cfg.ResourceSourceDocsSecret, nil)
	if err != nil {
		return nil, domain.ErrMatrixUnavailable
	}
	return endpoint.BootstrapSource(query)
}

// MatrixResourceBootstrapSource returns ORIGINAL public owner/freeze metadata.
// The existing OIDC product audience and consumed SDK purpose are both required;
// a service credential or a signed JSON actor alone is not a session.
func (s *Service) MatrixResourceBootstrapSource(ctx context.Context, actor, product string, signed resource.SignedBootstrapSourceQuery) (resource.BootstrapSourceSnapshot, error) {
	var zero resource.BootstrapSourceSnapshot
	if s == nil || s.pstore == nil || s.matrix == nil {
		return zero, domain.ErrMatrixUnavailable
	}
	scope, _, err := s.resourceSessionScope(ctx, actor, product)
	if err != nil {
		return zero, err
	}
	if !signed.Valid() || signed.OwnerScope != scope || signed.Resource.Product != product {
		return zero, domain.ErrForbidden
	}
	provider, err := s.pstore.ResourceSessionProvider()
	if err != nil {
		return zero, err
	}
	current, err := provider.Resolve(ctx, scope, signed.Target.DeviceID)
	if err != nil {
		return zero, err
	}
	query, err := resource.VerifyBootstrapSourceQuery(ctx, signed, current)
	if err != nil {
		return zero, err
	}
	source, err := s.resourceBootstrapSource(query)
	if err != nil {
		return zero, err
	}
	snapshot, err := source.BootstrapSource(ctx, signed.Resource)
	if err != nil {
		if errors.Is(err, resource.ErrInvalid) || errors.Is(err, resource.ErrRevoked) || errors.Is(err, matrixproof.ErrUnverified) {
			return zero, domain.ErrForbidden
		}
		return zero, domain.ErrMatrixUnavailable
	}
	if err := query.Revalidate(ctx); err != nil {
		return zero, err
	}
	actual, _, err := s.resourceSessionScope(ctx, actor, product)
	if err != nil {
		return zero, err
	}
	if actual != scope {
		return zero, domain.ErrForbidden
	}
	return snapshot, nil
}
