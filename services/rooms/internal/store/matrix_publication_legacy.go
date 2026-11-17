// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func matrixPublicationScopeTx(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
	_, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true)`, scope.Subject, scope.Product)
	if err != nil {
		return err
	}
	return matrixPublicationContractTx(ctx, tx)
}

// AdmitLegacyMatrixPublicationOrigin preserves the original canonical SDK
// account. It does not borrow the isolated account's root or device authority.
// The identity lock serializes this admission with durable migration Begin.
func (s *Store) AdmitLegacyMatrixPublicationOrigin(ctx context.Context, scope storage.Scope, expectedUser string, event publication.OriginEvent) error {
	if !event.Valid() || event.Type != "m.room.encrypted" || event.Sender != expectedUser {
		return domain.ErrForbidden
	}
	digest, marker, err := publication.WireDigest(event.Content, false)
	if err != nil {
		return err
	}
	if marker.Valid() {
		return domain.ErrForbidden
	}
	return s.withStorageCutover(ctx, scope.Subject, scope.Product, func(ctx context.Context, tx pgx.Tx, actual storage.Scope) error {
		if actual != scope {
			return domain.ErrForbidden
		}
		var owns bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_storage_rooms WHERE tenant=$1 AND subject=$2 AND product=$3 AND room_id=$4)`, scope.Tenant, scope.Subject, scope.Product, event.Reference.RoomID).Scan(&owns); err != nil {
			return err
		}
		if !owns {
			return domain.ErrForbidden
		}
		if err := matrixPublicationScopeTx(ctx, tx, scope); err != nil {
			return err
		}
		existing, err := matrixPublicationExistingOriginTx(ctx, tx, scope, event.Reference.EventID, digest, expectedUser)
		if err != nil || existing {
			return err
		}
		state, err := storageCutoverTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		if state.Phase != "legacy" {
			return domain.ErrConflict
		}
		txn, err := matrixproof.RequestDigest(event.Reference)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO matrix_storage_publications(tenant,subject,product,id,namespace,origin_kind,matrix_user_id,device_id,room_id,transaction_id,cipher_sha256,authority_generation,status,event_id) VALUES($1,$2,$3,$4,$5,'legacy',$6,'',$7,$8,$9,0,'sending',$10)`, scope.Tenant, scope.Subject, scope.Product, uuid.New(), "storage:"+scope.Product, expectedUser, event.Reference.RoomID, "legacy_"+txn, digest, event.Reference.EventID)
		return err
	})
}

func matrixLegacyPublicationPendingTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, rooms []string) (bool, error) {
	if err := matrixPublicationScopeTx(ctx, tx, scope); err != nil {
		return false, err
	}
	var pending bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_storage_publications WHERE tenant=$1 AND subject=$2 AND product=$3 AND room_id=ANY($4::text[]) AND status='sending')`, scope.Tenant, scope.Subject, scope.Product, rooms).Scan(&pending)
	return pending, err
}
