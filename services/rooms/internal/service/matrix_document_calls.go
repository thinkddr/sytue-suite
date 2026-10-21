// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func (s *Service) DocumentCallAccessChanged(ctx context.Context, id uuid.UUID, tenant string, version int64) error {
	if s.pstore == nil {
		return domain.ErrMatrixUnavailable
	}
	return s.pstore.AdvanceDocumentCallAccess(ctx, id, tenant, version)
}

func (s *Service) DocumentCallAccessVersion(ctx context.Context, id uuid.UUID) (int64, error) {
	if s.pstore == nil {
		return 0, domain.ErrMatrixUnavailable
	}
	return s.pstore.DocumentCallAccessVersion(ctx, id)
}

func (s *Service) DocumentIdentityAccessChanged(ctx context.Context, actor, tenant string, version int64) error {
	if s.pstore == nil {
		return domain.ErrMatrixUnavailable
	}
	return s.pstore.AdvanceDocumentIdentityAccess(ctx, actor, tenant, version)
}

func (s *Service) DocumentIdentityAccessVersion(ctx context.Context, actor, tenant string) (int64, error) {
	if s.pstore == nil {
		return 0, domain.ErrMatrixUnavailable
	}
	return s.pstore.DocumentIdentityAccessVersion(ctx, actor, tenant)
}
