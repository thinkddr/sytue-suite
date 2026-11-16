// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func (s *Service) matrixAuthorityScope(ctx context.Context, actor, product string) error {
	tenant, err := tenancy.From(ctx)
	scope := storage.Scope{Tenant: tenant.Slug, Subject: actor, Product: product}
	if s == nil || s.pstore == nil || s.matrix == nil {
		return domain.ErrMatrixUnavailable
	}
	if err != nil || !scope.Valid() {
		return domain.ErrInvalidInput
	}
	if err := s.matrixNativeActor(ctx, actor); err != nil {
		return err
	}
	return nil
}

func (s *Service) MatrixStorageAuthority(ctx context.Context, actor, product string) (matrixproof.Authority, error) {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return matrixproof.Authority{}, err
	}
	return s.pstore.MatrixStorageAuthority(ctx, actor, product)
}

func (s *Service) MatrixRootChallenge(ctx context.Context, actor, product, epoch string, target storage.PublicationTarget) (matrixproof.RootChallenge, error) {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return matrixproof.RootChallenge{}, err
	}
	return s.pstore.MatrixRootChallenge(ctx, actor, product, epoch, target)
}

func (s *Service) ConfirmMatrixRoot(ctx context.Context, actor, product string, proof matrixproof.SignedAction) (matrixproof.Authority, error) {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return matrixproof.Authority{}, err
	}
	return s.pstore.ConfirmMatrixRoot(ctx, actor, product, proof)
}

func (s *Service) VerifyMatrixStorageAction(ctx context.Context, actor, product string, expected matrixproof.Action, proof matrixproof.SignedAction) (matrixproof.Authorization, error) {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return matrixproof.Authorization{}, err
	}
	return s.pstore.VerifyMatrixStorageAction(ctx, actor, product, expected, proof)
}

func (s *Service) MatrixRootChangeChallenge(ctx context.Context, actor, product, kind, epoch string, target storage.PublicationTarget) (matrixproof.RootChallenge, error) {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return matrixproof.RootChallenge{}, err
	}
	return s.pstore.MatrixRootChangeChallenge(ctx, actor, product, kind, epoch, target)
}
