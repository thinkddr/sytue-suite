// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// WithLegacyOwnerWrite keeps the shared source-state lock across an external
// S3 write or delete. Freeze owns the exclusive side, so an already running
// encoder/deleter must finish before enumeration and no new one can start.
func (s *Store) WithLegacyOwnerWrite(ctx context.Context, owner string, write func() error) error {
	tenant, err := tenancy.From(ctx)
	if err != nil || s == nil || s.pool == nil || owner == "" || write == nil {
		return pg.ErrPrivateStorageScope
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT public.sytue_private_storage_guard('public.photos_private_storage_states'::regclass,$1,$2)`, tenant.Slug, owner); err != nil {
			return err
		}
		return write()
	})
}
