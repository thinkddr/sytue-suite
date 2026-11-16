// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func (s *Service) ReserveMatrixStoragePublication(ctx context.Context, actor, product string, request publication.SignedRequest) (publication.Receipt, error) {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return publication.Receipt{}, err
	}
	coverage, err := s.matrix.RequirePublicationCoverage(ctx)
	if err != nil {
		return publication.Receipt{}, err
	}
	receipt, err := s.pstore.ReserveMatrixStoragePublication(ctx, actor, product, request)
	if err != nil {
		return publication.Receipt{}, err
	}
	if err := requireSamePublicationOrigin(ctx, s.matrix, coverage); err != nil {
		return publication.Receipt{}, err
	}
	return receipt, nil
}

func (s *Service) ReadMatrixStoragePublication(ctx context.Context, actor, product, room, txn string) (publication.Receipt, error) {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return publication.Receipt{}, err
	}
	return s.pstore.ReadMatrixStoragePublication(ctx, actor, product, room, txn)
}

// BeforeMatrixPublicationOrigin resolves routing independently of the module
// payload and authorizes only the exact owner identity for a private room.
func (s *Service) BeforeMatrixPublicationOrigin(ctx context.Context, event publication.OriginEvent) (publication.OriginDecision, error) {
	return s.matrixPublicationOrigin(ctx, event, false)
}

func (s *Service) AcceptedMatrixPublicationOrigin(ctx context.Context, event publication.OriginEvent) (publication.OriginDecision, error) {
	return s.matrixPublicationOrigin(ctx, event, true)
}

func (s *Service) matrixPublicationOrigin(ctx context.Context, event publication.OriginEvent, accepted bool) (publication.OriginDecision, error) {
	result := publication.OriginDecision{Version: 1, Coverage: publication.OriginCoverageKind}
	if !event.Valid() {
		return result, domain.ErrInvalidInput
	}
	if s == nil || s.pstore == nil || s.matrix == nil {
		return result, domain.ErrMatrixUnavailable
	}
	route, managed, err := s.pstore.MatrixPublicationRoomRoute(ctx, event.Reference.RoomID)
	if err != nil {
		return result, err
	}
	result.Managed = managed
	if !managed {
		result.Allow = !strings.HasPrefix(event.Sender, storage.IsolatedUserPrefix)
		return result, nil
	}
	if route.Resource.Valid() {
		// Registration makes this a managed room before its first event. The
		// separate fixed-source resource origin is not yet composed; never fall
		// through to canonical-human/unmanaged or personal-storage authority.
		return result, nil
	}
	scope := route.Scope
	expected, err := s.matrix.StorageUserID(scope)
	if route.Identity == storage.LegacyIdentity {
		expected, err = s.matrix.UserID(scope.Subject)
	}
	if err != nil {
		return result, err
	}
	if event.Sender != expected || event.Type != "m.room.encrypted" {
		return result, nil
	}
	tenant, err := tenancy.New(scope.Tenant)
	if err != nil {
		return result, err
	}
	ctx = tenancy.Into(ctx, tenant)
	switch {
	case accepted:
		err = s.pstore.AcceptMatrixPublicationOrigin(ctx, scope, expected, event)
	case route.Identity == storage.LegacyIdentity:
		err = s.pstore.AdmitLegacyMatrixPublicationOrigin(ctx, scope, expected, event)
	default:
		err = s.pstore.AdmitMatrixPublicationOrigin(ctx, scope, expected, event, s.cfg.MatrixPublicationRequireMarker)
	}
	if err != nil {
		return result, err
	}
	result.Allow = true
	return result, nil
}

func (s *Service) CancelMatrixStoragePublication(ctx context.Context, actor, product string, request publication.SignedRequest) (publication.Receipt, error) {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return publication.Receipt{}, err
	}
	return s.pstore.CancelMatrixStoragePublication(ctx, actor, product, request)
}

func (s *Service) ReadMatrixStoragePublicationRecovery(ctx context.Context, actor, product, room, txn string) (publication.RecoveryState, error) {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return publication.RecoveryState{}, err
	}
	return s.pstore.ReadMatrixStoragePublicationRecovery(ctx, actor, product, room, txn)
}
