// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func (s *Service) VerifyMatrixStorageReceipt(ctx context.Context, actor, product string, target storage.PublicationTarget, receipt storage.Reference) error {
	if err := s.matrixAuthorityScope(ctx, actor, product); err != nil {
		return err
	}
	return s.pstore.VerifyMatrixStorageReceipt(ctx, actor, product, target, receipt, func(ctx context.Context, scope storage.Scope, ref storage.Reference) error {
		return s.matrix.ValidateStorageEvent(ctx, domain.MatrixEventRef{RoomID: ref.RoomID, EventID: ref.EventID}, scope)
	})
}
