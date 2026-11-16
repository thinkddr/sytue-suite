// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func (s *Store) PrivateIngressQueue(authority callaccess.PrivateIngressAuthority, objects objectstore.PrivateIngressObjects) pg.PrivateIngressQueue {
	return pg.PrivateIngressQueue{Transaction: func(ctx context.Context, scope storage.Scope, epoch, kind string, operation func(pgx.Tx, pg.AuthorizedPrivateIngressConfig) error) error {
		if authority == nil {
			return pg.ErrPrivateIngressMissing
		}
		if !mailIngressOwner(ctx, scope) || !storage.MigrationEpoch(epoch) {
			return pg.ErrPrivateStorageScope
		}
		parsed, err := uuid.Parse(epoch)
		if err != nil {
			return err
		}
		return s.privateTx(ctx, scope.Subject, func(tx pgx.Tx) error {
			if err := s.mailIngressEpoch(ctx, tx, scope, parsed, kind); err != nil {
				return err
			}
			cfg := privateInboundConfig(scope, parsed)
			cfg.VerifyReceipt = func(ctx context.Context, claim ingress.Claim, receipt ingress.Receipt) error {
				return authority.VerifyStorageReceipt(ctx, scope, claim.Target, receipt.Commit)
			}
			return operation(tx, pg.AuthorizedPrivateIngressConfig{PrivateIngressConfig: cfg, Authority: authority})
		})
	}, Body: func(ctx context.Context, scope storage.Scope, id string) (io.ReadCloser, error) {
		if objects == nil || !mailIngressOwner(ctx, scope) {
			return nil, pg.ErrPrivateIngressMissing
		}
		tenant, err := tenancy.From(ctx)
		if err != nil {
			return nil, err
		}
		key, err := objectstore.PrivateIngressBlobKey(scope, id)
		if err != nil {
			return nil, err
		}
		return objects.Get(ctx, tenant, key)
	}}
}

func mailIngressOwner(ctx context.Context, scope storage.Scope) bool {
	claims, ok := authn.FromContext(ctx)
	tenant, err := tenancy.From(ctx)
	return ok && err == nil && !claims.Delegado && scope.Valid() && scope.Product == "mail" && scope.Tenant == tenant.Slug && claims.Tenant == scope.Tenant && claims.Subject == scope.Subject
}

func (s *Store) mailIngressEpoch(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID, kind string) error {
	var account uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE tenant=$1 AND subject=$2 AND kind='user'`, scope.Tenant, scope.Subject).Scan(&account); err != nil {
		return err
	}
	if err := pg.EnsurePrivateStorageState(ctx, tx, "mail_private_storage_states", scope.Tenant, scope.Subject, &account); err != nil {
		return err
	}
	var mode string
	var current *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT mode,epoch FROM mail_private_storage_states WHERE tenant=$1 AND owner_sub=$2 AND account_id=$3 FOR SHARE`, scope.Tenant, scope.Subject, account).Scan(&mode, &current); err != nil {
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
