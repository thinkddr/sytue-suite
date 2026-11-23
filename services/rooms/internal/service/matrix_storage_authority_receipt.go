// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func (s *Service) VerifyMatrixStorageReceipt(ctx context.Context, actor, product string, target storage.PublicationTarget, receipt storage.Reference) error {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return err
	}
	tenant, err := tenancy.From(ctx)
	if err != nil {
		return domain.ErrInvalidInput
	}
	scope := storage.Scope{Tenant: tenant.Slug, Subject: actor, Product: product}
	// Un ámbito de bóveda (ADR 0087) no tiene eventos de Matrix que mirar.
	autoridad, errAutoridad := s.pstore.MatrixStorageAuthority(ctx, actor, product)
	return verificarRecibo(ctx, scope, autoridad, errAutoridad, target, receipt, s.recibosBoveda, func() error {
		return s.pstore.VerifyMatrixStorageReceipt(ctx, actor, product, target, receipt, func(ctx context.Context, scope storage.Scope, ref storage.Reference) error {
			return s.matrix.ValidateStorageEvent(ctx, domain.MatrixEventRef{RoomID: ref.RoomID, EventID: ref.EventID}, scope)
		})
	})
}
