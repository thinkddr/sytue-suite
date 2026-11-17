// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

func (s *Store) PrivateIngressQueue(authority callaccess.PrivateIngressAuthority) pg.PrivateIngressQueue {
	return pg.PrivateIngressQueue{Transaction: func(ctx context.Context, scope storage.Scope, epoch, kind string, operation func(pgx.Tx, pg.AuthorizedPrivateIngressConfig) error) error {
		if authority == nil {
			return ErrPrivateStorageIngress
		}
		if scope.Product != "calendar" || !storage.MigrationEpoch(epoch) {
			return pg.ErrPrivateStorageScope
		}
		parsed, err := uuid.Parse(epoch)
		if err != nil {
			return err
		}
		return s.privateCalendarTx(ctx, scope.Subject, func(identity privateCalendarIdentity, tx pgx.Tx) error {
			if identity.scope != scope {
				return pg.ErrPrivateStorageScope
			}
			if err := calendarIngressEpoch(ctx, tx, scope, parsed, kind); err != nil {
				return err
			}
			cfg := pg.PrivateIngressConfig{Scope: scope, Epoch: parsed, VerifyReceipt: func(ctx context.Context, claim ingress.Claim, receipt ingress.Receipt) error {
				return authority.VerifyStorageReceipt(ctx, scope, claim.Target, receipt.Commit)
			}}
			return operation(tx, pg.AuthorizedPrivateIngressConfig{PrivateIngressConfig: cfg, Authority: authority})
		})
	}, Body: func(ctx context.Context, scope storage.Scope, id string) (io.ReadCloser, error) {
		var body io.ReadCloser
		err := s.privateCalendarTx(ctx, scope.Subject, func(identity privateCalendarIdentity, tx pgx.Tx) error {
			if scope != identity.scope {
				return pg.ErrPrivateStorageScope
			}
			// The opaque body's immutable epoch is resolved from the domain state,
			// never from a body locator supplied by the client.
			var epoch uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT epoch FROM calendar_private_storage_states WHERE tenant=$1 AND owner_sub=$2 AND mode IN('frozen','private') FOR SHARE`, scope.Tenant, scope.Subject).Scan(&epoch); err != nil {
				return err
			}
			var err error
			body, err = pg.ReadPrivateIngressInlineBody(ctx, tx, pg.PrivateIngressConfig{Scope: scope, Epoch: epoch}, id)
			return err
		})
		return body, err
	}}
}

func calendarIngressEpoch(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID, kind string) error {
	if err := pg.EnsurePrivateStorageState(ctx, tx, "calendar_private_storage_states", scope.Tenant, scope.Subject, nil); err != nil {
		return err
	}
	var mode string
	var current *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT mode,epoch FROM calendar_private_storage_states WHERE tenant=$1 AND owner_sub=$2 FOR SHARE`, scope.Tenant, scope.Subject).Scan(&mode, &current); err != nil {
		return err
	}
	if (kind == "key.enroll" || kind == "enrollment") && mode == "legacy" {
		return nil
	}
	if (mode != "frozen" && mode != "private") || current == nil || *current != epoch {
		return ErrPrivateStorageEpoch
	}
	return nil
}
