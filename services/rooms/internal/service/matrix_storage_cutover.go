// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	matrixclient "git.sytue.eu/sytue/one/services/rooms/internal/matrix"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func (s *Service) cutoverActor(ctx context.Context, actor, product string) error {
	if !storage.Product(product) {
		return domain.ErrInvalidInput
	}
	if s.matrix == nil || s.pstore == nil {
		return domain.ErrMatrixUnavailable
	}
	return s.matrixNativeActor(ctx, actor)
}

func (s *Service) MatrixStorageMigration(ctx context.Context, actor, product string) (storage.IdentityCutoverState, error) {
	if err := s.cutoverActor(ctx, actor, product); err != nil {
		return storage.IdentityCutoverState{}, err
	}
	return s.pstore.MatrixStorageCutover(ctx, actor, product)
}

func (s *Service) sourceRoomFence(actor, product string, coverage matrixclient.PublicationCoverage) store.FreezeStorageRoom {
	return func(ctx context.Context, room string) error {
		if err := requireSamePublicationOrigin(ctx, s.matrix, coverage); err != nil {
			return err
		}
		_, err := s.matrix.EnsureStorageRoomPolicy(ctx, tenancy.MustFrom(ctx).Slug, actor, product, room, true)
		if err != nil {
			return err
		}
		return requireSamePublicationOrigin(ctx, s.matrix, coverage)
	}
}

// Begin uses two commits deliberately: a durable freezing marker precedes
// Matrix writes, so remote success followed by a database error cannot reopen
// a retained source after a process restart.
func (s *Service) BeginMatrixStorageMigration(ctx context.Context, actor, product, epoch string) (storage.IdentityCutoverState, error) {
	if err := s.cutoverActor(ctx, actor, product); err != nil {
		return storage.IdentityCutoverState{}, err
	}
	coverage, err := s.matrix.RequirePublicationCoverage(ctx)
	if err != nil {
		return storage.IdentityCutoverState{}, err
	}
	state, err := s.pstore.BeginMatrixStorageCutover(ctx, actor, product, epoch)
	if err != nil || state.Phase == "isolated" {
		return state, err
	}
	return s.pstore.FreezeMatrixStorageCutover(ctx, actor, product, epoch, s.sourceRoomFence(actor, product, coverage))
}

func (s *Service) CompleteMatrixStorageMigration(ctx context.Context, actor, product, epoch string, receipt storage.Reference) (storage.IdentityCutoverState, error) {
	if err := s.cutoverActor(ctx, actor, product); err != nil {
		return storage.IdentityCutoverState{}, err
	}
	coverage, err := s.matrix.RequirePublicationCoverage(ctx)
	if err != nil {
		return storage.IdentityCutoverState{}, err
	}
	scope := storage.Scope{Tenant: tenancy.MustFrom(ctx).Slug, Subject: actor, Product: product}
	return s.pstore.CompleteMatrixStorageCutover(ctx, actor, product, epoch, receipt, s.sourceRoomFence(actor, product, coverage), func(ctx context.Context) error {
		return s.matrix.ValidateStorageEvent(ctx, domain.MatrixEventRef{RoomID: receipt.RoomID, EventID: receipt.EventID}, scope)
	})
}
