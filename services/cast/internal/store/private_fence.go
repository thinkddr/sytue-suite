// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// WithLegacyVideoWrite resolves the actual retained owner from SQL, then keeps
// the common SH row lock across the entire S3 effect. Freeze takes its EX side.
// Prefixes must identify exactly one video, including soft-deleted originals.
func (s *Store) WithLegacyVideoWrite(ctx context.Context, tenant, key string, write func() error) error {
	video, _, ok := strings.Cut(key, "/")
	id, err := uuid.Parse(video)
	if err != nil || id == uuid.Nil || id.String() != video || !ok || write == nil || s == nil || s.pool == nil || !tenancy.ValidSlug(tenant) {
		return pg.ErrPrivateStorageScope
	}
	if _, err := model.MediaID(video, strings.TrimSuffix(key, "/")); err != nil {
		return pg.ErrPrivateStorageScope
	}
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: tenant})
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var owner string
		if err := tx.QueryRow(ctx, `SELECT owner_sub FROM videos WHERE tenant=$1 AND id=$2`, tenant, id).Scan(&owner); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNoExiste
			}
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT public.cast_private_subject_lock($1)`, owner); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT public.sytue_private_storage_guard('public.cast_private_storage_states'::regclass,$1,$2)`, tenant, owner); err != nil {
			return err
		}
		return write()
	})
}
