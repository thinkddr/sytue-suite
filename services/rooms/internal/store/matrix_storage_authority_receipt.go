// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// VerifyMatrixStorageReceipt holds the authoritative root/identity locks while
// the configured Matrix origin validates public encrypted-event metadata. It
// never reads plaintext, recovery material or a caller-selected trust anchor.
func (s *Store) VerifyMatrixStorageReceipt(ctx context.Context, subject, product string, target storage.PublicationTarget, receipt storage.Reference, verify func(context.Context, storage.Scope, storage.Reference) error) error {
	if verify == nil || !target.Valid() || !receipt.Valid() || receipt.RoomID != target.RoomID {
		return domain.ErrInvalidInput
	}
	return s.withMatrixAuthority(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		authority, err := matrixAuthorityTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		if err := authority.RequireActive(scope, authority.Generation); err != nil {
			return err
		}
		if err := sinTraspasoTx(ctx, tx, scope, matrixproof.ErrUnverified); err != nil {
			return err
		}
		if err := matrixAuthorityOwnsTarget(ctx, tx, scope, target); err != nil {
			return err
		}
		return verify(ctx, scope, receipt)
	})
}
